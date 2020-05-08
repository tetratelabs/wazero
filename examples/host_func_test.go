package examples

import (
	"bytes"
	"io/ioutil"
	"reflect"
	"testing"

	"github.com/mathetake/gasm/hostfunc"
	"github.com/mathetake/gasm/wasi"
	"github.com/mathetake/gasm/wasm"
	"github.com/stretchr/testify/require"
)

func Test_hostFunc(t *testing.T) {
	buf, err := ioutil.ReadFile("wasm/host_func.wasm")
	require.NoError(t, err)

	mod, err := wasm.DecodeModule(bytes.NewBuffer(buf))
	require.NoError(t, err)

	var cnt uint64
	hostFunc := func(*wasm.VirtualMachine) reflect.Value {
		return reflect.ValueOf(func() {
			cnt++
		})
	}

	builder := hostfunc.NewModuleBuilderWith(wasi.Modules)
	builder.MustSetFunction("env", "host_func", hostFunc)
	vm, err := wasm.NewVM(mod, builder.Done())
	require.NoError(t, err)

	for _, exp := range []uint64{5, 10, 15} {
		_, _, err = vm.ExecExportedFunction("call_host_func", exp)
		require.NoError(t, err)
		require.Equal(t, exp, cnt)
		cnt = 0
	}
}
