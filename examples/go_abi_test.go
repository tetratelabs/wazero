package examples

import (
	"bytes"
	"io/ioutil"
	"testing"

	"github.com/mathetake/gasm/gojs"
	"github.com/mathetake/gasm/wasm"
	"github.com/stretchr/testify/require"
)

func Test_goABI(t *testing.T) {
	buf, err := ioutil.ReadFile("wasm/go_abi.wasm")
	require.NoError(t, err)

	mod, err := wasm.DecodeModule(bytes.NewBuffer(buf))
	require.NoError(t, err)

	_, err = wasm.NewVM(mod, gojs.Modules)
	require.NoError(t, err)
}
