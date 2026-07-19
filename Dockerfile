# syntax=docker/dockerfile:1

FROM golang:1.23 AS builder
WORKDIR /app
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG BUILD=unknown
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -ldflags "-X main.Version=${VERSION} -X main.Build=${BUILD} -X main.ProjectName=message-delivery" \
  -o /bin/message-delivery ./cmd/main.go

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=builder /bin/message-delivery /usr/bin/message-delivery
COPY message-delivery.example.json /etc/message-delivery/config.json
COPY templates /etc/message-delivery/templates
EXPOSE 8090
ENTRYPOINT ["/usr/bin/message-delivery"]
CMD ["--config", "/etc/message-delivery/config.json"]
