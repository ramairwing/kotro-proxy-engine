# Multi-stage build: Go mock upstream + Rust proxy, minimal Alpine runtime.
# Both runtimes pre-fetch dependencies in isolated layers for cache efficiency.

# ==========================================
# STAGE 1: Go dependency fetch & mock build
# ==========================================
FROM golang:1.24-alpine AS go-builder

RUN apk add --no-cache ca-certificates git

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/mockupstream ./cmd/mockupstream

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/mock-upstream ./cmd/mockupstream

# ==========================================
# STAGE 2: Rust dependency fetch & build
# ==========================================
FROM rust:1-alpine AS rust-builder

RUN apk add --no-cache musl-dev gcc pkgconfig perl make

WORKDIR /app/rust

# Workspace manifest + lockfile for dependency layer caching.
COPY rust/Cargo.toml rust/Cargo.lock ./
COPY rust/korto-proxy/Cargo.toml korto-proxy/Cargo.toml

# Dummy crate forces registry fetch without copying real sources.
RUN mkdir -p korto-proxy/src \
    && printf 'pub fn dummy() {}\n' > korto-proxy/src/lib.rs \
    && printf 'fn main() {}\n' > korto-proxy/src/main.rs

ENV CARGO_TARGET_DIR=/app/rust/target
RUN cargo build --release -p korto-proxy

# Replace stub with real sources and rebuild application code only.
RUN rm -rf korto-proxy/src
COPY rust/korto-proxy/src korto-proxy/src
RUN touch korto-proxy/src/main.rs korto-proxy/src/lib.rs \
    && cargo build --release -p korto-proxy

# ==========================================
# STAGE 3: Minimal production runtime
# ==========================================
FROM alpine:3.21 AS production

RUN apk add --no-cache ca-certificates curl

WORKDIR /root

COPY --from=go-builder /bin/mock-upstream .
COPY --from=rust-builder /app/rust/target/release/korto-proxy .

RUN mkdir -p /root/data

EXPOSE 8080 9000

# Default: Rust high-performance proxy (override in Compose for mock-upstream).
CMD ["./korto-proxy"]
