# 多阶段构建
# 阶段1：构建阶段
FROM node:18-alpine AS builder

# 设置工作目录
WORKDIR /app

# 复制package.json和package-lock.json
COPY package*.json ./

# 安装依赖
RUN npm ci --only=production --silent

# 复制源代码
COPY . .

# 构建应用
RUN npm run build

# 阶段2：生产阶段
FROM nginx:1.25-alpine

# 安装必要工具
RUN apk --no-cache add curl

# 复制构建产物
COPY --from=builder /app/dist /usr/share/nginx/html

# 复制nginx配置
COPY nginx.conf /etc/nginx/conf.d/default.conf

# 创建nginx用户和组（如果不存在）
RUN addgroup -g 101 -S nginx || true && \\n    adduser -S -D -H -u 101 -h /var/cache/nginx -s /sbin/nologin -G nginx -g nginx nginx || true

# 设置权限
RUN chown -R nginx:nginx /usr/share/nginx/html && \\n    chmod -R 755 /usr/share/nginx/html

# 暴露端口
EXPOSE 80

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \\n    CMD curl -f http://localhost/ || exit 1

# 启动nginx
CMD [\"nginx\", \"-g\", \"daemon off;\"]