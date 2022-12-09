package dwarftestdata

import _ "embed"

//go:embed testdata/tinygo/main.wasm
var TinyGoWasm []byte

//go:embed testdata/zig/main.wasm
var ZigWasm []byte
