package modgen

import (
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm/binary"
)

func TestModGen(t *testing.T) {
	for _, tc := range []struct {
		name string
		seed []byte
	}{
		{name: "empty", seed: []byte{}},
		{name: "small", seed: []byte{1}},
		{name: "small", seed: []byte{2}},
		{name: "small", seed: []byte{1, 1}},
		{name: "small", seed: []byte{1, 2}},
		{name: "small", seed: []byte{1, 3}},
		{name: "small", seed: []byte{1, 4}},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			m := Gen(tc.seed)
			// Generating with the same seed must result in the same module.
			require.Equal(t, m, Gen(tc.seed))
			buf := binary.EncodeModule(m)

			r := wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfig())
			_, err := r.CompileModule(buf)
			require.NoError(t, err)
		})
	}
}
