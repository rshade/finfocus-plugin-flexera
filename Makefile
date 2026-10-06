BINARY=finfocus-plugin-flexera
VERSION ?= 1.0.0

# Get git information
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
GIT_BRANCH := $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")
GIT_STATE := $(shell if git diff --quiet 2>/dev/null; then echo "clean"; else echo "dirty"; fi)
BUILD_DATE := $(shell date -u '+%Y-%m-%d_%H:%M:%S_UTC')

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

test:
	go test ./...

test-integration:
	go test -tags=integration ./test/integration/...

vet:
	go vet ./...

lint:
	golangci-lint run

lint-markdown:
	npx markdownlint-cli README.md *.md --ignore node_modules

validate-workflows:
	actionlint .github/workflows/*.yml

govulncheck:
	go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...

validate: vet lint govulncheck

depend:
	@echo "Installing Go development tools..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@v2.3.1
	@go install golang.org/x/tools/cmd/goimports@latest
	@go install github.com/fatih/gomodifytags@latest
	@go install github.com/josharian/impl@latest
	@go install github.com/cweill/gotests/gotests@latest
	@go install github.com/golang/mock/mockgen@latest
	@go install github.com/axw/gocov/gocov@latest
	@go install github.com/AlekSi/gocov-xml@latest
	@go install github.com/tebeka/go2xunit@latest
	@echo "Go development tools installed successfully!"

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
