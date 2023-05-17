package experimental

import (
	"github.com/tetratelabs/wazero/api"
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
