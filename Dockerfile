# ---------- 构建阶段 ----------
FROM golang:1.22-alpine AS builder

WORKDIR /src

# 先拷贝依赖清单，命中缓存后改代码不必重新下载依赖
COPY go.mod go.sum ./
RUN go mod download

# 前端资源通过 go:embed 打进二进制，构建产物不含 web 目录
COPY . .

# buildx 多架构构建时由 BuildKit 注入；本地普通构建回落到当前 Go 的默认架构
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=1.0.0

RUN CGO_ENABLED=0 \
      GOOS=${TARGETOS:-linux} \
      GOARCH=${TARGETARCH:-$(go env GOARCH)} \
      go build \
      -trimpath \
      -ldflags "-s -w -X main.version=${VERSION}" \
      -o /out/libra .

# ---------- 运行阶段 ----------
FROM alpine:3.20

ARG VERSION=1.0.0
LABEL org.opencontainers.image.title="Libra" \
      org.opencontainers.image.description="Lightweight self-hosted ebook library for NAS" \
      org.opencontainers.image.version="${VERSION}"

# WITH_POPPLER=0 可以去掉 poppler-utils，把镜像压到 ~20MB，
# 代价是 PDF 只能使用内嵌封面图，无法渲染首页。
ARG WITH_POPPLER=1

RUN apk add --no-cache ca-certificates tzdata \
 && if [ "$WITH_POPPLER" = "1" ]; then apk add --no-cache poppler-utils; fi \
 && adduser -D -u 1000 -h /app libra \
 && mkdir -p /data /books \
 && chown -R libra:libra /data /books /app

COPY --from=builder /out/libra /usr/local/bin/libra

ENV LIBRA_ADDR=":8080" \
    LIBRA_DATA_DIR="/data" \
    LIBRA_LIBRARY_ROOT="/books" \
    LIBRA_SCAN_CONCURRENCY="2" \
    LIBRA_SCAN_ON_START="true" \
    TZ="Asia/Shanghai"

USER libra
WORKDIR /app

VOLUME ["/data"]
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null 2>&1 || exit 1

ENTRYPOINT ["/usr/local/bin/libra"]
