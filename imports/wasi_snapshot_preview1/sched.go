package wasi_snapshot_preview1

const functionSchedYield = "sched_yield"

// schedYield is the WASI function named functionSchedYield which temporarily
// yields execution of the calling thread.
//
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-sched_yield---errno
var schedYield = stubFunction(functionSchedYield, nil, nil)
