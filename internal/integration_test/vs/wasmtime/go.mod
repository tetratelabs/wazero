module github.com/tetratelabs/wazero/internal/integration_test/vs/wasmtime

go 1.18

require (
	github.com/bytecodealliance/wasmtime-go/v5 v5.0.0
	github.com/tetratelabs/wazero v0.0.0
)

replace github.com/tetratelabs/wazero => ../../../..
