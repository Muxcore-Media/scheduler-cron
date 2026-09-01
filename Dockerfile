FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /build/scheduler-cron ./cmd/module

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /build/scheduler-cron /
EXPOSE 9200
HEALTHCHECK --interval=30s --timeout=5s --start-period=3s --retries=3 \
  CMD ["/scheduler-cron", "--health-check"]
ENTRYPOINT ["/scheduler-cron"]
