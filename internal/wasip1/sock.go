package wasip1

const (
	SockAcceptName   = "sock_accept"
	SockRecvName     = "sock_recv"
	SockSendName     = "sock_send"
	SockShutdownName = "sock_shutdown"
)

// SD Flags indicate which channels on a socket to shut down.
// https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-sdflags-flagsu8
const (
	// SD_RD disables further receive operations.
	SD_RD = 0b1
	// SD_WR disables further send operations.
	SD_WR = 0b10
)

// SI Flags are flags provided to sock_send. As there are currently no flags defined, it must be set to zero.
// https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#-siflags-u16

// RI Flags are flags provided to sock_recv.
const (
	// RECV_PEEK returns the message without removing it from the socket's receive queue
	RECV_PEEK uint32 = 0b1
	// RECV_WAITALL on byte-stream sockets, block until the full amount of data can be returned.
	RECV_WAITALL uint32 = 0b10
)

// RO Flags are flags returned by sock_recv.
const (
	// RECV_DATA_TRUNCATED is returned by sock_recv when message data has been truncated.
	RECV_DATA_TRUNCATED = 0b1
)
