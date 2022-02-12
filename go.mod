module github.com/tetratelabs/wazero

// temporarily support go 1.16 per #37
go 1.16

require (
	// only used in benchmarks
	github.com/bytecodealliance/wasmtime-go v0.32.0
	github.com/stretchr/testify v1.7.0
	// Once we reach some maturity, remove this dep and implement our own assembler.
	github.com/twitchyliquid64/golang-asm v0.15.1
	// only used in benchmarks
	github.com/wasmerio/wasmer-go v1.0.4
)

// per https://github.com/tetratelabs/wazero/issues/227
replace github.com/tetratelabs/wazero => ./wazero
