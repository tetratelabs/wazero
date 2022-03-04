module github.com/tetratelabs/wazero

go 1.17

require (
	github.com/stretchr/testify v1.7.0
	// Once we reach some maturity, remove this dep and implement our own assembler.
	github.com/twitchyliquid64/golang-asm v0.15.1
)

require (
	github.com/davecgh/go-spew v1.1.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.0-20200313102051-9f266ea9e77c // indirect
)
