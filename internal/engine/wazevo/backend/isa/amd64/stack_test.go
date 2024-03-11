package amd64

import (
	"encoding/binary"
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestUnwindStack(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func() (stack []byte, exp []uintptr)
	}{
		{name: "no frame", setup: func() (_ []byte, exp []uintptr) { return []byte{0}, exp }},
		{name: "three", setup: func() ([]byte, []uintptr) {
			exp := []uintptr{0xffffffff_00000000, 0xffffffff_00000001, 0xffffffff_00000002}
			stack := make([]byte, 240)
			bp := uintptr(unsafe.Pointer(&stack[0]))
			oldRBP1 := bp + 32
			binary.LittleEndian.PutUint64(stack[0:], uint64(oldRBP1))             // old bp
			binary.LittleEndian.PutUint64(stack[8:], uint64(0xffffffff_00000000)) // return address
			oldRBP2 := oldRBP1 + 16
			binary.LittleEndian.PutUint64(stack[oldRBP1-bp:], uint64(oldRBP2))               // old bp
			binary.LittleEndian.PutUint64(stack[oldRBP1-bp+8:], uint64(0xffffffff_00000001)) // return address
			binary.LittleEndian.PutUint64(stack[oldRBP2-bp:], uint64(0))                     // old bp
			binary.LittleEndian.PutUint64(stack[oldRBP2-bp+8:], uint64(0xffffffff_00000002)) // return address
			return stack, exp
		}},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			stack, exp := tc.setup()
			bp := uintptr(unsafe.Pointer(&stack[0]))
			returnAddresses := UnwindStack(0, bp, uintptr(unsafe.Pointer(&stack[len(stack)-1])), nil)
			require.Equal(t, exp, returnAddresses)
		})
	}
}

func addressOf(v *byte) uint64 {
	return uint64(uintptr(unsafe.Pointer(v)))
}

func TestAdjustClonedStack(t *testing.T) {
	// In order to allocate slices on Go heap, we need to allocSlice function.
	allocSlice := func(size int) []byte {
		return make([]byte, size)
	}

	oldStack := allocSlice(512)
	oldRsp := uintptr(unsafe.Pointer(&oldStack[0]))
	oldTop := uintptr(unsafe.Pointer(&oldStack[len(oldStack)-1]))
	rbpIndex := uintptr(32)
	binary.LittleEndian.PutUint64(oldStack[rbpIndex:], addressOf(&oldStack[16+rbpIndex]))
	binary.LittleEndian.PutUint64(oldStack[rbpIndex+16:], addressOf(&oldStack[32+rbpIndex]))
	binary.LittleEndian.PutUint64(oldStack[rbpIndex+32:], addressOf(&oldStack[160+rbpIndex]))

	newStack := allocSlice(1024)
	rsp := uintptr(unsafe.Pointer(&newStack[0]))
	rbp := rsp + rbpIndex
	// Coy old stack to new stack which contains the old pointers to the old stack elements.
	copy(newStack, oldStack)

	AdjustClonedStack(oldRsp, oldTop, rsp, rbp, uintptr(addressOf(&newStack[len(newStack)-1])))
	require.Equal(t, addressOf(&newStack[rbpIndex+16]), binary.LittleEndian.Uint64(newStack[rbpIndex:]))
	require.Equal(t, addressOf(&newStack[rbpIndex+32]), binary.LittleEndian.Uint64(newStack[rbpIndex+16:]))
	require.Equal(t, addressOf(&newStack[rbpIndex+160]), binary.LittleEndian.Uint64(newStack[rbpIndex+32:]))
}
