package experimental

import "github.com/tetratelabs/wazero/api"

// CoreFeaturesThreads enables threads instructions ("threads").
//
// Note: The instruction list is too long to enumerate in godoc.
// See https://github.com/WebAssembly/threads/blob/main/proposals/threads/Overview.md
const CoreFeaturesThreads = api.CoreFeatureSIMD << 1
