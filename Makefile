goimports := golang.org/x/tools/cmd/goimports@v0.1.5
golangci_lint := github.com/golangci/golangci-lint/cmd/golangci-lint@v1.42.0

.PHONY: build.examples
build.examples:
	@find ./examples/wasm -type f -name "*.go" | xargs -Ip /bin/sh -c 'tinygo build -o $$(echo p | sed -e 's/\.go/\.wasm/') -scheduler=none -target=wasi p'

# TODO: implement wat to wasm and do it in tests when necessary.
build.spectest:
	@find ./wasm/spectest -type f -name "*.wast" | xargs -Ip /bin/sh -c 'wat2wasm p -o $$(echo p | sed -e 's/\.wast/\.wasm/')'

.PHONY: test
test:
	@go test $$(eval go list ./... | grep -v "examples/wasm") -v -race

.PHONY: lint
lint:
	@go run $(golangci_lint) run --timeout 5m

.PHONY: format
format:
	@find . -type f -name '*.go' | xargs gofmt -s -w
	@for f in `find . -name '*.go'`; do \
	    awk '/^import \($$/,/^\)$$/{if($$0=="")next}{print}' $$f > /tmp/fmt; \
	    mv /tmp/fmt $$f; \
	done
	@go run $(goimports) -w -local github.com/mathetake/gasm `find . -name '*.go'`

.PHONY: check
check:
	@$(MAKE) format
	@go mod tidy
	@if [ ! -z "`git status -s`" ]; then \
		echo "The following differences will fail CI until committed:"; \
		git diff --exit-code; \
	fi
