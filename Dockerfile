# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS builder

WORKDIR /app
COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /nodeloc-store ./cmd/server

# Frontend build
FROM node:22-alpine AS frontend-builder
WORKDIR /app
COPY frontend/ ./frontend/
RUN cd frontend && npm install -g pnpm && pnpm install && pnpm build

# Runtime
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /nodeloc-store .
COPY --from=frontend-builder /app/frontend/user/dist ./web/user
COPY --from=frontend-builder /app/frontend/admin/dist ./web/admin
COPY config.yml.example ./config.yml

ENV TZ=Asia/Shanghai
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/health || exit 1

ENTRYPOINT ["./nodeloc-store"]
