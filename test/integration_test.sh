#!/bin/bash
# =============================================================================
# KubeStellar Terraform Provider - Integration Test Script
# =============================================================================
# This script sets up a test environment using kind clusters and runs
# integration tests for the Terraform provider.
# =============================================================================

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Configuration
KUBESTELLAR_CLUSTER="kubestellar-test"
WEC_CLUSTER_1="wec-test-1"
WEC_CLUSTER_2="wec-test-2"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."
    
    if ! command -v kind &> /dev/null; then
        log_error "kind is not installed. Please install it first."
        exit 1
    fi
    
    if ! command -v kubectl &> /dev/null; then
        log_error "kubectl is not installed. Please install it first."
        exit 1
    fi
    
    if ! command -v terraform &> /dev/null; then
        log_error "terraform is not installed. Please install it first."
        exit 1
    fi
    
    if ! command -v go &> /dev/null; then
        log_error "go is not installed. Please install it first."
        exit 1
    fi
    
    log_info "All prerequisites satisfied."
}

# Create kind clusters
create_clusters() {
    log_info "Creating kind clusters..."
    
    # Create main KubeStellar cluster
    if kind get clusters | grep -q "^${KUBESTELLAR_CLUSTER}$"; then
        log_warn "Cluster ${KUBESTELLAR_CLUSTER} already exists, skipping..."
    else
        log_info "Creating ${KUBESTELLAR_CLUSTER}..."
        kind create cluster --name "${KUBESTELLAR_CLUSTER}" --wait 60s
    fi
    
    # Create WEC clusters
    for cluster in "${WEC_CLUSTER_1}" "${WEC_CLUSTER_2}"; do
        if kind get clusters | grep -q "^${cluster}$"; then
            log_warn "Cluster ${cluster} already exists, skipping..."
        else
            log_info "Creating ${cluster}..."
            kind create cluster --name "${cluster}" --wait 60s
        fi
    done
    
    log_info "All clusters created."
}

# Install KubeStellar CRDs
install_crds() {
    log_info "Installing KubeStellar CRDs..."
    
    kubectl config use-context "kind-${KUBESTELLAR_CLUSTER}"
    
    # Apply test CRDs
    kubectl apply -f "${PROJECT_ROOT}/test/crds/"
    
    # Create kubestellar namespace
    kubectl create namespace kubestellar --dry-run=client -o yaml | kubectl apply -f -
    
    log_info "CRDs installed."
}

# Build the provider
build_provider() {
    log_info "Building the provider..."
    
    cd "${PROJECT_ROOT}"
    go build -o terraform-provider-kubestellar
    
    # Install locally
    make install
    
    log_info "Provider built and installed."
}

# Run unit tests
run_unit_tests() {
    log_info "Running unit tests..."
    
    cd "${PROJECT_ROOT}"
    go test -v -cover ./internal/provider/... -run "^Test[^Acc]"
    
    log_info "Unit tests completed."
}

# Run acceptance tests
run_acceptance_tests() {
    log_info "Running acceptance tests..."
    
    cd "${PROJECT_ROOT}"
    
    # Set up environment for acceptance tests
    export TF_ACC=1
    export KUBECONFIG="${HOME}/.kube/config"
    
    # Run acceptance tests
    go test -v -cover -timeout 120m ./internal/provider/... -run "^TestAcc"
    
    log_info "Acceptance tests completed."
}

# Run Terraform plan/apply verification
run_terraform_tests() {
    log_info "Running Terraform verification tests..."
    
    # Switch to test context
    kubectl config use-context "kind-${KUBESTELLAR_CLUSTER}"
    
    # Create temp directory for test
    TEST_DIR=$(mktemp -d)
    log_info "Test directory: ${TEST_DIR}"
    
    # Copy basic example
    cp -r "${PROJECT_ROOT}/examples/basic/"* "${TEST_DIR}/"
    
    cd "${TEST_DIR}"
    
    # Initialize Terraform
    log_info "Running terraform init..."
    terraform init
    
    # Run plan
    log_info "Running terraform plan..."
    terraform plan -out=tfplan
    
    # Apply (optional - uncomment to run)
    # log_info "Running terraform apply..."
    # terraform apply -auto-approve tfplan
    
    # Cleanup
    # terraform destroy -auto-approve
    
    rm -rf "${TEST_DIR}"
    
    log_info "Terraform verification completed."
}

# Cleanup clusters
cleanup() {
    log_info "Cleaning up..."
    
    for cluster in "${KUBESTELLAR_CLUSTER}" "${WEC_CLUSTER_1}" "${WEC_CLUSTER_2}"; do
        if kind get clusters | grep -q "^${cluster}$"; then
            log_info "Deleting cluster ${cluster}..."
            kind delete cluster --name "${cluster}"
        fi
    done
    
    log_info "Cleanup completed."
}

# Print usage
usage() {
    echo "Usage: $0 [command]"
    echo ""
    echo "Commands:"
    echo "  setup       Create test clusters and install CRDs"
    echo "  build       Build the provider"
    echo "  unit        Run unit tests"
    echo "  acceptance  Run acceptance tests (requires setup)"
    echo "  terraform   Run Terraform verification tests"
    echo "  all         Run all tests"
    echo "  cleanup     Delete test clusters"
    echo ""
}

# Main
main() {
    case "${1:-all}" in
        setup)
            check_prerequisites
            create_clusters
            install_crds
            ;;
        build)
            build_provider
            ;;
        unit)
            run_unit_tests
            ;;
        acceptance)
            run_acceptance_tests
            ;;
        terraform)
            run_terraform_tests
            ;;
        all)
            check_prerequisites
            create_clusters
            install_crds
            build_provider
            run_unit_tests
            run_acceptance_tests
            run_terraform_tests
            ;;
        cleanup)
            cleanup
            ;;
        *)
            usage
            exit 1
            ;;
    esac
}

main "$@"
