# Hanzo Agents Control Plane — build, push, deploy
IMAGE     := ghcr.io/hanzoai/agents-control-plane
TAG       := $(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
VERSION   := $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.0.0-dev")
COMMIT    := $(shell git rev-parse HEAD 2>/dev/null || echo "none")
DATE      := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
NAMESPACE := hanzo

.PHONY: dev build push deploy apply status logs clean

## Local development
dev:
	cd web/client && npm run dev

dev-collab:
	cd web/client && npm run dev:collab

## Build frontend only
frontend:
	cd web/client && npm ci && npx vite build

## Build Go binary locally (with embedded UI)
binary: frontend
	CGO_ENABLED=0 go build -tags embedded \
		-ldflags "-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)" \
		-o bin/hanzo-agents-server ./cmd/hanzo-agents-server

## Build Docker image
build:
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg DATE=$(DATE) \
		-t $(IMAGE):$(TAG) \
		-t $(IMAGE):latest \
		.

## Push Docker image to ghcr.io
push: build
	docker push $(IMAGE):$(TAG)
	docker push $(IMAGE):latest

## Apply K8s manifests
apply:
	kubectl apply -f ../../../devops/k8s/services/bot-site/deployment.yaml

## Rolling restart of agents deployment
deploy: push apply
	kubectl rollout restart deployment/agents -n $(NAMESPACE)
	kubectl rollout status deployment/agents -n $(NAMESPACE) --timeout=120s

## Check status
status:
	@echo "=== Deployment ==="
	kubectl get deployment agents -n $(NAMESPACE) -o wide
	@echo ""
	@echo "=== Pods ==="
	kubectl get pods -n $(NAMESPACE) -l app=agents -o wide
	@echo ""
	@echo "=== Service ==="
	kubectl get svc agents -n $(NAMESPACE)
	@echo ""
	@echo "=== Ingress ==="
	kubectl get ingress hanzo-agents-ui -n $(NAMESPACE)

## Tail logs
logs:
	kubectl logs -n $(NAMESPACE) -l app=agents --tail=100 -f

## Clean build artifacts
clean:
	rm -rf bin/ web/client/dist/
