package wazevo

import (
	"context"
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func Test_writeIface_readIface(t *testing.T) {
	buf := make([]byte, 100)

	var called bool
	var goFn api.GoFunction = api.GoFunc(func(context.Context, []uint64) {
		called = true
	})
	writeIface(goFn, buf)
	got := readIface(buf).(api.GoFunction)
	got.Call(context.Background(), nil)
	require.True(t, called)
}

func Test_buildHostModuleOpaque(t *testing.T) {
	for _, tc := range []struct {
		name      string
		m         *wasm.Module
		listeners []experimental.FunctionListener
	}{
		{
			name: "no listeners",
			m: &wasm.Module{
				CodeSection: []wasm.Code{
					{GoFunc: api.GoFunc(func(context.Context, []uint64) {})},
					{GoFunc: api.GoFunc(func(context.Context, []uint64) {})},
				},
			},
		},
		{
			name: "listeners",
			m: &wasm.Module{
				CodeSection: []wasm.Code{
					{GoFunc: api.GoFunc(func(context.Context, []uint64) {})},
					{GoFunc: api.GoFunc(func(context.Context, []uint64) {})},
					{GoFunc: api.GoFunc(func(context.Context, []uint64) {})},
					{GoFunc: api.GoFunc(func(context.Context, []uint64) {})},
				},
			},
			listeners: make([]experimental.FunctionListener, 50),
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := buildHostModuleOpaque(tc.m, tc.listeners)
			opaque := uintptr(unsafe.Pointer(&got[0]))
			require.Equal(t, tc.m, hostModuleFromOpaque(opaque))
			if len(tc.listeners) > 0 {
				require.Equal(t, tc.listeners, hostModuleListenersSliceFromOpaque(opaque))
			}
			for i, c := range tc.m.CodeSection {
				require.Equal(t, c.GoFunc, hostModuleGoFuncFromOpaque[api.GoFunction](i, opaque))
			}
		})
	}
}
