# Kubernetes Code-Generation Helpers

This directory contains scripts that simplify generation of Kubernetes-style
helper code (deep-copies, clientsets, listers, informers, OpenAPI, …) for Go
projects.

---

## universal-codegen.sh

A portable wrapper around `k8s.io/code-generator`.

### Quick start
```bash
# From project root
API_DIR=internal/k8s/apis ./hack/universal-codegen.sh
```

> ⚠️  **boilerplate.go.txt обязателен** — файл с лицензионным заголовком должен
> находиться рядом с `universal-codegen.sh` (в директории `hack/`). Скрипт
> передаёт его в `code-generator` / `openapi-gen` через `--go-header-file`. Если
> файла нет, генерация упадёт.

### Environment variables / flags
| Variable | Required | Default | Description |
| -------- | -------- | ------- | ----------- |
| `API_DIR` | yes | – | Directory that holds `group/version` API packages (relative to repo root). |
| `MODULE_PATH` | no | autodetect via `go list -m` | Go module import path. |
| `OUTPUT_PKG` | no | `$MODULE_PATH/pkg/k8s` | Import path where generated client code will live. |
| `OUTPUT_DIR` | no | `$(pwd)/pkg/k8s` | Filesystem directory for generated code. |
| `CODEGEN_VERSION` | no | `v0.33.2` | Version of `k8s.io/code-generator` to use. |
| `SKIP_OPENAPI` | no | unset | When set to any value, skips OpenAPI generation step. |

### Typical workflow
1. Ensure your API types live under `internal/.../apis/<group>/<version>`.  
   Each version directory must have `doc.go`, `types.go`, etc.
2. Export variables and run the script:
   ```bash
   API_DIR=internal/apis \
   OUTPUT_PKG=github.com/company/project/pkg/k8s \
   ./hack/universal-codegen.sh
   ```
3. `go vet ./... && go test ./...` – verify everything builds.
4. Commit the newly generated code.

### FAQ
* **Q:** `kube_codegen.sh not found`  
  **A:** Run `go mod download k8s.io/code-generator@<version>` or adjust
  `CODEGEN_VERSION`.

* **Q:** How to change Kubernetes version?  
  **A:** Pin matching versions of `k8s.io/client-go`, `k8s.io/api`,
  `k8s.io/apimachinery`, *and* `k8s.io/code-generator` in `go.mod`, then rerun
  the script.

* **Q:** Can I skip OpenAPI?  
  **A:** If `openapi-gen` binary is not found in `PATH`, the script will
  silently skip this step.

---