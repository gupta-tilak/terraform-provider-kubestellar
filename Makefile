# Terraform Provider KubeStellar Makefile

HOSTNAME=registry.terraform.io
NAMESPACE=kubestellar
NAME=kubestellar
BINARY=terraform-provider-${NAME}
VERSION=0.1.0
OS_ARCH=$(shell go env GOOS)_$(shell go env GOARCH)

default: install

# Build the provider
build:
	go build -o ${BINARY}

# Install the provider locally for testing
install: build
	mkdir -p ~/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/${VERSION}/${OS_ARCH}
	mv ${BINARY} ~/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}/${VERSION}/${OS_ARCH}

# Run unit tests
test:
	go test -v -cover -timeout=120s -parallel=4 ./...

# Run acceptance tests (requires running Kubernetes cluster)
testacc:
	TF_ACC=1 go test -v -cover -timeout 120m ./...

# Generate documentation
generate:
	go generate ./...

# Format code
fmt:
	go fmt ./...
	terraform fmt -recursive ./examples/

# Lint code
lint:
	golangci-lint run ./...

# Clean build artifacts
clean:
	rm -f ${BINARY}
	rm -rf ~/.terraform.d/plugins/${HOSTNAME}/${NAMESPACE}/${NAME}

# Run kind cluster for testing
kind-create:
	kind create cluster --name kubestellar-test
	kubectl apply -f ./test/crds/

kind-delete:
	kind delete cluster --name kubestellar-test

# Install CRDs for testing
install-crds:
	kubectl apply -f ./test/crds/

.PHONY: build install test testacc generate fmt lint clean kind-create kind-delete install-crds
