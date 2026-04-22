# syntax=docker/dockerfile:1

# Stage 1: Build frontend
FROM node:22-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Stage 2: Build Go binary
FROM golang:1.25-alpine AS go-builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend-builder /app/frontend/dist ./frontend/dist
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /app/vitra ./cmd/vitra

# Stage 3: Final minimal image
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=go-builder /app/vitra /vitra
EXPOSE 8080
ENV PORT=8080
USER nonroot:nonroot
ENTRYPOINT ["/vitra"]
