# syntax=docker/dockerfile:1

# Stage 1: Build Go backend
FROM golang:1.26-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /nodeloc-store ./cmd/server

# Stage 2: Final minimal image
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata curl

WORKDIR /app
COPY --from=builder /nodeloc-store .
COPY config.yml.example ./config.yml

RUN mkdir -p ./web/user ./web/admin

ENV TZ=Asia/Shanghai
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/api/health || exit 1

ENTRYPOINT ["./nodeloc-store"]
