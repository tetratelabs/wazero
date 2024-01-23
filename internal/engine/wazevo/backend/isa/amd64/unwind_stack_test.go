package amd64

import (
	"encoding/binary"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"testing"
	"unsafe"
)

func TestUnwindStack(t *testing.T) {
	for _, tc := range []struct {
		name     string
		contents []uint64
		exp      []uintptr
	}{} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			buf := make([]byte, len(tc.contents)*8+1)
			for i, v := range tc.contents {
				binary.LittleEndian.PutUint64(buf[i*8:], v)
			}
			sp := uintptr(unsafe.Pointer(&buf[0]))
			returnAddresses := UnwindStack(sp, uintptr(unsafe.Pointer(&buf[len(buf)-1])), nil)
			require.Equal(t, tc.exp, returnAddresses)
		})
	}
}
