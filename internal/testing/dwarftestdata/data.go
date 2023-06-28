package dwarftestdata

import (
	"bytes"
	"embed"
	"fmt"
	"os/exec"
)

// DWARF tests are build-sensitive in the sense that
// whenever rebuilding the program, it is highly likely that
// line number, source code file, etc, can change. Therefore,
// even though these binaries are huge, we check them in to the repositories.

//go:embed testdata/tinygo/main.wasm
var TinyGoWasm []byte

//go:embed testdata/zig/main.wasm
var ZigWasm []byte

//go:embed testdata/zig-cc/main.wasm
var ZigCCWasm []byte

// RustWasm comes with huge DWARF sections, so we do not check it in directly,
// but instead xz-compressed one is.
var RustWasm []byte

//go:embed testdata/rust
var testRustDir embed.FS

func init() {
	const wasmPath = "testdata/rust/main.wasm"
	var err error
	RustWasm, err = testRustDir.ReadFile(wasmPath)
	if err != nil {
		const xzPath = "testdata/rust/main.wasm.xz"
		xzWasm, err := testRustDir.ReadFile(xzPath)
		if err != nil {
			panic(err)
		}
		cmd := exec.Command("xz", "-d")
		cmd.Stdin = bytes.NewReader(xzWasm)
		out := bytes.NewBuffer(nil)
		cmd.Stdout = out
		if err = cmd.Run(); err != nil {
			fmt.Printf("Skipping DWARF tests for rusts as xz command failed: %v", err)
			return
		}
		RustWasm = out.Bytes()
	}
}
