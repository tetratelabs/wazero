package dwarftestdata

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"io"
)

//go:embed testdata/tinygo/main.wasm
var TinyGoWasm []byte

//go:embed testdata/zig/main.wasm
var ZigWasm []byte

var RustWasm []byte

//go:embed testdata/rust/main.wasm.gz
var rustWasmGz []byte

func init() {
	r, err := gzip.NewReader(bytes.NewReader(rustWasmGz))
	if err != nil {
		panic(err)
	}

	RustWasm, err = io.ReadAll(r)
	if err != nil {
		panic(err)
	}
}
