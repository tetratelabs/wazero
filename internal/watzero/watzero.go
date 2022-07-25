package watzero

import (
	internalwasm "github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/binary"
	"github.com/tetratelabs/wazero/internal/watzero/internal"
)

// Wat2Wasm converts the WebAssembly Text Format (%.wat) into the Binary Format (%.wasm).
// This function returns when the input is exhausted or an error occurs.
//
// Here's a description of the return values:
//   - result is the module parsed or nil on error
//   - err is an error invoking the parser, dangling block comments or unexpected characters.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#text-format%E2%91%A0
func Wat2Wasm(wat string) ([]byte, error) {
	if m, err := internal.DecodeModule([]byte(wat), internalwasm.Features20220419, internalwasm.MemorySizer); err != nil {
		return nil, err
	} else {
		return binary.EncodeModule(m), nil
	}
}
