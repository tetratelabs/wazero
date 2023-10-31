package wazevo

import (
	"reflect"
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestCallEngine_init(t *testing.T) {
	c := &callEngine{}
	c.init()
	require.True(t, c.stackTop%16 == 0)
	require.Equal(t, &c.stack[0], c.execCtx.stackBottomPtr)
}

func TestCallEngine_growStack(t *testing.T) {
	t.Run("stack overflow", func(t *testing.T) {
		c := &callEngine{stack: make([]byte, callStackCeiling+1)}
		_, err := c.growStack()
		require.Error(t, err)
	})

	t.Run("ok", func(t *testing.T) {
		s := make([]byte, 32)
		for i := range s {
			s[i] = byte(i)
		}
		c := &callEngine{
			stack:    s,
			stackTop: uintptr(unsafe.Pointer(&s[15])),
			execCtx: executionContext{
				stackGrowRequiredSize:    160,
				stackPointerBeforeGoCall: (*uint64)(unsafe.Pointer(&s[10])),
			},
		}
		newSP, err := c.growStack()
		require.NoError(t, err)
		require.Equal(t, 160+32*2+16, len(c.stack))

		require.True(t, c.stackTop%16 == 0)
		require.Equal(t, &c.stack[0], c.execCtx.stackBottomPtr)

		var view []byte
		{
			sh := (*reflect.SliceHeader)(unsafe.Pointer(&view))
			sh.Data = newSP
			sh.Len = 5
			sh.Cap = 5
		}
		require.Equal(t, []byte{10, 11, 12, 13, 14}, view)
		require.True(t, newSP >= uintptr(unsafe.Pointer(c.execCtx.stackBottomPtr)))
		require.True(t, newSP <= c.stackTop)
	})
}

func TestCallEngine_requiredInitialStackSize(t *testing.T) {
	c := &callEngine{}
	require.Equal(t, 512, c.requiredInitialStackSize())
	c.sizeOfParamResultSlice = 10
	require.Equal(t, 512, c.requiredInitialStackSize())
	c.sizeOfParamResultSlice = 120
	require.Equal(t, 120*16+32+16, c.requiredInitialStackSize())
}
