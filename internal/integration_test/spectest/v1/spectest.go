package v1

import (
	"embed"

	"github.com/tetratelabs/wazero/api"
)

// Testcases is exported for cross-process file cache tests.
//
//go:embed testdata/*.wasm
//go:embed testdata/*.json
var Testcases embed.FS

// EnabledFeatures is exported for cross-process file cache tests.
const EnabledFeatures = api.CoreFeaturesV1
