FROM --platform=$BUILDPLATFORM node:24-alpine3.23@sha256:e44c87c7dd212720eb00469aeac89504ca2be77b8f0d6dbffb30665d5fcc59e1 AS frontend
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run check && npm test && npm run build

FROM --platform=$BUILDPLATFORM golang:1.26-alpine3.23@sha256:6750308561953d451210ed1a8086d36c8c8bdcbf672d38d9d0d424b8a7a0216c AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY . .
COPY --from=frontend /app/frontend/dist ./frontend/dist
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /vitra ./cmd/vitra
RUN mkdir -p /data/vault /data/index

FROM alpine:3.23@sha256:85fe1e81d6758c208f3e1eed4338a1997e19d4be002d4dd32d3100c9a8c010a0
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /vitra /usr/local/bin/vitra
COPY --from=builder --chown=10001:10001 /data /data
USER 10001:10001
WORKDIR /data
ENV VAULT_PATH=/data/vault CHROMEM_PATH=/data/index BIND_ADDRESS=0.0.0.0 PORT=8080
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s CMD wget -q -O /dev/null http://127.0.0.1:${PORT}/healthz || exit 1
ENTRYPOINT ["/usr/local/bin/vitra"]
