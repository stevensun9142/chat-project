FROM golang:1.22 AS builder
WORKDIR /src
COPY proto/ ../proto/
COPY gateway/go.mod gateway/go.sum ./
RUN go mod download
COPY gateway/ .
RUN CGO_ENABLED=0 GOOS=linux go build -o /gateway .

FROM gcr.io/distroless/static-debian12
COPY --from=builder /gateway /gateway
ENTRYPOINT ["/gateway"]
