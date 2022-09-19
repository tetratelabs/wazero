package wasi_snapshot_preview1

import "github.com/tetratelabs/wazero/internal/wasm"

const (
	functionSockAccept   = "sock_accept"
	functionSockRecv     = "sock_recv"
	functionSockSend     = "sock_send"
	functionSockShutdown = "sock_shutdown"
)

// sockAccept is the WASI function named functionSockAccept which accepts a new
// incoming connection.
//
// See: https://github.com/WebAssembly/WASI/blob/0ba0c5e2e37625ca5a6d3e4255a998dfaa3efc52/phases/snapshot/docs.md#sock_accept
// and https://github.com/WebAssembly/WASI/pull/458
var sockAccept = stubFunction(
	functionSockAccept,
	[]wasm.ValueType{i32, i32, i32},
	[]string{"fd", "flags", "result.fd"},
)

// sockRecv is the WASI function named functionSockRecv which receives a
// message from a socket.
//
// See: https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-sock_recvfd-fd-ri_data-iovec_array-ri_flags-riflags---errno-size-roflags
var sockRecv = stubFunction(
	functionSockRecv,
	[]wasm.ValueType{i32, i32, i32, i32, i32, i32},
	[]string{"fd", "ri_data", "ri_data_count", "ri_flags", "result.ro_datalen", "result.ro_flags"},
)

// sockSend is the WASI function named functionSockSend which sends a message
// on a socket.
//
// See: https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-sock_sendfd-fd-si_data-ciovec_array-si_flags-siflags---errno-size
var sockSend = stubFunction(
	functionSockSend,
	[]wasm.ValueType{i32, i32, i32, i32, i32},
	[]string{"fd", "si_data", "si_data_count", "si_flags", "result.so_datalen"},
)

// sockShutdown is the WASI function named functionSockShutdown which shuts
// down socket send and receive channels.
//
// See: https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-sock_shutdownfd-fd-how-sdflags---errno
var sockShutdown = stubFunction(
	functionSockShutdown,
	[]wasm.ValueType{i32, i32},
	[]string{"fd", "how"},
)
