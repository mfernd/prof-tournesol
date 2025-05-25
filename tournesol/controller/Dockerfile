FROM golang:1.24.2-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o controller


FROM alpine:3.18

# Install wget
RUN apk add --no-cache wget ca-certificates

WORKDIR /app
COPY --from=builder /app/controller /app/controller

ENTRYPOINT ["/app/controller"]
