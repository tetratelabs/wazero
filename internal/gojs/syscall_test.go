package gojs_test

import (
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_syscall(t *testing.T) {
	t.Parallel()

	stdout, stderr, err := compileAndRun(testCtx, "syscall", wazero.NewModuleConfig())

	require.EqualError(t, err, `module "" closed with exit_code(0)`)
	require.Zero(t, stderr)
	require.Equal(t, `syscall.Getpid()=1
syscall.Getppid()=0
syscall.Getuid()=0
syscall.Getgid()=0
syscall.Geteuid()=0
syscall.Umask(0077)=0o22
syscall.Getgroups()=[0]
os.FindProcess(1).Pid=1
`, stdout)
}
