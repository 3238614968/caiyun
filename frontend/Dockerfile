# Generate the ignored frontend contract inside the Docker build. The build
# context is the repository root so a clean checkout does not depend on a
# developer/CI host having Python or a stale generated file.
FROM python:3.13-alpine AS contracts

WORKDIR /src
COPY backend ./backend
COPY scripts ./scripts

RUN python scripts/generate-openapi-local.py \
 && python scripts/generate-asyncapi-local.py \
 && python scripts/generate-api-client-types.py

FROM node:20-alpine AS builder

WORKDIR /app

COPY frontend/package*.json ./
RUN npm ci

COPY frontend/ .
COPY --from=contracts /src/frontend/src/api/generated ./src/api/generated

RUN npm run build --ignore-scripts && npm run bundle:budget

FROM nginxinc/nginx-unprivileged:1.27-alpine

COPY frontend/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /app/dist /usr/share/nginx/html

EXPOSE 8080

CMD ["nginx", "-g", "daemon off;"]
