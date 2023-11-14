package v2

import "embed"

// Testcases is exported for testing wazevo in internal/engine/wazevo.
//
//go:embed testdata/*.wasm
//go:embed testdata/*.json
var Testcases embed.FS
