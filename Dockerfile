# ==========================================
# Stage 1: Build standalone Go binary
# ==========================================
FROM golang:1.24-alpine3.21 AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates

WORKDIR /build

# Cache dependencies layer
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy application source code
COPY cmd/ cmd/
COPY internal/ internal/
COPY plugins/ plugins/

# Build statically linked standalone Linux executable
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -trimpath \
    -ldflags="-w -s" \
    -o /build/bot \
    ./cmd/bot

# ==========================================
# Stage 2: Minimal, secure runtime image
# ==========================================
FROM alpine:3.21

# Install runtime security certificates and timezone data
RUN apk add --no-cache ca-certificates tzdata

# Create dedicated unprivileged non-root user and group
RUN addgroup -g 10001 -S botgroup && \
    adduser -u 10001 -S botuser -G botgroup -h /app -s /sbin/nologin

WORKDIR /app

# Copy exclusively the compiled standalone binary from builder
COPY --from=builder --chown=botuser:botgroup /build/bot /app/bot

# Ensure executable permissions and directory ownership
RUN chmod 0755 /app/bot && \
    chown -R botuser:botgroup /app

# Switch to non-root user (UID 10001)
USER 10001:10001

# Execute standalone bot binary
ENTRYPOINT ["/app/bot"]
