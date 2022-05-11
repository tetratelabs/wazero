goimports := golang.org/x/tools/cmd/goimports@v0.1.10
golangci_lint := github.com/golangci/golangci-lint/cmd/golangci-lint@v1.46.0
# sync this with netlify.toml!
hugo          := github.com/gohugoio/hugo@v0.98.0

ensureJITFastest := -ldflags '-X github.com/tetratelabs/wazero/internal/integration_test/vs.ensureJITFastest=true'
.PHONY: bench
bench:
	@go test -run=NONE -benchmem -bench=. ./internal/integration_test/bench/...
	@go test -benchmem -bench=. ./internal/integration_test/vs/... $(ensureJITFastest)

.PHONY: bench.check
bench.check:
	@go build ./internal/integration_test/bench/...
	@# Don't use -test.benchmem as it isn't accurate when comparing against CGO libs
	@for d in vs/wasmedge vs/wasmer vs/wasmtime ; do \
		cd ./internal/integration_test/$$d ; \
		go test -bench=. . -tags='wasmedge' $(ensureJITFastest) ; \
		cd - ;\
	done

bench_testdata_dir := internal/integration_test/bench/testdata
.PHONY: build.bench
build.bench:
	@tinygo build -o $(bench_testdata_dir)/case.wasm -scheduler=none --no-debug -target=wasi $(bench_testdata_dir)/case.go

tinygo_sources := $(wildcard examples/*/testdata/*.go examples/*/*/testdata/*.go)
.PHONY: build.examples
build.examples: $(tinygo_sources)
	@for f in $^; do \
	    tinygo build -o $$(echo $$f | sed -e 's/\.go/\.wasm/') -scheduler=none --no-debug --target=wasi $$f; \
	done

spectest_base_dir := internal/integration_test/spectest
spec_version_v1 := wg-1.0
spectest_v1_testdata_dir := $(spectest_base_dir)/v1/testdata

.PHONY: build.spectest
build.spectest: # Note: wabt by default uses >1.0 features, so wast2json flags might drift as they include more. See WebAssembly/wabt#1878
	@$(MAKE) build.spectest.v1

.PHONY: build.spectest.v1
build.spectest.v1:
	@rm -rf $(spectest_v1_testdata_dir)
	@mkdir -p $(spectest_v1_testdata_dir)
	@cd $(spectest_v1_testdata_dir) \
		&& curl -sSL 'https://api.github.com/repos/WebAssembly/spec/contents/test/core?ref=$(spec_version_v1)' | jq -r '.[]| .download_url' | grep -E ".wast" | xargs -Iurl curl -sJL url -O
	@cd $(spectest_v1_testdata_dir) && for f in `find . -name '*.wast'`; do \
		perl -pi -e 's/\((assert_return_canonical_nan|assert_return_arithmetic_nan)\s(\(invoke\s"f32.demote_f64"\s\((f[0-9]{2})\.const\s[a-z0-9.+:-]+\)\))\)/\(assert_return $$2 \(f32.const nan\)\)/g' $$f; \
		perl -pi -e 's/\((assert_return_canonical_nan|assert_return_arithmetic_nan)\s(\(invoke\s"f64\.promote_f32"\s\((f[0-9]{2})\.const\s[a-z0-9.+:-]+\)\))\)/\(assert_return $$2 \(f64.const nan\)\)/g' $$f; \
		perl -pi -e 's/\((assert_return_canonical_nan|assert_return_arithmetic_nan)\s(\(invoke\s"[a-z._0-9]+"\s\((f[0-9]{2})\.const\s[a-z0-9.+:-]+\)\))\)/\(assert_return $$2 \($$3.const nan\)\)/g' $$f; \
		perl -pi -e 's/\((assert_return_canonical_nan|assert_return_arithmetic_nan)\s(\(invoke\s"[a-z._0-9]+"\s\((f[0-9]{2})\.const\s[a-z0-9.+:-]+\)\s\([a-z0-9.\s+-:]+\)\))\)/\(assert_return $$2 \($$3.const nan\)\)/g' $$f; \
		perl -pi -e 's/\((assert_return_canonical_nan|assert_return_arithmetic_nan)\s(\(invoke\s"[a-z._0-9]+"\s\((f[0-9]{2})\.const\s[a-z0-9.+:-]+\)\))\)/\(assert_return $$2 \($$3.const nan\)\)/g' $$f; \
		wast2json \
			--disable-saturating-float-to-int \
			--disable-sign-extension \
			--disable-simd \
			--disable-multi-value \
			--disable-bulk-memory \
			--disable-reference-types \
			--debug-names $$f; \
	done

.PHONY: test
test:
	@go test $$(go list ./... | grep -v spectest) -timeout 120s
	@cd internal/integration_test/asm && go test ./... -timeout 120s

.PHONY: spectest
spectest:
	@$(MAKE) spectest.v1

spectest.v1:
	@go test $$(go list ./... | grep $(spectest_v2_testdata_dir)) -timeout 120s

golangci_lint_path := $(shell go env GOPATH)/bin/golangci-lint

$(golangci_lint_path):
	@go install $(golangci_lint)

golangci_lint_goarch ?= $(shell go env GOARCH)

.PHONY: lint
lint: $(golangci_lint_path)
	@GOARCH=$(golangci_lint_goarch) $(golangci_lint_path) run --timeout 5m

.PHONY: format
format:
	@find . -type f -name '*.go' | xargs gofmt -s -w
	@for f in `find . -name '*.go'`; do \
	    awk '/^import \($$/,/^\)$$/{if($$0=="")next}{print}' $$f > /tmp/fmt; \
	    mv /tmp/fmt $$f; \
	done
	@go run $(goimports) -w -local github.com/tetratelabs/wazero `find . -name '*.go'`

.PHONY: check
check:
	@$(MAKE) lint golangci_lint_goarch=arm64
	@$(MAKE) lint golangci_lint_goarch=amd64
	@$(MAKE) format
	@go mod tidy
	@if [ ! -z "`git status -s`" ]; then \
		echo "The following differences will fail CI until committed:"; \
		git diff --exit-code; \
	fi

.PHONY: site
site: ## Serve website content
	@git submodule update --init
	@cd site && go run $(hugo) server --minify --disableFastRender --baseURL localhost:1313 --cleanDestinationDir -D
