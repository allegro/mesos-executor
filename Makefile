APPLICATION_NAME    := github.com/allegro/mesos-executor
APPLICATION_VERSION := $(shell git describe --tags || echo "unknown")

LDFLAGS := -X main.Version=$(APPLICATION_VERSION)

GO_BUILD := go build -v -ldflags "$(LDFLAGS)" -a

CURRENT_DIR = $(shell pwd)
PATH := $(CURRENT_DIR)/bin:$(PATH)

.PHONY: clean test all build package deps lint lint-deps \
		generate-source generate-source-deps

all: lint test build

build: target
	$(GO_BUILD) -o target/executor ./cmd/executor

target:
	mkdir target

clean:
	go clean -v .
	rm -rf target
	rm -rf $(CURRENT_DIR)/bin

generate-source: generate-source-deps
	go generate -v $$(go list ./... | grep -v /vendor/)

generate-source-deps:
	go get -v -u golang.org/x/tools/cmd/stringer

lint: lint-deps
	golangci-lint run --config=golangcilinter.yaml ./...

lint-deps:
	@which golangci-lint > /dev/null || \
		(GO111MODULE=on go get -v github.com/golangci/golangci-lint/cmd/golangci-lint@v1.25.1)

package: target/executor
	zip -j target/executor-$(APPLICATION_VERSION)-linux-amd64.zip target/executor
	chmod 0755 target/executor-$(APPLICATION_VERSION)-linux-amd64.zip

test: test-deps
	go test -coverprofile=target/coverage.txt -covermode=atomic ./...

test-deps: target
	./scripts/install-consul.sh
