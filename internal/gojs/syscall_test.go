package gojs_test

import (
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

//go:embed testdata/syscall/main.go
var syscallGo string

func Test_syscall(t *testing.T) {
	stdout, stderr, err := compileAndRunJsWasm(testCtx, t, syscallGo, wazero.NewModuleConfig())

	require.EqualError(t, err, `module "" closed with exit_code(0)`)
	require.Zero(t, stderr)
	require.Equal(t, `syscall.Getpid()=1
syscall.Getppid()=0
syscall.Getuid()=0
syscall.Getgid()=0
syscall.Geteuid()=0
syscall.Umask(0077)=0o77
syscall.Getgroups()=[0]
os.FindProcess(pid)=&{1 0 0 {{0 0} 0 0 0 0}}
`, stdout)
}
