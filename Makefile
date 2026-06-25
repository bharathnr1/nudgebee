# Root orchestration Makefile for the nudgebee monorepo.
#
# Dispatches common targets across every service. Each service keeps its own
# Makefile; this file just fans out to them via `$(MAKE) -C <dir> <target>`.
# CI calls each service's commands directly and does NOT depend on this file,
# so adding it changes no existing build behavior.
#
# Each target requires the corresponding service toolchains to be installed
# (Go + golangci-lint, Poetry/uv, Node). Run `make help` to list targets.

GO_SERVICES := \
	api-server/services \
	ticket-server \
	runbook-server \
	collector-server/cloud-collector \
	collector-server/k8s-collector/relay-server \
	llm/code-analysis \
	llm/llm-server

PY_SERVICES := \
	ml-k8s-server \
	llm/rag-server \
	collector-server/k8s-collector/app \
	notifications-server \
	llm/benchmark

TS_SERVICES := app
E2E_SERVICES := app-e2e-tests

# Only services whose Makefile defines the target participate in each fan-out.
FMT_SERVICES  := $(GO_SERVICES) $(PY_SERVICES) $(TS_SERVICES)
LINT_SERVICES := $(GO_SERVICES) $(PY_SERVICES) $(TS_SERVICES)
TEST_SERVICES := $(GO_SERVICES) $(PY_SERVICES) $(TS_SERVICES) $(E2E_SERVICES)

.PHONY: help fmt lint test validate

help: ## Show available targets
	@echo "Nudgebee monorepo — root targets (fan out to every service):"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-9s\033[0m %s\n", $$1, $$2}'

fmt: ## Format code in every service
	@for d in $(FMT_SERVICES); do echo "==> fmt $$d"; $(MAKE) -C $$d fmt || exit 1; done

lint: ## Lint every service
	@for d in $(LINT_SERVICES); do echo "==> lint $$d"; $(MAKE) -C $$d lint || exit 1; done

test: ## Run tests in every service
	@for d in $(TEST_SERVICES); do echo "==> test $$d"; $(MAKE) -C $$d test || exit 1; done

validate: lint test ## Lint then test every service
