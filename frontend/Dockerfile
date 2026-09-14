# 契约文件（src/api/generated）必须在宿主机/CI 先用仓库根的 python 脚本生成
# （scripts/generate-api-client-types.py），构建上下文里直接 COPY 进来；
# 容器内没有 python，也无法 `cd ..` 回到仓库根，因此绕过 prebuild 钩子。
FROM node:20-alpine AS builder

WORKDIR /app

COPY package*.json ./
RUN npm ci

COPY . .

RUN npm run build --ignore-scripts && npm run bundle:budget

FROM nginxinc/nginx-unprivileged:1.27-alpine

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /app/dist /usr/share/nginx/html

EXPOSE 8080

CMD ["nginx", "-g", "daemon off;"]
