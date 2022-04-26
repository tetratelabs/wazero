module github.com/tetratelabs/wazero/internal/integration_test/vs/wasm3

go 1.17

require (
	github.com/birros/go-wasm3 v0.0.0-20220320175540-c625eebef38c
	github.com/tetratelabs/wazero v0.0.0
)

replace github.com/tetratelabs/wazero => ../../../..

