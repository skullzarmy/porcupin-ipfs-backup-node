# Build stage
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git gcc musl-dev

WORKDIR /app

# Copy go mod files
COPY porcupin/go.mod porcupin/go.sum ./
RUN go mod download

# Copy source
COPY porcupin/ ./

# Build headless binary
RUN CGO_ENABLED=1 go build -o /porcupin-server ./cmd/headless

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN adduser -D -u 1000 porcupin
USER porcupin

WORKDIR /home/porcupin

# Copy binary
COPY --from=builder /porcupin-server /usr/local/bin/porcupin-server

# Data volume
VOLUME /home/porcupin/.porcupin

# Default command
ENTRYPOINT ["/usr/local/bin/porcupin-server"]
CMD ["--data", "/home/porcupin/.porcupin"]
