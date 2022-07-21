package wasi_snapshot_preview1

const (
	functionSockRecv     = "sock_recv"
	functionSockSend     = "sock_send"
	functionSockShutdown = "sock_shutdown"
)

// sockRecv is the WASI function named functionSockRecv which receives a
// message from a socket.
//
// See: https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-sock_recvfd-fd-ri_data-iovec_array-ri_flags-riflags---errno-size-roflags
var sockRecv = stubFunction(i32, i32, i32, i32, i32, i32) // stubbed for GrainLang per #271.

// sockSend is the WASI function named functionSockSend which sends a message
// on a socket.
//
// See: https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-sock_sendfd-fd-si_data-ciovec_array-si_flags-siflags---errno-size
var sockSend = stubFunction(i32, i32, i32, i32, i32) // stubbed for GrainLang per #271.

// sockShutdown is the WASI function named functionSockShutdown which shuts
// down socket send and receive channels.
//
// See: https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-sock_shutdownfd-fd-how-sdflags---errno
var sockShutdown = stubFunction(i32, i32) // stubbed for GrainLang per #271.
