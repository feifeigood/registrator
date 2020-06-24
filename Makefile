GO=go
FIRST_GOPATH=$(firstword $(subst :, ,$(shell $(GO) env GOPATH)))

BINARY_NAME=registrator
MAIN_PATH=main.go
VERSION=$$(cat VERSION)
GIT_SHA=$$(git rev-parse --short HEAD || echo "GitNotFound")
LDFLAGS= -X main.Version=$(VERSION)  -X 'main.GitSHA=$(GIT_SHA)'

TESTARGS= -v
GOFMT_FILES=$$(find . -name '*.go')|grep -v vendor
TEST=$$(go list ./... |grep -v '/vendor/')
COVERARGS= -coverprofile=profile.out -covermode=atomic
VETARGS= -all

all: fmtcheck test build

fmtcheck:
	@echo ">> checking code style"
	@! gofmt -d $(shell find . -path ./vendor -prune -o -name '*.go' -print) |grep '^'

test: fmtcheck
	@echo ">> running tests"
	@$(GO) test $(TEST) $(TESTARGS)

cover: fmtcheck
	@echo ">> running test coverage"
	rm -f coverage.txt
	@for d in $(TEST); do \
		$(GO) test $(TESTARGS) $(COVERARGS) $$d; \
		if [ -f profile.out ]; then \
			cat profile.out >> coverage.txt; \
			rm profile.out; \
		fi \
	done

format:
	@echo ">> formatting code"
	@gofmt -w $(GOFMT_FILES)

vet:
	@echo ">> vetting code"
	@$(GO) vet $(VETARGS) $$(ls -d */ | grep -v vendor) ; if [ $$? -eq 1 ]; then \
		echo ""; \
		echo "Vet found suspicious constructs. Please check the reported constructs"; \
		echo "and fix them if necessary before submitting the code for review."; \
		exit 1; \
	fi

build:
	@echo ">> build registrator"
	$(GO) build -v -o $(BINARY_NAME) -ldflags "$(LDFLAGS)" $(MAIN_PATH)

clean:
	rm -f $(BINARY_NAME)
	rm -f coverage.txt

.PYTHON: fmtcheck test cover format vet build clean