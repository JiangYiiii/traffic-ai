#!/usr/bin/env python3
"""
从 traffic 数据面 GET /v1/models 拉取当前 API Key 可用模型，写回 OpenClaw 的 traffic 段。

- 不依赖浏览器；与 token 的可用模型列表一致（按数据面 /v1/models 路由结果）。
- 建议由 OpenClaw 自带 Gateway 定时：运行 scripts/openclaw-cron-install-traffic-sync.sh
  注册后由 ~/.openclaw/cron/ 内任务调度；亦可手动执行本脚本或配合
  openclaw-run-traffic-model-sync.sh。

环境变量:
  OPENCLAW_CONFIG       OpenClaw 主配置路径，默认 ~/.openclaw/openclaw.json
  TRAFFIC_MODELS_PATH   覆盖 list models 的 URL 路径，默认 {baseUrl}/models（baseUrl 从配置读）
  DEFAULT_CONTEXT       未提供时的 contextWindow，默认 128000
  DEFAULT_MAX_TOKENS     未提供时的 maxTokens，默认 8192
  TRAFFIC_PRIMARY_MODEL  若设且仍在新列表中，则固定为 primary；否则保留原 primary（若仍可用），否则取排序后首条
  TRAFFIC_EMBEDDING_MODEL   可选：强制指定记忆检索用的 embedding 模型 id（须出现在 /v1/models 列表中）
  TRAFFIC_EMBEDDING_DIMENSIONS  可选：向量维度；自定义模型名且非内置维度表时必须设置（与 OpenClaw embedding.dimensions 一致）
  OPENCLAW_SYNC_RESTART  设为 1 时，在配置有变更时执行: openclaw daemon restart
  DRY_RUN               设为 1 时只打印差异不写文件、不 restart

同一 Traffic 线路会同步写入 agents.defaults.memorySearch（provider=openai + remote.baseUrl/apiKey，
与 OpenClaw 2026.4+ schema 一致）。不再写入根级 embedding（doctor 会报 Unrecognized key）。
"""
from __future__ import annotations

import json
import os
import shutil
import subprocess
import sys
import urllib.error
import urllib.parse
import urllib.request
from datetime import datetime, timezone
from pathlib import Path
from typing import Any


def _read_json(p: Path) -> dict[str, Any]:
    with p.open("r", encoding="utf-8") as f:
        return json.load(f)


def _write_json(p: Path, data: dict[str, Any]) -> None:
    text = json.dumps(data, ensure_ascii=False, indent=2) + "\n"
    p.write_text(text, encoding="utf-8")


def _http_get_json(url: str, token: str, timeout: float) -> dict[str, Any]:
    req = urllib.request.Request(
        url,
        headers={"Authorization": f"Bearer {token}"},
        method="GET",
    )
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        body = resp.read().decode("utf-8")
    return json.loads(body)


def _is_reasoning_model(model_id: str) -> bool:
    s = model_id.lower()
    for part in (
        "o1",
        "o3",
        "gpt-5",
        "reasoning",
        "think",
        "r1",
        "deep-research",
    ):
        if part in s:
            return True
    return False


def _build_model_entry(
    model_id: str, ctx: int, max_tok: int, previous_by_id: dict[str, dict]
) -> dict[str, Any]:
    prev = previous_by_id.get(model_id)
    if prev:
        out = dict(prev)
        out["id"] = model_id
        if "name" not in out or not str(out.get("name", "")).strip():
            out["name"] = f"{model_id} (traffic local)"
        if "contextWindow" not in out:
            out["contextWindow"] = ctx
        if "maxTokens" not in out:
            out["maxTokens"] = max_tok
        if "input" not in out:
            out["input"] = ["text"]
        if "reasoning" not in out:
            out["reasoning"] = _is_reasoning_model(model_id)
        return out

    return {
        "id": model_id,
        "name": f"{model_id} (traffic local)",
        "reasoning": _is_reasoning_model(model_id),
        "input": ["text"],
        "cost": {
            "input": 0,
            "output": 0,
            "cacheRead": 0,
            "cacheWrite": 0,
        },
        "contextWindow": ctx,
        "maxTokens": max_tok,
    }


def _parse_models_response(body: dict[str, Any]) -> list[str]:
    data = body.get("data")
    if not isinstance(data, list):
        return []
    ids: list[str] = []
    for item in data:
        if not isinstance(item, dict):
            continue
        mid = item.get("id")
        if isinstance(mid, str) and mid.strip():
            ids.append(mid.strip())
    return sorted(set(ids))


# OpenClaw 内建校验与常见 OpenAI embedding 维度（与 node_modules/openclaw 中 vectorDimsForModel 对齐）
KNOWN_EMBEDDING_DIMS: dict[str, int] = {
    "text-embedding-3-small": 1536,
    "text-embedding-3-large": 3072,
    "text-embedding-ada-002": 1536,
}

_EMB_NAME_HINTS = ("embed", "embedding", "bge", "e5", "gte", "m3", "text-embedding")


def _resolve_embedding_model_id(
    id_list: list[str],
    env_model: str | None,
    prev_model: str | None,
) -> str | None:
    if env_model:
        if env_model in id_list:
            return env_model
        print(
            f"WARN: TRAFFIC_EMBEDDING_MODEL={env_model} 不在当前 /v1/models 列表中，跳过 embedding 块更新。",
            file=sys.stderr,
        )
        return None
    if prev_model and prev_model in id_list:
        return prev_model
    for mid in id_list:
        low = mid.lower()
        if any(h in low for h in _EMB_NAME_HINTS):
            return mid
    return None


def _resolve_embedding_dimensions(
    model_id: str,
    env_dims: str | None,
    prev_same_model: bool,
    prev_dims: int | None,
) -> int | None:
    if env_dims:
        try:
            return int(env_dims.strip())
        except ValueError:
            print("WARN: TRAFFIC_EMBEDDING_DIMENSIONS 非法，跳过 embedding 更新。", file=sys.stderr)
            return None
    if model_id in KNOWN_EMBEDDING_DIMS:
        return KNOWN_EMBEDDING_DIMS[model_id]
    if prev_same_model and isinstance(prev_dims, int):
        return prev_dims
    print(
        f"WARN: 无法推断向量维度（model={model_id}）。请设置 TRAFFIC_EMBEDDING_DIMENSIONS。跳过 embedding 更新。",
        file=sys.stderr,
    )
    return None


def _build_embedding_block(
    data: dict[str, Any],
    traffic: dict[str, Any],
    id_list: list[str],
) -> dict[str, Any] | None:
    """若可解析 model 与 dimensions，返回写入 openclaw.json 的 embedding 对象；否则 None 保留原配置。"""
    api_key = traffic.get("apiKey")
    base_url = str(traffic.get("baseUrl", "")).rstrip("/")
    if not api_key or not isinstance(api_key, str) or not base_url:
        return None

    prev = data.get("embedding")
    prev_model = prev.get("model") if isinstance(prev, dict) else None
    prev_dims = prev.get("dimensions") if isinstance(prev, dict) else None

    env_model = os.environ.get("TRAFFIC_EMBEDDING_MODEL", "").strip() or None
    env_dims = os.environ.get("TRAFFIC_EMBEDDING_DIMENSIONS", "").strip() or None

    prev_model_s = prev_model if isinstance(prev_model, str) else None
    model_id = _resolve_embedding_model_id(id_list, env_model, prev_model_s)
    if not model_id:
        print(
            "提示: 未解析到 embedding 模型：请在 Traffic 上架含 embed/bge 等名称的模型，或设置 TRAFFIC_EMBEDDING_MODEL。保留原 embedding 配置。",
            file=sys.stderr,
        )
        return None

    prev_same = prev_model_s == model_id
    dims = _resolve_embedding_dimensions(model_id, env_dims, prev_same, prev_dims)
    if dims is None:
        return None

    return {
        "apiKey": api_key,
        "baseUrl": base_url,
        "model": model_id,
        "dimensions": dims,
    }


def _merge_openclaw_memory_search(
    defaults: dict[str, Any],
    base_url: str,
    api_key: str,
    emb: dict[str, Any] | None,
) -> tuple[dict[str, Any], bool]:
    """写入 agents.defaults.memorySearch（OpenAI 兼容远端 = Traffic 网关 /v1/embeddings）。"""
    prev = defaults.get("memorySearch")
    prev_d: dict[str, Any] = dict(prev) if isinstance(prev, dict) else {}
    if emb is None:
        return prev_d, False
    model_id = emb["model"]
    dims = emb["dimensions"]
    remote_prev = prev_d.get("remote") if isinstance(prev_d.get("remote"), dict) else {}
    merged_remote = {**remote_prev, "baseUrl": base_url, "apiKey": api_key}
    merged: dict[str, Any] = {
        **prev_d,
        "enabled": True,
        "provider": "openai",
        "model": model_id,
        "outputDimensionality": dims,
        "remote": merged_remote,
    }
    changed = json.dumps(merged, sort_keys=True, ensure_ascii=True) != json.dumps(
        prev_d, sort_keys=True, ensure_ascii=True
    )
    return merged, changed


def _pick_primary(
    ids: list[str],
    env_primary: str | None,
    current_primary: str,
    provider_tag: str,
) -> str:
    """primary 为 traffic/<id> 形式。"""
    prefix = f"{provider_tag}/"
    if env_primary and env_primary.startswith(prefix):
        eid = env_primary[len(prefix) :]
        if eid in ids:
            return env_primary
    if current_primary.startswith(prefix):
        cur_id = current_primary[len(prefix) :]
        if cur_id in ids:
            return current_primary
    if ids:
        return f"{provider_tag}/{ids[0]}"
    return current_primary


def main() -> int:
    dry = os.environ.get("DRY_RUN", "").strip() in ("1", "true", "yes")
    cfg_path = Path(
        os.environ.get("OPENCLAW_CONFIG", "").strip()
        or (Path.home() / ".openclaw" / "openclaw.json")
    )
    if not cfg_path.is_file():
        print(f"ERROR: 配置文件不存在: {cfg_path}", file=sys.stderr)
        return 1

    ctx_default = int(os.environ.get("DEFAULT_CONTEXT", "128000"))
    max_tok_default = int(os.environ.get("DEFAULT_MAX_TOKENS", "8192"))
    env_primary = os.environ.get("TRAFFIC_PRIMARY_MODEL", "").strip() or None
    if env_primary and "/" not in env_primary:
        env_primary = f"traffic/{env_primary.lstrip()}"
    restart_on_change = os.environ.get("OPENCLAW_SYNC_RESTART", "").strip() in (
        "1",
        "true",
        "yes",
    )
    provider_tag = "traffic"
    models_url_override = os.environ.get("TRAFFIC_MODELS_PATH", "").strip()

    data = _read_json(cfg_path)
    try:
        traffic = data["models"]["providers"][provider_tag]
    except KeyError:
        print(
            f"ERROR: 配置中无 models.providers.{provider_tag}，请先在 openclaw 中配置该 provider。",
            file=sys.stderr,
        )
        return 1

    base_url = str(traffic.get("baseUrl", "")).rstrip("/")
    api_key = traffic.get("apiKey")
    if not base_url or not api_key or not isinstance(api_key, str):
        print("ERROR: traffic 缺少 baseUrl 或 apiKey。", file=sys.stderr)
        return 1

    if models_url_override:
        list_url = models_url_override
    else:
        # baseUrl 形如 http://host:8081/v1 → 列表为 /v1/models
        list_url = f"{base_url}/models"

    timeout = float(os.environ.get("SYNC_HTTP_TIMEOUT", "30"))

    try:
        remote = _http_get_json(list_url, api_key, timeout=timeout)
    except urllib.error.HTTPError as e:
        print(
            f"ERROR: 拉取模型列表失败 HTTP {e.code} {e.reason} URL={list_url}",
            file=sys.stderr,
        )
        return 1
    except (urllib.error.URLError, TimeoutError, json.JSONDecodeError, OSError) as e:
        print(f"ERROR: 拉取或解析失败: {e!s} URL={list_url}", file=sys.stderr)
        return 1

    id_list = _parse_models_response(remote)
    if not id_list:
        print(
            f"WARNING: 远端返回 0 个模型，跳过写回以免清空本地列表。URL={list_url}",
            file=sys.stderr,
        )
        return 0

    prev_models = traffic.get("models", [])
    if not isinstance(prev_models, list):
        prev_models = []
    previous_by_id: dict[str, dict] = {}
    for m in prev_models:
        if isinstance(m, dict) and isinstance(m.get("id"), str):
            previous_by_id[m["id"]] = m

    new_models: list[dict[str, Any]] = []
    for mid in id_list:
        new_models.append(
            _build_model_entry(mid, ctx_default, max_tok_default, previous_by_id)
        )

    def norm_models(ms: list) -> str:
        return json.dumps(
            [m if isinstance(m, dict) and "id" in m else m for m in ms],
            sort_keys=True,
            ensure_ascii=True,
        )

    traffic_changed = norm_models(traffic.get("models", [])) != norm_models(
        new_models
    )

    new_embedding = _build_embedding_block(data, traffic, id_list)

    ad = data.get("agents", {}).get("defaults", {})
    if not isinstance(ad, dict):
        ad = {}
    new_memory_search, memory_search_changed = _merge_openclaw_memory_search(
        ad, base_url, api_key, new_embedding
    )
    mcfg = ad.get("model", {})
    sub = ad.get("subagents", {})
    current_primary = str(mcfg.get("primary", f"{provider_tag}/{id_list[0]}"))

    new_primary = _pick_primary(
        id_list, env_primary, current_primary, provider_tag
    )
    # 子代理默认与主模型一致。旧逻辑会在「旧 sub.id 仍在 id_list」时保留（例如仍为 haiku），
    # 与用户通过 TRAFFIC_PRIMARY_MODEL 切换主模型时期望不一致，故不再保留旧 sub。
    new_sub = new_primary

    new_agent_models: dict[str, Any] = {}
    for mid in id_list:
        k = f"{provider_tag}/{mid}"
        am = (ad.get("models") or {}).get(k) if isinstance(ad.get("models"), dict) else None
        if am is not None and isinstance(am, dict):
            new_agent_models[k] = am
        else:
            new_agent_models[k] = {}

    agents_changed = (
        str(mcfg.get("primary", "")) != new_primary
        or str(sub.get("model", "")) != new_sub
        or json.dumps(ad.get("models"), sort_keys=True)
        != json.dumps(new_agent_models, sort_keys=True)
    )

    if not traffic_changed and not agents_changed and not memory_search_changed:
        now = datetime.now(timezone.utc).replace(microsecond=0)
        print(f"OK: 与远端一致，无需写回。models={len(id_list)} 检查时间={now.isoformat()}")
        return 0

    if dry:
        print(
            f"DRY_RUN: 将写入 {len(id_list)} 个模型，primary={new_primary} subagents={new_sub}"
        )
        for mid in id_list:
            print(f"  - {mid}")
        if new_embedding is not None:
            print(
                f"DRY_RUN: memorySearch model={new_embedding.get('model')} dim={new_embedding.get('dimensions')} provider=openai"
            )
        return 0

    # 备份
    backup = cfg_path.with_suffix(".json.bak")
    try:
        shutil.copy2(cfg_path, backup)
    except OSError as e:
        print(f"ERROR: 备份失败: {e!s}", file=sys.stderr)
        return 1

    if not isinstance(data.get("meta"), dict):
        data["meta"] = {}
    data["meta"]["lastTouchedAt"] = (
        datetime.now(timezone.utc).replace(microsecond=0).isoformat().replace("+00:00", "Z")
    )

    data.setdefault("models", {}).setdefault("providers", {})[provider_tag] = {
        **traffic,
        "models": new_models,
    }

    data["agents"] = data.get("agents", {})
    data["agents"].setdefault("defaults", ad)
    d = data["agents"]["defaults"]
    d["model"] = d.get("model", {})
    d["model"]["primary"] = new_primary
    d["model"]["fallbacks"] = mcfg.get("fallbacks", [])
    d["models"] = new_agent_models
    d["subagents"] = {**d.get("subagents", {}), "model": new_sub}

    d["memorySearch"] = new_memory_search
    data.pop("embedding", None)

    try:
        _write_json(cfg_path, data)
    except OSError as e:
        print(f"ERROR: 写入失败: {e!s}", file=sys.stderr)
        return 1

    now = datetime.now(timezone.utc).replace(microsecond=0)
    emb_note = ""
    if new_embedding is not None and memory_search_changed:
        emb_note = f" memorySearch={new_embedding.get('model')} dim={new_embedding.get('dimensions')}"
    print(
        f"OK: 已更新 {cfg_path}（备份 {backup}）models={len(id_list)} primary={new_primary}{emb_note} @ {now.isoformat()}"
    )

    if restart_on_change and shutil.which("openclaw"):
        try:
            r = subprocess.run(
                ["openclaw", "daemon", "restart"],
                check=False,
                timeout=60,
                capture_output=True,
                text=True,
            )
            if r.returncode == 0:
                print("OK: openclaw daemon restart 已执行。")
            else:
                err = (r.stderr or r.stdout or "").strip()
                print(
                    f"WARN: openclaw daemon restart 返回 {r.returncode}：{err}",
                    file=sys.stderr,
                )
        except (subprocess.SubprocessError, OSError) as e:
            print(f"WARN: 无法执行 openclaw daemon restart: {e!s}", file=sys.stderr)
    if not restart_on_change:
        print("提示: 可设置 OPENCLAW_SYNC_RESTART=1 以在变更后自动 openclaw daemon restart。")

    return 0


if __name__ == "__main__":
    sys.exit(main())
