package wasi_snapshot_preview1

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

// Test_sockRecv only tests it is stubbed for GrainLang per #271
func Test_sockRecv(t *testing.T) {
	log := requireErrnoNosys(t, functionSockRecv, 0, 0, 0, 0, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.sock_recv(fd=0,ri_data=0,ri_data_count=0,ri_flags=0,result.ro_datalen=0,result.ro_flags=0)
<-- ENOSYS
`, log)
}

// Test_sockSend only tests it is stubbed for GrainLang per #271
func Test_sockSend(t *testing.T) {
	log := requireErrnoNosys(t, functionSockSend, 0, 0, 0, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.sock_send(fd=0,si_data=0,si_data_count=0,si_flags=0,result.so_datalen=0)
<-- ENOSYS
`, log)
}

// Test_sockShutdown only tests it is stubbed for GrainLang per #271
func Test_sockShutdown(t *testing.T) {
	log := requireErrnoNosys(t, functionSockShutdown, 0, 0)
	require.Equal(t, `
--> wasi_snapshot_preview1.sock_shutdown(fd=0,how=0)
<-- ENOSYS
`, log)
}
