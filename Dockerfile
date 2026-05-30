# traffic-ai 控制面 / 数据面统一镜像
# 构建上下文须为仓库根目录，与 .cnb.yml 一致：
#   docker build -f Dockerfile --build-arg BUILD_TARGET=control -t traffic-ai-control .
#   docker build -f Dockerfile --build-arg BUILD_TARGET=gateway  -t traffic-ai-gateway  .

ARG GO_VERSION=1.23-alpine
FROM golang:${GO_VERSION} AS builder

ARG BUILD_TARGET=control

RUN apk add --no-cache git ca-certificates
WORKDIR /src

COPY go.mod go.sum ./
ENV GOTOOLCHAIN=auto
ENV GOPROXY=https://goproxy.cn,direct
RUN go mod download

COPY . .

RUN if [ "$BUILD_TARGET" = "control" ]; then \
      go build -ldflags="-s -w" -o /out/app ./cmd/control ; \
    elif [ "$BUILD_TARGET" = "gateway" ]; then \
      go build -ldflags="-s -w" -o /out/app ./cmd/gateway ; \
    else \
      echo "unknown BUILD_TARGET: $BUILD_TARGET (expected control|gateway)" >&2; exit 1; \
    fi

FROM alpine:3.20 AS production

ARG BUILD_TARGET=control

RUN apk add --no-cache ca-certificates tzdata curl
ENV TZ=Asia/Shanghai

WORKDIR /app
COPY --from=builder /out/app ./bin/app
COPY configs/config.prod.yaml.example ./configs/config.yaml

ENV BUILD_TARGET=${BUILD_TARGET}

EXPOSE 8080 8083 8081

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
  CMD if [ "$BUILD_TARGET" = "gateway" ]; then curl -sf "http://127.0.0.1:8081/healthz" >/dev/null; else curl -sf "http://127.0.0.1:8080/healthz" >/dev/null; fi

CMD ["./bin/app", "-config", "./configs/config.yaml"]
