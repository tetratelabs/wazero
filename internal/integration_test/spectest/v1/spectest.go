package v1

import (
	"embed"
)

// Testcases is exported for cross-process file cache tests.
//
//go:embed testdata/*.wasm
//go:embed testdata/*.json
var Testcases embed.FS
