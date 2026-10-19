# Tshell 前端镜像（nginx 托管 SPA，并将 /api 反代到服务端）
#
# 构建上下文为 Tshell-web/ 目录：
#   docker build -t tshell-web:latest Tshell-web
#
# 说明：服务端镜像已内嵌前端（//go:embed dist），本镜像用于"nginx 在前"的部署形态
# （见 nginx.conf 的 proxy_pass tshell-server:8089）。

FROM node:20-alpine AS builder
WORKDIR /app
COPY package.json package-lock.json ./
# vite 等构建工具在 devDependencies：不能用 --production，否则 npm run build 找不到 vite
RUN npm ci --legacy-peer-deps
COPY . .
RUN npm run build

FROM nginx:1.25-alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD wget -q --spider http://127.0.0.1/ || exit 1
CMD ["nginx", "-g", "daemon off;"]
