# Stage 1: Build
FROM golang:1.24-alpine3.22 AS builder

# Install build deps (git is often needed for Go modules)
RUN apk add --no-cache git

WORKDIR /app

# Pre-copy go.mod and go.sum to leverage Docker layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build with optimizations
RUN go build -ldflags="-s -w" -o main main.go

# Stage 2: Runtime
FROM alpine:3.22

WORKDIR /app
COPY --from=builder /app/main .

EXPOSE 8080

# Run as non-root for security
RUN adduser -D appuser
USER appuser

CMD ["/app/main"]

