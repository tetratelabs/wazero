package examples

import (
	"bytes"
	"io/ioutil"
	"reflect"
	"testing"

	"github.com/mathetake/gasm/hostmodule"
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

	builder := hostmodule.NewBuilderWith(wasi.Modules)
	builder.MustAddFunction("env", "host_func", hostFunc)
	err = mod.BuildIndexSpaces(builder.Done())
	require.NoError(t, err)

	vm, err := wasm.NewVM(mod)
	require.NoError(t, err)

	for _, exp := range []uint64{5, 10, 15} {
		_, _, err = vm.ExecExportedFunction("call_host_func", exp)
		require.NoError(t, err)
		require.Equal(t, exp, cnt)
		cnt = 0
	}
}
