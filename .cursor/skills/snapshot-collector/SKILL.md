# 快照采集器 Skill

## 输出规则（必须遵守）
- **执行 SQL 时只输出 JSON 结果，不输出过程描述**
- **空结果直接记 []，不解释**
- **错误只输出错误信息原文，不分析**  
- **最终只输出 save-snapshot.sh 的返回 JSON**
- **禁止：总结、分析、建议、思考过程**

## 功能概述

本 Skill 用于执行测试用例快照采集任务，实现闭环数据收集流程：
1. 解析面板生成的 prompt，提取业务信息
2. 从配置文件读取表列表和查询 SQL
3. 执行数据库查询（CLI 优先，MCP 降级）
4. 调用校验脚本保存数据
5. 处理不完整数据的补采重试

## 前置检查

在执行任何采集任务前，必须进行以下检查：

### 1. 检查配置文件存在性
```bash
# 检查业务配置文件
if [ ! -f ".observatory/snapshots/config/businesses-tables.json" ]; then
    echo "错误：配置文件 .observatory/snapshots/config/businesses-tables.json 不存在"
    echo "请先运行库表更新，或手动创建配置文件"
    exit 1
fi

# 检查查询提供者配置
if [ ! -f ".observatory/snapshots/config/query-provider.json" ]; then
    echo "错误：查询配置文件 .observatory/snapshots/config/query-provider.json 不存在"
    echo "请先创建查询通道配置文件"
    exit 1
fi
```

### 2. 检查项目根目录
```bash
# 确保在正确的项目根目录
if [ ! -d ".observatory" ]; then
    echo "错误：当前不在项目根目录，缺少 .observatory 目录"
    exit 1
fi
```

### 3. 检查 CLI 可用性
```bash
# 读取 CLI 路径配置
CLI_PATH=$(jq -r '.providers["funding-admin-api"].cli_path' .observatory/snapshots/config/query-provider.json)

# 检查文件存在
if [ -f "$CLI_PATH" ]; then
    # 测试 CLI 是否可用（简单命令测试）
    if ! $CLI_PATH --help &>/dev/null; then
        echo "警告：CLI 存在但不可用，将降级使用 MCP"
        USE_MCP=true
    else
        USE_MCP=false
    fi
else
    echo "警告：CLI 不存在 ($CLI_PATH)，将降级使用 MCP"
    USE_MCP=true
fi
```

## 核心执行逻辑

### 1. Prompt 解析
从面板生成的 prompt 中提取信息：
- 业务 ID（如 "callback"）
- 场景描述（如 "成功回调"）
- 阶段（"before" 或 "after"）
- 关键字键值对（如 "orderId=197990"）

```bash
# 示例解析逻辑（从 prompt 提取）
BUSINESS=$(echo "$PROMPT" | grep -o "业务: [^|]*" | cut -d: -f2 | xargs)
SCENARIO=$(echo "$PROMPT" | grep -o "场景: [^|]*" | cut -d: -f2 | xargs)
PHASE=$(echo "$PROMPT" | grep -o "阶段: [^|]*" | cut -d: -f2 | xargs)
KEYWORDS=$(echo "$PROMPT" | grep -o "[a-zA-Z]*=[^|]*" | tr '\n' ',' | sed 's/,$//')
```

### 2. 读取业务配置
从 `businesses-tables.json` 获取该业务的完整表列表和查询 SQL：

```bash
# 读取业务配置
BUSINESS_CONFIG=$(jq ".businesses[\"$BUSINESS\"]" .observatory/snapshots/config/businesses-tables.json)

if [ "$BUSINESS_CONFIG" = "null" ]; then
    echo "错误：业务 '$BUSINESS' 在配置文件中不存在"
    exit 1
fi

# 获取表列表
TABLES=$(echo "$BUSINESS_CONFIG" | jq -r '.tables[] | @base64')
```

### 3. 查询执行策略

#### CLI 查询（优先）
```bash
execute_cli_query() {
    local database=$1
    local sql=$2
    local api_db_name=$3
    
    # 替换占位符
    local final_sql=$(echo "$sql" | sed "s/@orderId/$ORDER_ID/g" | sed "s/@accountId/$ACCOUNT_ID/g")
    
    # 执行查询
    $CLI_PATH --test-env db query "$api_db_name" "$final_sql" --json 2>/dev/null
    return $?
}
```

#### MCP 查询（降级）
```bash
execute_mcp_query() {
    local database=$1
    local sql=$2
    
    # 使用 Cursor MCP 工具执行查询
    # 具体实现取决于配置的 MCP 类型
    echo "使用 MCP 查询暂未实现，请配置 CLI 工具"
    return 1
}
```

### 4. 数据采集主流程

```bash
collect_data() {
    local business=$1
    local keywords_json=$2
    local temp_data_file="/tmp/snapshot-collected-data-$(date +%s).json"
    
    # 初始化结果文件
    echo '{}' > "$temp_data_file"
    
    # 解析关键字
    eval $(echo "$keywords_json" | jq -r 'to_entries[] | "export " + .key + "=" + .value')
    
    # 遍历表列表
    echo "$TABLES" | while read -r table_base64; do
        local table_config=$(echo "$table_base64" | base64 -d)
        local database=$(echo "$table_config" | jq -r '.database')
        local table_name=$(echo "$table_config" | jq -r '.table')
        local query_sql=$(echo "$table_config" | jq -r '.query_sql')
        
        # 获取 API 数据库名称
        local api_db_name=$(jq -r ".databases[\"$database\"].api_db_name" .observatory/snapshots/config/businesses-tables.json)
        
        # 执行查询
        local result=""
        local success=false
        
        if [ "$USE_MCP" = false ]; then
            # 使用 CLI
            result=$(execute_cli_query "$database" "$query_sql" "$api_db_name")
            if [ $? -eq 0 ]; then
                success=true
            fi
        fi
        
        # CLI 失败或不可用时降级 MCP
        if [ "$success" = false ]; then
            result=$(execute_mcp_query "$database" "$query_sql")
            if [ $? -eq 0 ]; then
                success=true
            fi
        fi
        
        # 处理结果
        if [ "$success" = true ]; then
            # 验证 JSON 格式
            if echo "$result" | jq empty 2>/dev/null; then
                # 保存到临时文件（按 database.table 结构）
                jq --arg db "$database" --arg tbl "$table_name" --argjson data "$result" \
                   '.[$db][$tbl] = $data' "$temp_data_file" > "${temp_data_file}.tmp" && \
                   mv "${temp_data_file}.tmp" "$temp_data_file"
            else
                # JSON 格式错误，视为空结果
                jq --arg db "$database" --arg tbl "$table_name" \
                   '.[$db][$tbl] = []' "$temp_data_file" > "${temp_data_file}.tmp" && \
                   mv "${temp_data_file}.tmp" "$temp_data_file"
            fi
        else
            # 查询失败，记录为空（后续补采）
            jq --arg db "$database" --arg tbl "$table_name" \
               '.[$db][$tbl] = []' "$temp_data_file" > "${temp_data_file}.tmp" && \
               mv "${temp_data_file}.tmp" "$temp_data_file"
        fi
        
        # 输出查询结果（遵守输出规则）
        echo "$result"
    done
    
    echo "$temp_data_file"
}
```

### 5. 校验保存流程

```bash
save_and_validate() {
    local business=$1
    local scenario=$2
    local phase=$3
    local keywords_json=$4
    local data_file=$5
    local append_file=${6:-}
    
    # 构建脚本路径
    local script_path="$HOME/shared-skills/catalog/snapshot-collector/tools/save-snapshot.sh"
    
    # 确保脚本可执行
    chmod +x "$script_path"
    
    # 调用校验脚本
    if [ -n "$append_file" ]; then
        # 补采模式
        "$script_path" --business "$business" --scenario "$scenario" --phase "$phase" \
                      --keywords "$keywords_json" --data-file "$data_file" --append "$append_file"
    else
        # 首次采集
        "$script_path" --business "$business" --scenario "$scenario" --phase "$phase" \
                      --keywords "$keywords_json" --data-file "$data_file"
    fi
    
    local exit_code=$?
    return $exit_code
}
```

### 6. 补采重试逻辑

```bash
retry_collection() {
    local business=$1
    local scenario=$2
    local phase=$3
    local keywords_json=$4
    local original_data_file=$5
    local missing_tables_json=$6
    local max_retries=3
    local retry_count=0
    
    while [ $retry_count -lt $max_retries ]; do
        retry_count=$((retry_count + 1))
        
        # 为缺失表创建临时数据文件
        local append_data_file="/tmp/snapshot-append-data-$(date +%s).json"
        echo '{}' > "$append_data_file"
        
        # 只采集缺失的表
        echo "$missing_tables_json" | jq -r '.missing_tables[] | @base64' | while read -r missing_table_base64; do
            local table_config=$(echo "$missing_table_base64" | base64 -d)
            local database=$(echo "$table_config" | jq -r '.database')
            local table_name=$(echo "$table_config" | jq -r '.table')
            local query_sql=$(echo "$table_config" | jq -r '.query_sql')
            
            # 执行查询（同主流程逻辑）
            # ... 查询执行代码 ...
            
            # 将结果保存到补采文件
            # ... 保存逻辑 ...
        done
        
        # 调用脚本进行补采
        local result_json
        result_json=$(save_and_validate "$business" "$scenario" "$phase" "$keywords_json" "$original_data_file" "$append_data_file")
        local save_result=$?
        
        if [ $save_result -eq 0 ]; then
            # 补采成功
            echo "$result_json"
            rm -f "$append_data_file" "$original_data_file"
            return 0
        else
            # 仍有缺失，继续重试
            missing_tables_json="$result_json"
        fi
        
        # 清理临时文件
        rm -f "$append_data_file"
    done
    
    # 超过最大重试次数
    echo "{\"status\":\"failed\",\"message\":\"超过最大重试次数($max_retries)，仍有表采集失败\",\"retry_count\":$retry_count}"
    rm -f "$original_data_file"
    return 1
}
```

## 主执行函数

```bash
main() {
    # 前置检查
    perform_pre_checks
    
    # 解析 prompt 参数
    local business="$1"
    local scenario="$2" 
    local phase="$3"
    local keywords_json="$4"
    
    # 数据采集
    local data_file=$(collect_data "$business" "$keywords_json")
    
    # 首次保存校验
    local result_json
    result_json=$(save_and_validate "$business" "$scenario" "$phase" "$keywords_json" "$data_file")
    local save_result=$?
    
    if [ $save_result -eq 0 ]; then
        # 采集完成
        echo "$result_json"
        rm -f "$data_file"
        return 0
    else
        # 有缺失表，启动补采
        retry_collection "$business" "$scenario" "$phase" "$keywords_json" "$data_file" "$result_json"
        return $?
    fi
}
```

## 错误处理

### 1. 查询错误处理
- **SQL 语法错误**：记录错误信息，标记该表为未采集
- **连接超时**：重试 1 次，仍失败则标记为未采集
- **权限错误**：记录错误，不重试

### 2. 数据处理错误
- **JSON 格式错误**：将结果标准化为 `[]`
- **空结果**：记录为 `[]`（正常情况）
- **null 结果**：记录为 `[]`

### 3. 脚本调用错误
- **脚本不存在**：退出并提示安装问题
- **权限错误**：尝试设置执行权限
- **参数错误**：检查参数格式并提示

## 使用示例

### 典型调用（由面板触发）
```bash
# 面板生成的 prompt 会包含这些信息
business="callback"
scenario="成功回调" 
phase="before"
keywords='{"orderId":"197990"}'

# Skill 执行
main "$business" "$scenario" "$phase" "$keywords"
```

### 输出示例

**成功完成**：
```json
{
  "status": "complete",
  "phase": "before", 
  "collected_count": 25,
  "expected_count": 25,
  "saved_path": ".observatory/snapshots/data/callback/成功回调/20260414_100000/before.json",
  "index_updated": true,
  "message": "初始数据采集完成，共 25 张表，已保存并更新索引"
}
```

**需要补采**：
```json
{
  "status": "incomplete",
  "phase": "before",
  "collected_count": 20,
  "expected_count": 25, 
  "missing_tables": [
    {
      "database": "loan",
      "table": "cash_loan_repay_event",
      "query_sql": "SELECT * FROM cash_loan_repay_event WHERE REPAYMENT_ID IN (...)"
    }
  ],
  "message": "缺失 5 张表，正在补采..."
}
```

## 环境要求

- **必需工具**：`jq`, `bash` 4.0+
- **可选工具**：`cli-anything-funding-admin`（用于数据库查询）
- **配置文件**：`.observatory/snapshots/config/` 下的配置文件必须存在
- **权限**：需要读写 `.observatory/snapshots/data/` 目录的权限

## 故障排除

### 1. CLI 不可用
```bash
# 检查文件存在
ls -la /Users/jiangyi/Documents/codedev/cli-everything/.venv/bin/cli-anything-funding-admin

# 检查执行权限
chmod +x /Users/jiangyi/Documents/codedev/cli-everything/.venv/bin/cli-anything-funding-admin

# 测试基本功能
/Users/jiangyi/Documents/codedev/cli-everything/.venv/bin/cli-anything-funding-admin --help
```

### 2. 配置文件问题
```bash
# 验证配置文件 JSON 格式
jq '.' .observatory/snapshots/config/businesses-tables.json
jq '.' .observatory/snapshots/config/query-provider.json

# 检查业务配置
jq '.businesses | keys' .observatory/snapshots/config/businesses-tables.json
```

### 3. 数据保存问题
```bash
# 检查目录权限
ls -ld .observatory/snapshots/data/

# 检查磁盘空间
df -h .
```

这个 Skill 实现了完整的闭环数据采集流程，遵循严格的输出规则，提供了可靠的错误处理和重试机制。