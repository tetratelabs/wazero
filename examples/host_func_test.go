package examples

import (
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mathetake/gasm/wasi"
	"github.com/mathetake/gasm/wasm"
	"github.com/mathetake/gasm/wasm/naivevm"
)

func Test_hostFunc(t *testing.T) {
	buf, err := os.ReadFile("wasm/host_func.wasm")
	require.NoError(t, err)

	mod, err := wasm.DecodeModule((buf))
	require.NoError(t, err)

	var cnt uint64
	hostFunc := func(*wasm.HostFunctionCallContext) {
		cnt++
	}

	store := wasm.NewStore(naivevm.NewEngine())

	err = store.AddHostFunction("env", "host_func", reflect.ValueOf(hostFunc))
	require.NoError(t, err)

	err = wasi.NewEnvironment().Register(store)
	require.NoError(t, err)

	err = store.Instantiate(mod, "test")
	require.NoError(t, err)

	for _, exp := range []uint64{5, 10, 15} {
		_, _, err = store.CallFunction("test", "call_host_func", exp)
		require.NoError(t, err)
		require.Equal(t, exp, cnt)
		cnt = 0
	}
}
