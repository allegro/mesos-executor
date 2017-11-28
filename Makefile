APPLICATION_NAME    := github.com/allegro/mesos-executor
APPLICATION_VERSION := $(shell cat VERSION)

LDFLAGS := -X main.Version=$(APPLICATION_VERSION) -extldflags "-static"
USER_ID := `id -u $$USER`

BUILD_FOLDER := target

GO_BUILD := go build -v -ldflags "$(LDFLAGS)" -a
GO_SRC := $(shell find . -name '*.go')
PACKAGES := $(shell go list ./... | grep -v /vendor/)

TEST_TARGETS = $(PACKAGES)
COVERAGEDIR := $(BUILD_FOLDER)/test-results

CURRENT_DIR = $(shell pwd)
PATH := $(CURRENT_DIR)/bin:$(PATH)

.PHONY: clean test all build release deps lint lint-deps \
		generate-source generate-source-deps

all: lint test build

build: $(BUILD_FOLDER)/executor

$(BUILD_FOLDER)/executor: $(GO_SRC)
	$(GO_BUILD) -o $(BUILD_FOLDER)/executor ./cmd/executor

clean:
	go clean -v .
	rm -rf $(BUILD_FOLDER)
	rm -rf $(CURRENT_DIR)/bin

generate-source: generate-source-deps
	go generate -v $$(go list ./... | grep -v /vendor/)

generate-source-deps:
	go get -v -u golang.org/x/tools/cmd/stringer

lint: lint-deps
	gometalinter.v1 --config=gometalinter.json ./...

lint-deps:
	@which gometalinter.v1 > /dev/null || \
		(go get gopkg.in/alecthomas/gometalinter.v1 && gometalinter.v1 --install)

release: clean build
	zip -j $(BUILD_FOLDER)/executor-linux-amd64.zip $(BUILD_FOLDER)/executor
	chmod 0755 $(BUILD_FOLDER)/executor-linux-amd64.zip
	chmod 0777 $(BUILD_FOLDER)

test: $(COVERAGEDIR)/coverage.out

$(COVERAGEDIR)/coverage.out: test-deps $(COVERAGEDIR) $(GO_SRC) $(TEST_TARGETS)
	gover $(COVERAGEDIR) $(COVERAGEDIR)/coverage.out

$(TEST_TARGETS):
	go test -v -coverprofile=$(COVERAGEDIR)/$(shell basename $@).coverprofile $(TESTARGS) $@

coveralls: test $(COVERAGEDIR)/coverage.out
	goveralls -coverprofile=$(COVERAGEDIR)/coverage.out -service=travis-ci

$(COVERAGEDIR):
	mkdir -p $(BUILD_FOLDER)/test-results

test-deps:
	./scripts/install-consul.sh
	@which gover > /dev/null || \
		(go get github.com/modocache/gover)
	@which goveralls > /dev/null || \
		(go get github.com/mattn/goveralls)
