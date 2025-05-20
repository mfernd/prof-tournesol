FROM rust:1.87-alpine AS builder
WORKDIR /app

RUN apk update && apk upgrade --no-cache
RUN apk add --no-cache pkgconfig openssl-dev build-base

RUN mkdir -p src/ \
 && echo "fn main() {}" > ./src/main.rs
COPY ./Cargo.* ./
RUN cargo fetch

COPY src/ src/
RUN cargo build --release

FROM alpine:3
WORKDIR /app

RUN apk update && apk upgrade --no-cache

COPY --from=builder /app/target/release/gh .

EXPOSE 3000

CMD ["./gh"]
