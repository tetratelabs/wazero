module github.com/tetratelabs/wazero/internal/integration_test/vs/wasmer

go 1.17

require (
	github.com/tetratelabs/wazero v0.0.0
	github.com/wasmerio/wasmer-go v1.0.4
)

replace github.com/tetratelabs/wazero => ../../../..
