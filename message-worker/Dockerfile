FROM golang:1.22 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /message-worker .

FROM gcr.io/distroless/static-debian12
COPY --from=builder /message-worker /message-worker
ENTRYPOINT ["/message-worker"]
