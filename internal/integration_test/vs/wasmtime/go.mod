module github.com/tetratelabs/wazero/internal/integration_test/vs/wasmtime

go 1.19

require (
	github.com/bytecodealliance/wasmtime-go/v9 v9.0.0
	github.com/tetratelabs/wazero v0.0.0
)

replace github.com/tetratelabs/wazero => ../../../..
