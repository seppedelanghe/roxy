FROM golang:1.26-bookworm AS builder
RUN apt-get update && apt-get install -y --no-install-recommends \
    libraw-dev libvips-dev build-essential pkg-config \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=1 go build -tags vips -o /out/roxy ./cmd/roxy

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    libraw23 libvips42 ca-certificates \
    && rm -rf /var/lib/apt/lists/*
COPY --from=builder /out/roxy /usr/local/bin/roxy
EXPOSE 8080
ENTRYPOINT ["roxy"]
