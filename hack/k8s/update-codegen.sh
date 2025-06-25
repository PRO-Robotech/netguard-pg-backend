#!/usr/bin/env bash

# Copyright 2024 The Netguard Authors.
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

SCRIPT_ROOT=$(dirname "${BASH_SOURCE[0]}")/../..
CODEGEN_PKG=${CODEGEN_PKG:-$(cd "${SCRIPT_ROOT}"; ls -d -1 ./vendor/k8s.io/code-generator 2>/dev/null || echo ../code-generator)}

# Generate deepcopy functions
go run k8s.io/code-generator/cmd/deepcopy-gen \
  --bounding-dirs=netguard-pg-backend/internal/k8s/apis/netguard/v1beta1 \
  --output-file-base=zz_generated.deepcopy \
  --go-header-file="${SCRIPT_ROOT}/hack/k8s/boilerplate.go.txt" \
  netguard-pg-backend/internal/k8s/apis/netguard/v1beta1

# Generate clientset
go run k8s.io/code-generator/cmd/client-gen \
  --clientset-name=versioned \
  --input-base="" \
  --input=netguard-pg-backend/internal/k8s/apis/netguard/v1beta1 \
  --output-package=netguard-pg-backend/pkg/k8s/clientset \
  --go-header-file="${SCRIPT_ROOT}/hack/k8s/boilerplate.go.txt"

# Generate listers
go run k8s.io/code-generator/cmd/lister-gen \
  --input-dirs=netguard-pg-backend/internal/k8s/apis/netguard/v1beta1 \
  --output-package=netguard-pg-backend/pkg/k8s/listers \
  --go-header-file="${SCRIPT_ROOT}/hack/k8s/boilerplate.go.txt"

# Generate informers
go run k8s.io/code-generator/cmd/informer-gen \
  --input-dirs=netguard-pg-backend/internal/k8s/apis/netguard/v1beta1 \
  --versioned-clientset-package=netguard-pg-backend/pkg/k8s/clientset/versioned \
  --listers-package=netguard-pg-backend/pkg/k8s/listers \
  --output-package=netguard-pg-backend/pkg/k8s/informers \
  --go-header-file="${SCRIPT_ROOT}/hack/k8s/boilerplate.go.txt"

echo "Code generation completed successfully" 