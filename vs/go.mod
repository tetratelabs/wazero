module github.com/tetratelabs/wazero/vs

// temporarily support go 1.16 per #37
go 1.16

require (
	github.com/bytecodealliance/wasmtime-go v0.34.0
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/stretchr/testify v1.7.0
	github.com/tetratelabs/wazero v0.0.0
	github.com/wasmerio/wasmer-go v1.0.4
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
)

replace github.com/tetratelabs/wazero => ../
