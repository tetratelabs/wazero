package examples

import (
	"io/ioutil"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mathetake/gasm/wasi"
	"github.com/mathetake/gasm/wasm"
)

func Test_hostFunc(t *testing.T) {
	buf, err := ioutil.ReadFile("wasm/host_func.wasm")
	require.NoError(t, err)

	mod, err := wasm.DecodeModule((buf))
	require.NoError(t, err)

	var cnt uint64
	hostFunc := func(*wasm.VirtualMachine) {
		cnt++
	}

	vm, err := wasm.NewVM()
	require.NoError(t, err)

	err = vm.AddHostFunction("env", "host_func", reflect.ValueOf(hostFunc))
	require.NoError(t, err)

	err = wasi.NewEnvironment().RegisterToVirtualMachine(vm)
	require.NoError(t, err)

	err = vm.InstantiateModule(mod, "test")
	require.NoError(t, err)

	for _, exp := range []uint64{5, 10, 15} {
		_, _, err = vm.ExecExportedFunction("test", "call_host_func", exp)
		require.NoError(t, err)
		require.Equal(t, exp, cnt)
		cnt = 0
	}
}
