# 多阶段构建，优化镜像大小
# 阶段1：构建阶段
FROM golang:1.21-alpine AS builder

# 设置工作目录
WORKDIR /app

# 安装必要的工具
RUN apk add --no-cache git gcc musl-dev sqlite-dev

# 复制go mod文件
COPY go.mod go.sum ./

# 下载依赖
RUN go mod download

# 复制源代码
COPY . .

# 构建应用
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -ldflags '-extldflags \"-static\"' -o tshell-server ./main.go

# 阶段2：运行阶段
FROM alpine:latest

# 安装运行时依赖
RUN apk --no-cache add ca-certificates tzdata

# 设置时区
RUN cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && echo \"Asia/Shanghai\" > /etc/timezone

# 创建非root用户
RUN addgroup -g 1001 -S tshell && \\n    adduser -S -D -H -u 1001 -h /app -s /sbin/nologin -G tshell -g tshell tshell

# 设置工作目录
WORKDIR /app

# 复制构建的二进制文件
COPY --from=builder /app/tshell-server .
COPY --from=builder /app/config.yaml .

# 创建必要的目录
RUN mkdir -p logs dist/static && \\n    chown -R tshell:tshell /app

# 切换到非root用户
USER tshell

# 暴露端口
EXPOSE 8089

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \\n    CMD wget --no-verbose --tries=1 --spider http://localhost:8089/api/dashboard/stats || exit 1

# 启动命令
ENTRYPOINT [\"./tshell-server\"]
CMD [\"-p\", \"8089\"]