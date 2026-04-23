CONTROLLER_GEN ?= controller-gen
IMG ?= extensions:latest

.PHONY: generate
generate: ## Regenerate deepcopy methods.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: manifests
manifests: ## Generate CRD and RBAC manifests from kubebuilder markers.
	$(CONTROLLER_GEN) crd rbac:roleName=manager-role paths="./..." \
		output:crd:artifacts:config=config/crd \
		output:rbac:artifacts:config=config/rbac

.PHONY: build
build: generate ## Build the manager binary.
	go build -o bin/manager ./cmd/...

.PHONY: test
test: generate ## Run unit tests.
	go test ./... -v -count=1

.PHONY: vet
vet: ## Run go vet.
	go vet ./...

.PHONY: vendor
vendor: ## Tidy and vendor dependencies.
	go mod tidy && go mod vendor

.PHONY: docker-build
docker-build: ## Build the controller image.
	docker build -t $(IMG) .

.PHONY: install
install: manifests ## Install CRDs into the current cluster.
	kubectl apply -f config/crd

.PHONY: uninstall
uninstall: ## Remove CRDs from the current cluster.
	kubectl delete -f config/crd --ignore-not-found

.PHONY: catalog
catalog: ## Apply all catalog PlatformExtension manifests to the current cluster.
	kubectl apply -f config/catalog/
