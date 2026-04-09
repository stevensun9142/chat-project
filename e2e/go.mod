module github.com/stevensun/chat-project/e2e

go 1.22.2

require (
	github.com/golang-jwt/jwt/v5 v5.2.1
	github.com/gorilla/websocket v1.5.3
	github.com/lib/pq v1.12.3
	github.com/redis/go-redis/v9 v9.5.1
	github.com/stevensun/chat-project/gateway v0.0.0
	github.com/stevensun/chat-project/proto v0.0.0
	github.com/stevensun/chat-project/router v0.0.0
	google.golang.org/grpc v1.65.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/klauspost/compress v1.15.9 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
	github.com/segmentio/kafka-go v0.4.47 // indirect
	golang.org/x/net v0.25.0 // indirect
	golang.org/x/sys v0.20.0 // indirect
	golang.org/x/text v0.15.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240528184218-531527333157 // indirect
	google.golang.org/protobuf v1.34.2 // indirect
)

replace (
	github.com/stevensun/chat-project/gateway => ../gateway
	github.com/stevensun/chat-project/proto => ../proto
	github.com/stevensun/chat-project/router => ../router
)
