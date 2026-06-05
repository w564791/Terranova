# -- Stage 1: Build --
FROM node:22-alpine AS builder
WORKDIR /app

# Vite 打 monaco-vscode-api (~14MB main chunk) 需要更大 Node 堆。
# 默认 1.5GB 必 OOM。给 4GB 留余量。
# 注意:Docker Desktop 默认 builder 内存 2GB,需要在 Settings > Resources 提到 ≥6GB。
ENV NODE_OPTIONS="--max-old-space-size=4096"

COPY package.json package-lock.json* ./
RUN npm ci --no-audit
COPY . .
RUN npx vite build

# -- Stage 2: Runtime --
FROM nginx:1.27-alpine
COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
