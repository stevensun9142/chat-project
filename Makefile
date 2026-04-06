HELM_CMD = sudo helm --kube-context kind-chat
KUBECTL_CMD = sudo kubectl --context kind-chat -n chat

.PHONY: deploy upgrade status pods logs-gateway cassandra-schema

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
	cd gateway && JWT_SECRET=change-me-in-prod KAFKA_BROKERS=localhost:9092,localhost:9093,localhost:9094 go run main.go

# Run the frontend dev server
frontend:
	cd frontend && npm run dev

# Run tests
test:
	source .venv/bin/activate && python -m pytest tests/ -v
