module github.com/tetratelabs/wazero/internal/integration_test/vs

go 1.17

require (
	github.com/birros/go-wasm3 v0.0.0-20220320175540-c625eebef38c
	github.com/bytecodealliance/wasmtime-go v0.35.0
	github.com/tetratelabs/wazero v0.0.0
	github.com/wasmerio/wasmer-go v1.0.4
)

require (
	github.com/davecgh/go-spew v1.1.1 // indirect
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
)

replace github.com/tetratelabs/wazero => ../../..
