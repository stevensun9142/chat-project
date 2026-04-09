HELM_CMD = sudo helm --kube-context kind-chat
KUBECTL_CMD = sudo kubectl --context kind-chat -n chat

.PHONY: deploy upgrade status pods logs-gateway cassandra-schema api gateway frontend test message-worker test-gateway test-message-worker router

# First-time install
deploy:
	$(HELM_CMD) install chat k8s/chart/ --namespace chat --create-namespace

# Upgrade after chart changes
upgrade:
	$(HELM_CMD) upgrade chat k8s/chart/ -n chat

# Show all pods
pods:
	$(KUBECTL_CMD) get pods

# Show all resources
status:
	$(KUBECTL_CMD) get all

# Load Cassandra schema
cassandra-schema:
	$(KUBECTL_CMD) exec -i cassandra-0 -- cqlsh < schema/cassandra/init.cql

# Run the Python API server locally
api:
	source .venv/bin/activate && uvicorn app.main:app --port 8000

# Run the Go gateway locally
gateway:
	cd gateway && JWT_SECRET=change-me-in-prod KAFKA_BROKERS=localhost:9092,localhost:9093,localhost:9094 GATEWAY_PORT=8002 GRPC_PORT=9002 GATEWAY_ID=gateway-0 go run main.go

# Run the frontend dev server
frontend:
	cd frontend && npm run dev

# Run Python integration tests
test:
	source .venv/bin/activate && python -m pytest tests/ -v

# Run the Message Worker (Go)
message-worker:
	cd message-worker && KAFKA_BROKERS=localhost:9092,localhost:9093,localhost:9094 go run main.go

# Run the Router (Go)
router:
	cd router && KAFKA_BROKERS=localhost:9092,localhost:9093,localhost:9094 REDIS_ADDR=localhost:6379 PG_DSN="postgres://chat:chat_secret@localhost:5432/chat_db?sslmode=disable" GATEWAY_ADDRS="gateway-0=localhost:9002" go run main.go

# Run Go gateway integration tests (requires K8s cluster with Kafka)
test-gateway:
	cd gateway && go test -v -count=1 -timeout 120s .

# Run Go message-worker integration tests (requires K8s cluster)
test-message-worker:
	cd message-worker && go test -v -count=1 -timeout 120s .
