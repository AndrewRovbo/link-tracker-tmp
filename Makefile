COVERAGE_FILE ?= coverage.out

# Get all directories in cmd/ as available modules
MODULES := $(notdir $(wildcard cmd/*))

# Help target - display usage information
.PHONY: help
help:
	@echo "Available commands:"
	@echo "  \033[36mmake build\033[0m - Build all modules ($(MODULES))"
	@$(foreach mod,$(MODULES),echo "  \033[36mmake build_$(mod)\033[0m - Build $(mod) module";)
	@echo "  \033[36mmake test\033[0m - Run all tests"
	@echo "  \033[36mmake test-integration\033[0m - Run integration tests with Testcontainers"
	@echo "  \033[36mmake docker-build\033[0m - Build Docker images for Scrapper and Bot"

.PHONY: build
build:
	@echo "Building all modules: $(MODULES)"
	@mkdir -p bin
	@$(foreach mod,$(MODULES),echo "Building module: $(mod)"; go build -o ./bin/$(mod) ./cmd/$(mod);)

# Convenience targets for building individual modules
.PHONY: $(addprefix build_,$(MODULES))
$(addprefix build_,$(MODULES)):
	@modulename=$(subst build_,,$@); \
	echo "Building module: $$modulename"; \
	mkdir -p bin; \
	go build -o ./bin/$$modulename ./cmd/$$modulename

## test: run all tests
.PHONY: test
test:
	@go test -coverpkg='github.com/es-debug/backend-academy-2024-go-template/...' --race -count=1 -coverprofile='$(COVERAGE_FILE)' ./...
	@go tool cover -func='$(COVERAGE_FILE)' | grep ^total | tr -s '\t'

## test-integration: run integration tests with Testcontainers
.PHONY: test-integration
test-integration: docker-build
	@echo "Running integration tests..."
	@go test -tags=integration -count=1 -timeout=5m -v ./testcontainers_test.go

## docker-build: build Docker images for Scrapper and Bot
.PHONY: docker-build
docker-build: build
	@echo "Building Docker images..."
	@docker build -f Dockerfile.scrapper -t link-tracker:scrapper .
	@docker build -f Dockerfile.bot -t link-tracker:bot .
