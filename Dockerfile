# Build stage
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum* ./
RUN go mod download || true

# Copy source and build
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o server ./cmd/server

# Final stage
FROM alpine:latest

# Install dependencies and create user in single RUN
RUN apk --no-cache add ca-certificates wget \
    && adduser -D -u 1000 appuser

WORKDIR /home/appuser
COPY --from=builder --chown=appuser:appuser /app/server .
RUN chmod +x server

USER appuser
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

CMD ["./server"]