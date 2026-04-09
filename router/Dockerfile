FROM golang:1.22 AS builder
WORKDIR /src
COPY proto/ ../proto/
COPY router/go.mod router/go.sum ./
RUN go mod download
COPY router/ .
RUN CGO_ENABLED=0 GOOS=linux go build -o /router .

FROM gcr.io/distroless/static-debian12
COPY --from=builder /router /router
ENTRYPOINT ["/router"]
