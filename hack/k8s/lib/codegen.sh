#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

# This script is a simplified version of the official k8s.io/code-generator/generate-groups.sh

SCRIPT_ROOT=$(dirname "${BASH_SOURCE[0]}")/../../..
CODEGEN_PKG=${CODEGEN_PKG:-$(cd "${SCRIPT_ROOT}"; ls -d -1 ./vendor/k8s.io/code-generator 2>/dev/null || echo ../code-generator)}

# Ensure all generators are installed
go install k8s.io/code-generator/cmd/conversion-gen
go install k8s.io/code-generator/cmd/deepcopy-gen
go install k8s.io/code-generator/cmd/client-gen
go install k8s.io/code-generator/cmd/lister-gen
go install k8s.io/code-generator/cmd/informer-gen
go install k8s.io/kube-openapi/cmd/openapi-gen

# Call the official generate-groups.sh script with our parameters
bash "${CODEGEN_PKG}/generate-groups.sh" all \
  netguard-pg-backend/pkg/k8s/clientset \
  netguard-pg-backend/pkg/k8s/apis \
  "netguard:v1beta1" \
  --go-header-file "${SCRIPT_ROOT}/hack/k8s/boilerplate.go.txt" 