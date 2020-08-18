# syntax=docker/dockerfile:1
# Compose image for process split. Deploy replaces IAM and the KMS keeper.
FROM golang:1.22-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG BIN=api
RUN CGO_ENABLED=0 go build -o /out/${BIN} ./cmd/${BIN}

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
WORKDIR /app
ARG BIN=api
COPY --from=build /out/${BIN} /app/service
COPY configs /app/configs
COPY migrations /app/migrations
ENV CONFIG_PATH=/app/configs/app.yaml
EXPOSE 18480 18481
CMD ["/app/service"]
