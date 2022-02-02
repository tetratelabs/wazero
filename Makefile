goimports := golang.org/x/tools/cmd/goimports@v0.1.5
golangci_lint := github.com/golangci/golangci-lint/cmd/golangci-lint@v1.42.0

.PHONY: bench
bench:
	@go test -run=NONE -benchmem -bench=. ./tests/...

.PHONY: build.lib
build.lib:
	@echo "Ensuring that the wazero library can be compiled on primary platforms..."
	@GOOS=linux GOARCH=arm64 go build ./...
	@GOOS=linux GOARCH=amd64 go build ./...
	@GOOS=darwin GOARCH=arm64 go build ./...
	@GOOS=darwin GOARCH=amd64 go build ./...
	@GOOS=windows GOARCH=arm64 go build ./...
	@GOOS=windows GOARCH=amd64 go build ./...

bench_testdata_dir := tests/bench/testdata

.PHONY: build.bench
build.bench:
	tinygo build -o $(bench_testdata_dir)/case.wasm -scheduler=none -target=wasi $(bench_testdata_dir)/case.go

wasi_testdata_dir := ./examples/testdata ./tests/wasi/testdata

.PHONY: build.examples
build.examples:
	@$(MAKE) wasi_testdata_dir=./examples/testdata build.tinygo-wasi

.PHONY: build.tests-wasi
build.tests-wasi:
	@$(MAKE) wasi_testdata_dir=./tests/wasi/testdata build.tinygo-wasi

.PHONY: build.tinygo-wasi
build.tinygo-wasi:
	@find $(wasi_testdata_dir) -type f -name "*.go" | xargs -Ip /bin/sh -c 'tinygo build -o $$(echo p | sed -e 's/\.go/\.wasm/') -scheduler=none -target=wasi p'

spectest_testdata_dir := tests/spectest/testdata
spec_version := wg-1.0

.PHONY: build.spectest
build.spectest:
	@rm -rf $(spectest_testdata_dir) && mkdir -p $(spectest_testdata_dir)
	@cd $(spectest_testdata_dir) \
		&& curl -sSL 'https://api.github.com/repos/WebAssembly/spec/contents/test/core?ref=$(spec_version)' | jq -r '.[]| .download_url' | grep -E ".wast"| xargs wget -q
	@cd $(spectest_testdata_dir) && for f in `find . -name '*.wast'`; do \
		perl -pi -e 's/\((assert_return_canonical_nan|assert_return_arithmetic_nan)\s(\(invoke\s"f32.demote_f64"\s\((f[0-9]{2})\.const\s[a-z0-9.+:-]+\)\))\)/\(assert_return $$2 \(f32.const nan\)\)/g' $$f; \
		perl -pi -e 's/\((assert_return_canonical_nan|assert_return_arithmetic_nan)\s(\(invoke\s"f64\.promote_f32"\s\((f[0-9]{2})\.const\s[a-z0-9.+:-]+\)\))\)/\(assert_return $$2 \(f64.const nan\)\)/g' $$f; \
		perl -pi -e 's/\((assert_return_canonical_nan|assert_return_arithmetic_nan)\s(\(invoke\s"[a-z._0-9]+"\s\((f[0-9]{2})\.const\s[a-z0-9.+:-]+\)\))\)/\(assert_return $$2 \($$3.const nan\)\)/g' $$f; \
		perl -pi -e 's/\((assert_return_canonical_nan|assert_return_arithmetic_nan)\s(\(invoke\s"[a-z._0-9]+"\s\((f[0-9]{2})\.const\s[a-z0-9.+:-]+\)\s\([a-z0-9.\s+-:]+\)\))\)/\(assert_return $$2 \($$3.const nan\)\)/g' $$f; \
		perl -pi -e 's/\((assert_return_canonical_nan|assert_return_arithmetic_nan)\s(\(invoke\s"[a-z._0-9]+"\s\((f[0-9]{2})\.const\s[a-z0-9.+:-]+\)\))\)/\(assert_return $$2 \($$3.const nan\)\)/g' $$f; \
		wast2json --debug-names $$f; \
	done

.PHONY: test
test:
	@go test ./...

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
	@go run $(goimports) -w -local github.com/tetratelabs/wazero `find . -name '*.go'`

.PHONY: check
check:
	@$(MAKE) format
	@go mod tidy
	@if [ ! -z "`git status -s`" ]; then \
		echo "The following differences will fail CI until committed:"; \
		git diff --exit-code; \
	fi
