# Build stage
FROM golang:1.25-bookworm AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    gcc \
    libc6-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy go mod files
COPY porcupin/go.mod porcupin/go.sum ./
RUN go mod download

# Copy source
COPY porcupin/ ./

# Build headless binary
RUN CGO_ENABLED=1 go build -o /porcupin-server ./cmd/headless

# Runtime stage
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    tzdata \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user
RUN useradd -u 1000 -m porcupin
USER porcupin

WORKDIR /home/porcupin

# Copy binary
COPY --from=builder /porcupin-server /usr/local/bin/porcupin-server

# Data volume
VOLUME /home/porcupin/.porcupin

# Default command
ENTRYPOINT ["/usr/local/bin/porcupin-server"]
CMD ["--data", "/home/porcupin/.porcupin"]
