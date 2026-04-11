SHELL := /bin/bash
HELM_CMD = sudo helm --kube-context kind-chat
KUBECTL_CMD = sudo kubectl --context kind-chat -n chat

.PHONY: deploy upgrade status pods logs-gateway cassandra-schema api gateway frontend test message-worker test-gateway test-message-worker router test-router test-e2e build-gateway build-message-worker build-router build-api build-frontend build-all cloud-build-gateway cloud-build-message-worker cloud-build-router cloud-build-api cloud-build-frontend cloud-build-all cloud-deploy cloud-upgrade deploy-remote

# First-time install (build images, load into Kind, deploy via Helm)
deploy: build-all
	$(HELM_CMD) install chat k8s/chart/ --namespace chat --create-namespace

# Upgrade after chart changes (rebuild images, load into Kind, upgrade Helm)
upgrade: build-all
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

# Run the Python API server locally (port 8000 reserved by Kind NodePort)
api:
	source .venv/bin/activate && uvicorn app.main:app --port 8003

# Run the Go gateway locally
gateway:
	cd gateway && JWT_SECRET=change-me-in-prod KAFKA_BROKERS=localhost:9092,localhost:9093,localhost:9094 GATEWAY_PORT=8002 GRPC_PORT=9002 GATEWAY_ID=gateway-0 REDIS_RATELIMIT_ADDR=localhost:6381 go run main.go

# Run the frontend dev server
frontend:
	cd frontend && npm run dev

# Run Python integration tests
test:
	source .venv/bin/activate && python -m pytest tests/ -v

# Run the Message Worker (Go)
message-worker:
	cd message-worker && KAFKA_BROKERS=localhost:9092,localhost:9093,localhost:9094 REDIS_CACHE_ADDR=localhost:6380 go run main.go

# Run the Router (Go)
router:
	cd router && KAFKA_BROKERS=localhost:9092,localhost:9093,localhost:9094 REDIS_ADDR=localhost:6379 PG_DSN="postgres://chat:chat_secret@localhost:5432/chat_db?sslmode=disable" GATEWAY_ADDRS="gateway-0=localhost:9002" go run main.go

# Run Go gateway integration tests (requires K8s cluster with Kafka)
test-gateway:
	cd gateway && go test -v -count=1 -timeout 120s .

# Run Go message-worker integration tests (requires K8s cluster)
test-message-worker:
	cd message-worker && go test -v -count=1 -timeout 120s .

# Run Go router integration tests (requires K8s cluster + Redis)
test-router:
	cd router && go test -v -count=1 -timeout 120s .

# Run E2E cross-gateway delivery test (requires K8s cluster + Redis)
test-e2e:
	cd e2e && go test -v -count=1 -timeout 120s .

# Docker build + load into Kind
build-gateway:
	sudo docker build -f gateway/Dockerfile . -t chat-gateway:latest
	sudo kind load docker-image chat-gateway:latest --name chat

build-message-worker:
	sudo docker build -f message-worker/Dockerfile . -t chat-message-worker:latest
	sudo kind load docker-image chat-message-worker:latest --name chat

build-router:
	sudo docker build -f router/Dockerfile . -t chat-router:latest
	sudo kind load docker-image chat-router:latest --name chat

build-all: build-gateway build-message-worker build-router build-api build-frontend

build-api:
	sudo docker build -f app/Dockerfile . -t chat-api:latest
	sudo kind load docker-image chat-api:latest --name chat

build-frontend:
	sudo docker build -f frontend/Dockerfile . -t chat-frontend:latest
	sudo kind load docker-image chat-frontend:latest --name chat

# --- Cloud (Oracle ARM64 + OCIR) ---
# Set these before running cloud targets:
#   export OCIR_REGION_KEY=iad          (region key, not full name)
#   export OCIR_TENANCY=your-tenancy-namespace
OCIR_PREFIX = $(OCIR_REGION_KEY).ocir.io/$(OCIR_TENANCY)

cloud-build-gateway:
	docker buildx build --platform linux/arm64 -f gateway/Dockerfile . -t $(OCIR_PREFIX)/chat-gateway:latest --push

cloud-build-message-worker:
	docker buildx build --platform linux/arm64 -f message-worker/Dockerfile . -t $(OCIR_PREFIX)/chat-message-worker:latest --push

cloud-build-router:
	docker buildx build --platform linux/arm64 -f router/Dockerfile . -t $(OCIR_PREFIX)/chat-router:latest --push

cloud-build-api:
	docker buildx build --platform linux/arm64 -f app/Dockerfile . -t $(OCIR_PREFIX)/chat-api:latest --push

cloud-build-frontend:
	docker buildx build --platform linux/arm64 -f frontend/Dockerfile . \
		--build-arg VITE_API_URL=/api \
		--build-arg VITE_WS_URL=wss://$(CLOUD_HOST)/ws \
		-t $(OCIR_PREFIX)/chat-frontend:latest --push

cloud-build-all: cloud-build-gateway cloud-build-message-worker cloud-build-router cloud-build-api cloud-build-frontend

cloud-deploy:
	helm install chat k8s/chart/ -n chat --create-namespace -f k8s/values-cloud.yaml

cloud-upgrade:
	helm upgrade chat k8s/chart/ -n chat -f k8s/values-cloud.yaml

# Deploy to remote VM (Oracle Cloud k3s)
# Usage: make deploy-remote CLOUD_VM=opc@143.47.126.185
CLOUD_VM ?= opc@143.47.126.185
CLOUD_IMAGES = chat-gateway chat-message-worker chat-router chat-api chat-frontend

deploy-remote: cloud-build-all
	scp -r k8s/chart k8s/values-cloud.yaml $(CLOUD_VM):~/
	ssh $(CLOUD_VM) 'for img in $(CLOUD_IMAGES); do sudo crictl rmi $(OCIR_PREFIX)/$$img:latest 2>/dev/null; done'
	ssh $(CLOUD_VM) 'sudo KUBECONFIG=/etc/rancher/k3s/k3s.yaml helm upgrade chat ~/chart/ -n chat -f ~/values-cloud.yaml'
	ssh $(CLOUD_VM) 'sudo kubectl -n chat rollout restart deployment api frontend && sudo kubectl -n chat rollout restart statefulset gateway && sudo kubectl -n chat delete pod -l app=message-worker -l app=router 2>/dev/null; sudo kubectl -n chat delete pod -l app=message-worker && sudo kubectl -n chat delete pod -l app=router'
