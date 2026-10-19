# Tshell 服务端镜像（多阶段）
#
# 构建上下文必须是**仓库根目录**——前端源码在 Tshell-web/ 下，需与服务端同处一个上下文：
#   docker build -f Tshell-server/Dockerfile -t tshell:latest .
#
# 阶段划分与 build.sh 的流水线一致：前端构建 → 服务端 CGO 构建 → 精简运行镜像。
# 前端产物 dist/ 不入库（见根 .gitignore），故在镜像内构建，不做占位兜底。

# ---------- 阶段 1：前端 ----------
FROM node:20-alpine AS web
WORKDIR /web
# 先装依赖（利用层缓存）；vite 等在 devDependencies，故不能用 --production
COPY Tshell-web/package.json Tshell-web/package-lock.json ./
RUN npm ci --legacy-peer-deps
COPY Tshell-web/ ./
RUN npm run build

# ---------- 阶段 2：服务端（go-sqlite3 需要 cgo） ----------
FROM golang:1.24-alpine AS builder
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILDTIME=unknown
WORKDIR /src
RUN apk add --no-cache git gcc musl-dev
COPY Tshell-server/go.mod Tshell-server/go.sum ./
RUN go mod download
COPY Tshell-server/ ./
# 前端产物落到 //go:embed dist 期望的位置
RUN rm -rf ./dist && mkdir -p ./dist
COPY --from=web /web/dist/ ./dist/
RUN CGO_ENABLED=1 go build -buildvcs=false -trimpath \
      -ldflags "-s -w -X main.Version=${VERSION} -X main.GitCommit=${COMMIT} -X main.BuildTime=${BUILDTIME}" \
      -o tshell-server ./main.go

# ---------- 阶段 3：运行 ----------
FROM alpine:3.20
# 与 deploy/k8s/tshell.yaml 的 runAsUser/fsGroup 65532 对齐（distroless nonroot 约定）
RUN apk --no-cache add ca-certificates tzdata wget \
 && addgroup -g 65532 -S tshell \
 && adduser -S -D -H -u 65532 -h /app -s /sbin/nologin -G tshell tshell
WORKDIR /app
COPY --from=builder /src/tshell-server ./tshell-server
# 默认配置取自仓库模板：config.yaml 是本机私有文件（不入库，可能含密钥）
COPY Tshell-server/config.example.yaml ./config.yaml
# 独立 `docker run` 时可写；k8s 下根文件系统只读，可写状态须经配置指向 /data 卷
# （database.path / jwt.secret_key_file / logging.audit_log_dir / backup.dir）
RUN mkdir -p logs backups data Downloads && chown -R tshell:tshell /app
USER tshell
EXPOSE 8089
# 探针用 /（公开的 SPA 入口）；/api/* 需鉴权，用它们做健康检查会恒失败
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
  CMD wget -q --spider http://127.0.0.1:8089/ || exit 1
ENTRYPOINT ["./tshell-server"]
CMD ["-p", "8089"]
