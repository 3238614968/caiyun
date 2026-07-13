FROM golang:1.25.12-alpine AS builder

WORKDIR /app
RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download
COPY . .

ARG TARGETARCH=amd64
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_TIME=unknown
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath \
    -ldflags "-s -w -X caiyun/internal/version.Version=${VERSION} -X caiyun/internal/version.Commit=${COMMIT} -X caiyun/internal/version.BuildTime=${BUILD_TIME}" \
    -o /out/caiyun ./cmd/caiyun

FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata wget \
    && addgroup -S app \
    && adduser -S -G app app
WORKDIR /app
COPY --from=builder --chown=app:app /out/caiyun /usr/local/bin/caiyun

# Containers log structured events to stdout for collection by the runtime.
# Redis is also the only safe rate-limit backend for multi-replica deployments.
ENV LOG_FILE_PATH="" \
    LOG_JSON_FORMAT="true" \
    RATE_LIMIT_BACKEND="redis" \
    WS_EVENT_CHANNEL="caiyun:ws:events" \
    WS_MESSAGE_TTL="24h" \
    WS_ACK_TIMEOUT="5s" \
    WS_MAX_RETRIES="3"

USER app
EXPOSE 8080 8081
ENTRYPOINT ["caiyun"]
CMD ["api"]
