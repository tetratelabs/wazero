package v1

import (
	"embed"

	"github.com/tetratelabs/wazero/internal/wasm"
)

// Testcases is exported for cross-process file cache tests.
//go:embed testdata/*.wasm
//go:embed testdata/*.json
var Testcases embed.FS

// EnabledFeatures is exported for cross-process file cache tests.
const EnabledFeatures = wasm.Features20191205
