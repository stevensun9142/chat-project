FROM golang:1.22 AS builder
WORKDIR /src
COPY message-worker/go.mod message-worker/go.sum ./
RUN go mod download
COPY message-worker/ .
RUN CGO_ENABLED=0 GOOS=linux go build -o /message-worker .

FROM gcr.io/distroless/static-debian12
COPY --from=builder /message-worker /message-worker
ENTRYPOINT ["/message-worker"]
