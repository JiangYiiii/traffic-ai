#!/usr/bin/env bash
# 用 Podman 启动 TEI（Text Embeddings Inference），对外提供 OpenAI 兼容 /v1/embeddings。
#
# 用法:
#   chmod +x scripts/podman-tei-embedding.sh
#   ./scripts/podman-tei-embedding.sh
#
# 环境变量（可选）:
#   TEI_CONTAINER_NAME  容器名，默认 tei-embedding
#   TEI_HOST_PORT       宿主机端口，默认 8088（容器内固定 80）
#   TEI_MODEL_ID        Hugging Face 模型 id，默认 BAAI/bge-m3
#   TEI_SERVED_NAME     OpenAI 请求里用的 model 字段，默认与 TEI_MODEL_ID 最后一段一致（如 bge-m3）
#   TEI_IMAGE           镜像；Apple Silicon 用 cpu-arm64，Intel Mac/Linux amd64 用 cpu
#   HF_TOKEN            若模型需登录，设置 Hugging Face token
#
# Traffic 上游线路建议:
#   接入地址: http://127.0.0.1:${TEI_HOST_PORT}/v1
#   协议: chat
#   模型名 model_name: 与 TEI_SERVED_NAME 一致
#   凭据: 任意非空占位（如 sk-local）；TEI 默认不校验 Bearer，Traffic 仍会代发 Authorization
#
set -euo pipefail

NAME="${TEI_CONTAINER_NAME:-tei-embedding}"
PORT="${TEI_HOST_PORT:-8088}"
MODEL_ID="${TEI_MODEL_ID:-BAAI/bge-m3}"
SERVED="${TEI_SERVED_NAME:-${MODEL_ID##*/}}"

ARCH="$(uname -m)"
if [[ "$ARCH" == "arm64" ]]; then
  DEFAULT_IMG="ghcr.io/huggingface/text-embeddings-inference:cpu-arm64-1.9"
else
  DEFAULT_IMG="ghcr.io/huggingface/text-embeddings-inference:cpu-1.9"
fi
IMAGE="${TEI_IMAGE:-$DEFAULT_IMG}"

if podman container exists "$NAME" 2>/dev/null; then
  if [[ "$(podman container inspect --format '{{.State.Running}}' "$NAME" 2>/dev/null)" == "true" ]]; then
    echo "容器已在运行: $NAME  日志: podman logs -f $NAME"
  else
    echo "启动已有容器: $NAME"
    podman start "$NAME"
  fi
  exit 0
fi

VOL="tei-hf-cache"
podman volume inspect "$VOL" >/dev/null 2>&1 || podman volume create "$VOL" >/dev/null

PASS_ARGS=(--model-id "$MODEL_ID" --served-model-name "$SERVED")
if [[ -n "${HF_TOKEN:-}" ]]; then
  export HUGGING_FACE_HUB_TOKEN="$HF_TOKEN"
fi

echo "镜像: $IMAGE"
echo "模型: $MODEL_ID  |  OpenAI model 字段: $SERVED"
echo "监听: http://127.0.0.1:${PORT}/v1/embeddings"

ENV_ARGS=(-e HF_HUB_ENABLE_HF_TRANSFER=1)
if [[ -n "${HF_TOKEN:-}" ]]; then
  ENV_ARGS+=(-e "HF_TOKEN=${HF_TOKEN}" -e "HUGGING_FACE_HUB_TOKEN=${HF_TOKEN}")
fi

podman run -d --name "$NAME" \
  -p "${PORT}:80" \
  -v "${VOL}:/data" \
  "${ENV_ARGS[@]}" \
  "$IMAGE" \
  "${PASS_ARGS[@]}"

echo "已启动。首次拉模型可能较久，查看: podman logs -f $NAME"
echo "试算: curl -sS http://127.0.0.1:${PORT}/v1/embeddings -H 'Content-Type: application/json' -d '{\"model\":\"'\"$SERVED\"'\",\"input\":\"test\"}'"
