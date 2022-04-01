package wazero

import (
	"fmt"

	internalwasi "github.com/tetratelabs/wazero/internal/wasi"
	internalwasm "github.com/tetratelabs/wazero/internal/wasm"
)

const (
	// WASIModuleSnapshotPreview1 is the module name WASI functions are exported into
	// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md
	WASIModuleSnapshotPreview1 = "wasi_snapshot_preview1"
)

// WASISnapshotPreview1 are functions importable as the module name WASIModuleSnapshotPreview1
func WASISnapshotPreview1() *CompiledCode {
	_, fns := internalwasi.SnapshotPreview1Functions()
	m, err := internalwasm.NewHostModule(WASIModuleSnapshotPreview1, fns)
	if err != nil {
		panic(fmt.Errorf("BUG: %w", err))
	}
	return &CompiledCode{module: m}
}
