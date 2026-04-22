#!/usr/bin/env bash
# 通过 OpenClaw 内置 Gateway cron（非 launchd）定时触发：仅允许 exec 工具，执行
# openclaw-run-traffic-model-sync.sh → openclaw-sync-traffic-models.py
#
# 重要说明: OpenClaw 的 payload 只支持 systemEvent 或 agentTurn，没有“纯无模型 shell”
# 类型。本方式用极短、仅-exec 的独立会话跑同步脚本；调度与持久化在 Gateway（~/.openclaw/cron/jobs.json），
# 与 launchd 无关。
#
# 用法:
#   chmod +x scripts/openclaw-cron-install-traffic-sync.sh
#   ./scripts/openclaw-cron-install-traffic-sync.sh
#
# 环境变量:
#   OPENCLAW_TRAFFIC_SYNC_NAME  任务名，默认 traffic-openclaw-model-sync
#   UNINSTALL                   设为非空则只删除该任务
#   OPENCLAW_TRAFFIC_SYNC_EVERY 默认 15m
#   OPENCLAW_TRAFFIC_SYNC_MODEL  可选: 指定本 cron 的 agent 调用模型, e.g. traffic/qwen3.5-flash
#   REPO_ROOT                   默认脚本所在项目根
set -euo pipefail
REPO_ROOT="${REPO_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
JOB_NAME="${OPENCLAW_TRAFFIC_SYNC_NAME:-traffic-openclaw-model-sync}"
SYNC_SH="${SYNC_SH:-$REPO_ROOT/scripts/openclaw-run-traffic-model-sync.sh}"
EVERY="${OPENCLAW_TRAFFIC_SYNC_EVERY:-15m}"

if [[ ! -f "$SYNC_SH" || ! -x "$SYNC_SH" ]]; then
  echo "缺少可执行: $SYNC_SH （chmod +x 后再试）" >&2
  exit 1
fi

# 提取 JSON 对象（略过 openclaw 的 stderr/警告行）
_cron_list_json() {
  openclaw cron list --all --json 2>&1 | python3 -c 'import json,sys; s=sys.stdin.read(); i=s.find("{"); print(s[i:])' 
}

_remove_job_by_name() {
  local name="$1"
  while IFS= read -r id; do
    [[ -z "$id" ]] && continue
    openclaw cron rm "$id" 2>&1
  done < <(_cron_list_json | NAME="$name" python3 -c "
import json,os,sys
d=json.load(sys.stdin)
n=os.environ['NAME']
for j in d.get('jobs',[]):
  if j.get('name')==n:
    print(j['id'])
")
}

if [[ -n "${UNINSTALL:-}" ]]; then
  _remove_job_by_name "$JOB_NAME" || true
  echo "已尝试移除任务: $JOB_NAME"
  exit 0
fi

_remove_job_by_name "$JOB_NAME" 2>/dev/null || true

# 用绝对路径，避免子进程里 cwd 不一致
ABS_SH="$(cd "$(dirname "$SYNC_SH")" && pwd)/$(basename "$SYNC_SH")"
Q_SH=$(printf %q "$ABS_SH")
# agentTurn 会走一阶模型，但只放行 exec 工具，指令尽量短
CRON_MSG="只用 exec 一次: ${Q_SH} 不要 read/write。成功只回 OK 失败只回一行错。"

ADD_ARGS=(openclaw cron add
  --name "$JOB_NAME"
  --description "traffic 网关 /v1/models 同步到 ~/.openclaw (OpenClaw cron+exec)"
  --every "$EVERY"
  --session isolated
  --wake now
  --message "$CRON_MSG"
  --tools exec
  --thinking off
  --no-deliver
  --light-context
  --timeout-seconds 180
)

if [[ -n "${OPENCLAW_TRAFFIC_SYNC_MODEL:-}" ]]; then
  ADD_ARGS+=(--model "$OPENCLAW_TRAFFIC_SYNC_MODEL")
fi

echo "正在向 Gateway 注册 cron 任务: $JOB_NAME（$EVERY）"
"${ADD_ARGS[@]}"

echo
echo "完成。查看: openclaw cron list  手动触发: openclaw cron run <上表中的 jobId>"
echo "本任务存于: ~/.openclaw/cron/jobs.json ；卸载: UNINSTALL=1 $0 或: openclaw cron list --all --json 后按 id 删除"
echo
echo "需变更后自动 gateway 时，编辑 $REPO_ROOT/scripts/openclaw-run-traffic-model-sync.sh 中 OPENCLAW_SYNC_RESTART=1，或设环境变量后改为包装调用。"
