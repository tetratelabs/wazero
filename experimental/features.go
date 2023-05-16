package experimental

import (
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/features"
)

// CoreFeaturesThreads enables threads instructions ("threads").
//
// # Notes
//
//   - This is not yet implemented by default, so you will need to use
//     wazero.NewRuntimeConfigInterpreter
//   - The instruction list is too long to enumerate in godoc.
//     See https://github.com/WebAssembly/threads/blob/main/proposals/threads/Overview.md
//   - Atomic operations are guest-only until api.Memory or otherwise expose them to host functions.
const CoreFeaturesThreads = api.CoreFeatureSIMD << 1 // TODO: Implement the compiler engine

// EnableFeatures enables the list of features passed as arguments.
//
// Features are global because they affect properties of wazero which cannot
// be decoupled, such as interaction with system capabilities that affect the
// entire process instead of a particular wazero.Runtime instance.
//
// Currently, the following features are supported:
//
//   - "hugepages" on linux, enables the use of huge pages in memory mappings
//
// Features are always turned off by default, and may be activated by calling
// this function, which should usually happen before creating any wazero.Runtime
// instances. Once enabled, features cannot be disabled.
func EnableFeatures(featureNames ...string) { features.Enable(featureNames...) }
