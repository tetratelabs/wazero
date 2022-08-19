package v1

import (
	"embed"
	"github.com/tetratelabs/wazero/internal/wasm"
)

//go:embed testdata/*.wasm
//go:embed testdata/*.json
var Testcases embed.FS

const EnabledFeatures = wasm.Features20191205
