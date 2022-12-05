package wasi_snapshot_preview1

import "github.com/tetratelabs/wazero/internal/wasm"

const (
	sockAcceptName   = "sock_accept"
	sockRecvName     = "sock_recv"
	sockSendName     = "sock_send"
	sockShutdownName = "sock_shutdown"
)

// sockAccept is the WASI function named sockAcceptName which accepts a new
// incoming connection.
//
// See: https://github.com/WebAssembly/WASI/blob/0ba0c5e2e37625ca5a6d3e4255a998dfaa3efc52/phases/snapshot/docs.md#sock_accept
// and https://github.com/WebAssembly/WASI/pull/458
var sockAccept = stubFunction(
	sockAcceptName,
	[]wasm.ValueType{i32, i32, i32},
	"fd", "flags", "result.fd",
)

// sockRecv is the WASI function named sockRecvName which receives a
// message from a socket.
//
// See: https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-sock_recvfd-fd-ri_data-iovec_array-ri_flags-riflags---errno-size-roflags
var sockRecv = stubFunction(
	sockRecvName,
	[]wasm.ValueType{i32, i32, i32, i32, i32, i32},
	"fd", "ri_data", "ri_data_count", "ri_flags", "result.ro_datalen", "result.ro_flags",
)

// sockSend is the WASI function named sockSendName which sends a message
// on a socket.
//
// See: https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-sock_sendfd-fd-si_data-ciovec_array-si_flags-siflags---errno-size
var sockSend = stubFunction(
	sockSendName,
	[]wasm.ValueType{i32, i32, i32, i32, i32},
	"fd", "si_data", "si_data_count", "si_flags", "result.so_datalen",
)

// sockShutdown is the WASI function named sockShutdownName which shuts
// down socket send and receive channels.
//
// See: https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-sock_shutdownfd-fd-how-sdflags---errno
var sockShutdown = stubFunction(sockShutdownName, []wasm.ValueType{i32, i32}, "fd", "how")
