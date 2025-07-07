#!/usr/bin/env bash

# Copyright 2025 The Netguard Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o errexit
set -o nounset
set -o pipefail

# Determine script root directory
SCRIPT_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "${SCRIPT_ROOT}"

echo "Generating code using kube_codegen.sh for Kubernetes-style API types..."
echo "Script root: ${SCRIPT_ROOT}"

# Use the known path to kube_codegen.sh from Go module cache
CODEGEN_PKG=$(go env GOPATH)/pkg/mod/k8s.io/code-generator@v0.33.2

if [[ ! -f "${CODEGEN_PKG}/kube_codegen.sh" ]]; then
    echo "Error: kube_codegen.sh not found at ${CODEGEN_PKG}/kube_codegen.sh"
    echo "Please check your Go module cache or install code-generator"
    exit 1
fi

echo "Using code-generator script: ${CODEGEN_PKG}/kube_codegen.sh"

# Source the kube_codegen.sh script
source "${CODEGEN_PKG}/kube_codegen.sh"

echo "=== deepcopy / client / lister / informer ==="
kube::codegen::gen_helpers \
    --boilerplate "${SCRIPT_ROOT}/hack/k8s/boilerplate.go.txt" \
    "${SCRIPT_ROOT}/internal/k8s/apis"

kube::codegen::gen_client \
    --with-watch \
    --output-dir "${SCRIPT_ROOT}/pkg/k8s" \
    --output-pkg "netguard-pg-backend/pkg/k8s" \
    --boilerplate "${SCRIPT_ROOT}/hack/k8s/boilerplate.go.txt" \
    "${SCRIPT_ROOT}/internal/k8s/apis"

echo "=== OpenAPI definitions ==="
openapi-gen \
  --output-dir       "${SCRIPT_ROOT}/internal/k8s/apis/netguard/v1beta1" \
  --output-pkg       "netguard-pg-backend/internal/k8s/apis/netguard/v1beta1" \
  --output-file      zz_generated.openapi.go \
  --go-header-file   "${SCRIPT_ROOT}/hack/k8s/boilerplate.go.txt" \
  netguard-pg-backend/internal/k8s/apis/netguard/v1beta1 \
  k8s.io/apimachinery/pkg/apis/meta/v1 \
  k8s.io/apimachinery/pkg/version \
  k8s.io/apimachinery/pkg/runtime \
  k8s.io/apimachinery/pkg/runtime/schema \
  k8s.io/apimachinery/pkg/api/resource

echo ">>> Code-gen finished" 