BINARY=finfocus-plugin-flexera
VERSION ?= 0.1.0

# Get git information
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
GIT_STATE := $(shell if git diff --quiet 2>/dev/null; then echo "clean"; else echo "dirty"; fi)
BUILD_DATE := $(shell date -u '+%Y-%m-%d_%H:%M:%S_UTC')

.PHONY: all build test check-coverage test-integration vet lint lint-markdown validate-workflows govulncheck validate ensure install version version-info clean

all: build

build:
	mkdir -p bin
	go build -ldflags "\
		-X github.com/rshade/finfocus-plugin-flexera/pkg/version.Version=$(VERSION) \
		-X github.com/rshade/finfocus-plugin-flexera/pkg/version.BuildDate=$(BUILD_DATE) \
		-X github.com/rshade/finfocus-plugin-flexera/pkg/version.GitCommit=$(GIT_COMMIT) \
		-X github.com/rshade/finfocus-plugin-flexera/pkg/version.GitBranch=$(GIT_BRANCH) \
		-X github.com/rshade/finfocus-plugin-flexera/pkg/version.GitState=$(GIT_STATE)" \
		-o bin/$(BINARY) ./cmd/finfocus-plugin-flexera

COVERPROFILE ?= coverage.out
COVER_MIN ?= 80
# Set FORCE_COVERPROFILE to a cover profile to prove the gate fails.
FORCE_COVERPROFILE ?=

test:
	go test -race -covermode=atomic -coverprofile=$(COVERPROFILE) ./...
	$(MAKE) check-coverage COVERPROFILE=$(if $(FORCE_COVERPROFILE),$(FORCE_COVERPROFILE),$(COVERPROFILE))

check-coverage:
	awk -f scripts/covergate.awk -v min=$(COVER_MIN) $(COVERPROFILE)

test-integration:
	go test -tags=integration ./test/integration/...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

lint-markdown:
	npx markdownlint-cli README.md *.md --ignore node_modules

validate-workflows:
	actionlint .github/workflows/*.yml

govulncheck:
	govulncheck ./...

validate: vet lint govulncheck

# Tools are pinned in mise.toml (Go 1.27.1, golangci-lint 2.14.0).
ensure:
	mise install

install:
	mkdir -p $$HOME/.finfocus/plugins/flexera/$(VERSION)
	cp bin/$(BINARY) $$HOME/.finfocus/plugins/flexera/$(VERSION)/$(BINARY)

version:
	@echo "Version: $(VERSION)"
	@echo "Git Commit: $(GIT_COMMIT)"
	@echo "Git Branch: $(GIT_BRANCH)"
	@echo "Git State: $(GIT_STATE)"
	@echo "Build Date: $(BUILD_DATE)"

version-info:
	@echo "=== Version Information ==="
	@echo "Version: $(VERSION)"
	@echo "Git Commit: $(GIT_COMMIT)"
	@echo "Git Branch: $(GIT_BRANCH)"
	@echo "Git State: $(GIT_STATE)"
	@echo "Build Date: $(BUILD_DATE)"
	@echo "=========================="

clean:
	rm -rf bin/
