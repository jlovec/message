# syntax=docker/dockerfile:1.7
ARG GO_IMAGE=golang:1.23-bookworm

FROM ${GO_IMAGE} AS build
ARG GOPROXY=https://goproxy.cn,direct
ARG GOSUMDB=off
ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64 \
    GOPROXY=${GOPROXY} \
    GOSUMDB=${GOSUMDB}
WORKDIR /src

COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN go build -o /out/smsforwarder-webhook ./cmd/server && \
    go build -o /out/smsforwarder-mock ./cmd/smsforwarder-mock

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=build /out/smsforwarder-webhook /app/smsforwarder-webhook
COPY --from=build /out/smsforwarder-mock /app/smsforwarder-mock
ENV HOST=0.0.0.0 \
    PORT=8080
EXPOSE 8080
ENTRYPOINT ["/app/smsforwarder-webhook"]
