module github.com/tetratelabs/wazero

// temporarily support go 1.16 per #37
go 1.16

require (
	github.com/stretchr/testify v1.7.0
	// Once we reach some maturity, remove this dep and implement our own assembler.
	github.com/twitchyliquid64/golang-asm v0.15.1
)
