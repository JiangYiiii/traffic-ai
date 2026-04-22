# Snapshot Table Updater Skill

## 输出规则 (严格执行)

**本 Skill 执行时，ONLY 输出以下内容：**

1. **变更摘要** (JSON格式):
```json
{
  "summary": {
    "businesses": {"added": 2, "removed": 0, "updated": 1},
    "tables": {"added": 5, "removed": 1, "updated": 3},
    "keywords": {"added": 3, "removed": 0}
  },
  "changes": [
    "+ 新增业务: callback (回调业务)",
    "~ 更新业务: settlement 添加 2 个新表",
    "+ 新增表: cash_loan_callback (loan库)",
    "- 删除表: deprecated_table",
    "+ 新增关键字: @callbackId, @settlementAmount"
  ]
}
```

2. **更新后的完整配置文件** (businesses-tables.json)

**禁止输出：**
- ❌ 扫描过程描述
- ❌ 分析思路和推理过程
- ❌ 中间步骤说明
- ❌ 调试信息
- ❌ "正在扫描..."等进度信息

---

## 功能实现

### 1. 前置检查

执行库表更新前，必须验证以下条件：

```bash
# 检查配置文件存在性
if [ ! -f ".observatory/snapshots/config/businesses-tables.json" ]; then
    echo "❌ businesses-tables.json 不存在"
    exit 1
fi

# 验证 ai_doc_sources 配置
jq -e '.ai_doc_sources' .observatory/snapshots/config/businesses-tables.json > /dev/null
if [ $? -ne 0 ]; then
    echo "❌ ai_doc_sources 配置缺失"
    exit 1
fi
```

**检查项目：**
1. `businesses-tables.json` 存在且格式正确
2. `ai_doc_sources` 配置完整（包含 repo_root, meta_index_dir, flow_doc_dir）
3. 源码仓库路径可访问
4. 必需的外部文件存在性验证

### 2. 扫描源逻辑

**2.1 读取扫描配置**
```javascript
// 从 businesses-tables.json 读取 ai_doc_sources
const config = JSON.parse(readFile('.observatory/snapshots/config/businesses-tables.json'));
const sources = config.ai_doc_sources;

const repoRoot = sources.repo_root;
const metaIndexDir = sources.meta_index_dir;
const flowDocDir = sources.flow_doc_dir;
```

**2.2 业务流程发现**
```bash
# 列出 meta/ 目录下的 ai-index 文件
ls ${repoRoot}/${metaIndexDir}/ai-index-repay-*.json | grep -v table-relations
```

**2.3 核心文件读取**
- `ai-index-repay-{flow}.json` → 提取业务流程 ID
- `ai-index-repay-table-relations.json` → 完整表关联结构
- `repay-{flow}-flow.md` → 业务文档验证（可选）

### 3. 业务发现和对比

**3.1 提取业务 ID**
```javascript
// 从文件名提取业务流程 ID
const flowFiles = glob(`${repoRoot}/${metaIndexDir}/ai-index-repay-*.json`)
    .filter(f => !f.includes('table-relations'))
    .map(f => f.match(/ai-index-repay-(.+)\.json$/)[1]);

console.log("发现业务流程:", flowFiles);
```

**3.2 业务对比逻辑**
```javascript
const existingBusinesses = config.businesses || {};
const newBusinesses = {};

flowFiles.forEach(flowId => {
    if (!existingBusinesses[flowId]) {
        // 新增业务，生成默认配置
        newBusinesses[flowId] = {
            name: `${flowId}_业务`,
            description: `${flowId}相关的业务流程`,
            tables: {}, // 将在表关联解析中填充
            keywords: []
        };
    }
});
```

### 4. 表关联解析

**4.1 解析 table-relations.json 结构**
```javascript
const tableRelations = JSON.parse(readFile(`${repoRoot}/${metaIndexDir}/ai-index-repay-table-relations.json`));

const nodes = tableRelations.nodes || {}; // 表信息: {table_id: {database, table, level}}
const edges = tableRelations.edges || {}; // 关联关系: {edge_id: {from, to, joinCondition}}
const scenarios = tableRelations.scenarios || {}; // 业务场景: {flow: {tableChain: [...]}}
```

**4.2 业务表链提取**
```javascript
Object.keys(scenarios).forEach(flowId => {
    const tableChain = scenarios[flowId].tableChain || [];
    const businessTables = {};
    
    tableChain.forEach(tableId => {
        const node = nodes[tableId];
        if (node) {
            const tableName = node.table;
            const database = node.database;
            const level = node.level || 0;
            
            businessTables[tableName] = {
                database: database,
                sql: generateSQL(tableName, level, tableId, nodes, edges),
                custom: false
            };
        }
    });
    
    // 更新业务配置
    if (config.businesses[flowId]) {
        config.businesses[flowId].tables = businessTables;
    }
});
```

### 5. SQL 生成规则

**5.1 按层级生成 SQL**
```javascript
function generateSQL(tableName, level, tableId, nodes, edges) {
    if (level === 0) {
        // Level 0: 主表，直接用 orderId
        return `SELECT * FROM ${tableName} WHERE ID = @orderId`;
    }
    
    if (level === 1 && hasOrderIdField(tableName)) {
        // Level 1 且有 ORDER_ID 字段
        return `SELECT * FROM ${tableName} WHERE ORDER_ID = @orderId`;
    }
    
    if (level >= 2) {
        // Level 2+: 通过关联表生成子查询
        const parentEdge = findParentEdge(tableId, edges);
        if (parentEdge) {
            const parentTable = nodes[parentEdge.from].table;
            const joinCondition = parentEdge.joinCondition;
            
            // 解析 joinCondition 生成子查询
            const subQuery = generateSubQuery(parentTable, joinCondition);
            return `SELECT * FROM ${tableName} WHERE ${subQuery}`;
        }
    }
    
    // 默认查询
    return `SELECT * FROM ${tableName} WHERE 1=1`;
}

function generateSubQuery(parentTable, joinCondition) {
    // 解析 joinCondition: "t1.ID = t2.PARENT_ID"
    const match = joinCondition.match(/(\w+)\.(\w+)\s*=\s*(\w+)\.(\w+)/);
    if (match) {
        const [, t1, col1, t2, col2] = match;
        return `${col2} IN (SELECT ${col1} FROM ${parentTable} WHERE ORDER_ID = @orderId)`;
    }
    return "1=1";
}
```

**5.2 特殊维度表处理**
```javascript
const dimensionTables = ['account_info', 'user_profile', 'merchant_info'];

if (dimensionTables.includes(tableName)) {
    // 保留现有自定义 SQL 或生成特殊查询
    const existingSQL = config.businesses[flowId]?.tables?.[tableName]?.sql;
    if (existingSQL && config.businesses[flowId].tables[tableName].custom) {
        return existingSQL; // 保护自定义 SQL
    }
    
    // 根据表特性生成特殊查询
    if (tableName === 'account_info') {
        return `SELECT * FROM ${tableName} WHERE ACCOUNT_ID = @accountId`;
    }
}
```

### 6. 关键字提取

**6.1 从 SQL 中提取占位符**
```javascript
function extractKeywords(businesses) {
    const keywords = new Set();
    
    Object.values(businesses).forEach(business => {
        Object.values(business.tables || {}).forEach(table => {
            const sql = table.sql || '';
            const matches = sql.match(/@\w+/g) || [];
            matches.forEach(keyword => keywords.add(keyword.substring(1))); // 去掉 @
        });
    });
    
    return Array.from(keywords).sort();
}

// 使用示例
const allKeywords = extractKeywords(config.businesses);
config.keywords = allKeywords;
```

### 7. 配置更新策略

**7.1 保护机制**
```javascript
function mergeTableConfig(existingTable, newTable) {
    if (existingTable && existingTable.custom === true) {
        // 保护用户自定义的表配置
        return existingTable;
    }
    
    return {
        ...newTable,
        custom: false
    };
}
```

**7.2 全局表内联**
```javascript
function inlineGlobalTables(businesses, globalTables) {
    Object.keys(businesses).forEach(flowId => {
        businesses[flowId].tables = {
            ...globalTables, // 全局表优先
            ...businesses[flowId].tables // 业务特定表覆盖
        };
    });
}

// 确保每个业务包含完整的表配置
const globalTables = config.global_tables || {};
inlineGlobalTables(config.businesses, globalTables);
```

### 8. Diff 生成和输出

**8.1 变更统计**
```javascript
function generateDiffSummary(oldConfig, newConfig) {
    const diff = {
        summary: {
            businesses: {
                added: 0,
                removed: 0, 
                updated: 0
            },
            tables: {
                added: 0,
                removed: 0,
                updated: 0
            },
            keywords: {
                added: 0,
                removed: 0
            }
        },
        changes: []
    };
    
    // 统计业务变更
    const oldBusinesses = Object.keys(oldConfig.businesses || {});
    const newBusinesses = Object.keys(newConfig.businesses || {});
    
    newBusinesses.forEach(id => {
        if (!oldBusinesses.includes(id)) {
            diff.summary.businesses.added++;
            diff.changes.push(`+ 新增业务: ${id} (${newConfig.businesses[id].name})`);
        }
    });
    
    // 统计表变更和关键字变更...
    
    return diff;
}
```

### 9. 执行入口

当触发库表更新时，按以下顺序执行：

1. **验证前置条件** → 检查配置文件和路径
2. **扫描业务流程** → 发现新增/删除的业务
3. **解析表关联** → 提取表链和生成 SQL
4. **更新配置** → 合并新配置，保护自定义设置
5. **输出结果** → 只输出 diff 摘要和最终配置

**完整执行流程：**
```bash
# 1. 前置检查
validate_prerequisites()

# 2. 扫描和解析
scan_ai_doc_sources()
parse_table_relations()

# 3. 生成配置
generate_business_config()
update_global_tables()

# 4. 输出结果 (严格按输出规则)
output_diff_summary()
output_updated_config()
```

---

## 使用示例

**触发方式：**
```
用户: "库表更新"
用户: "扫描 @ai_doc 更新表配置"
用户: "更新 businesses-tables.json"
```

**期望输出：**
1. JSON 格式的变更摘要
2. 完整的 businesses-tables.json 内容

**不会输出任何扫描过程或分析信息。**