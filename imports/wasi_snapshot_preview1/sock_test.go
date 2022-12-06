package wasi_snapshot_preview1

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

// Test_sockAccept only tests it is stubbed for GrainLang per #271
func Test_sockAccept(t *testing.T) {
	log := requireErrnoNosys(t, sockAcceptName, 0, 0, 0)
	require.Equal(t, `
--> proxy.sock_accept(fd=0,flags=0,result.fd=0)
	--> wasi_snapshot_preview1.sock_accept(fd=0,flags=0,result.fd=0)
	<-- ENOSYS
<-- 52
`, log)
}

// Test_sockRecv only tests it is stubbed for GrainLang per #271
func Test_sockRecv(t *testing.T) {
	log := requireErrnoNosys(t, sockRecvName, 0, 0, 0, 0, 0, 0)
	require.Equal(t, `
--> proxy.sock_recv(fd=0,ri_data=0,ri_data_count=0,ri_flags=0,result.ro_datalen=0,result.ro_flags=0)
	--> wasi_snapshot_preview1.sock_recv(fd=0,ri_data=0,ri_data_count=0,ri_flags=0,result.ro_datalen=0,result.ro_flags=0)
	<-- ENOSYS
<-- 52
`, log)
}

// Test_sockSend only tests it is stubbed for GrainLang per #271
func Test_sockSend(t *testing.T) {
	log := requireErrnoNosys(t, sockSendName, 0, 0, 0, 0, 0)
	require.Equal(t, `
--> proxy.sock_send(fd=0,si_data=0,si_data_count=0,si_flags=0,result.so_datalen=0)
	--> wasi_snapshot_preview1.sock_send(fd=0,si_data=0,si_data_count=0,si_flags=0,result.so_datalen=0)
	<-- ENOSYS
<-- 52
`, log)
}

// Test_sockShutdown only tests it is stubbed for GrainLang per #271
func Test_sockShutdown(t *testing.T) {
	log := requireErrnoNosys(t, sockShutdownName, 0, 0)
	require.Equal(t, `
--> proxy.sock_shutdown(fd=0,how=0)
	--> wasi_snapshot_preview1.sock_shutdown(fd=0,how=0)
	<-- ENOSYS
<-- 52
`, log)
}
