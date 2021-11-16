package naivevm

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tetratelabs/wazero/wasm/buildoptions"
)

func TestFrameStack_Push(t *testing.T) {
	f1 := &frame{}
	f2 := &frame{}

	fs := newFrameStack()

	fs.push(f1)
	require.Equal(t, f1, fs.stack[0])
	require.Equal(t, 0, fs.sp)

	fs.push(f2)
	require.Equal(t, f1, fs.stack[0])
	require.Equal(t, f2, fs.stack[1])
	require.Equal(t, 1, fs.sp)
}

func TestFrameStack_Push_Grows(t *testing.T) {
	f := &frame{}

	fs := newFrameStack()

	for i := 0; i < initialLabelStackHeight; i++ {
		fs.push(f)
	}

	f2 := &frame{}
	fs.push(f2) // we expect to grow

	require.Equal(t, f, fs.stack[initialLabelStackHeight-1])
	require.Equal(t, f2, fs.stack[initialLabelStackHeight])
	require.Equal(t, initialLabelStackHeight, fs.sp)
}

func TestFrameStack_Push_StackOverflow(t *testing.T) {
	defer func() { callStackHeightLimit = buildoptions.CallStackHeightLimit }()

	f := &frame{}

	fs := newFrameStack()

	// in naivevm, the stack is a slice with an initial capacity, allow growing 2 past this.
	callStackHeightLimit = initialLabelStackHeight + 2

	for i := 0; i < callStackHeightLimit; i++ {
		fs.push(f)
	}

	// we're past our limit, so we should panic
	require.Panics(t, func() { fs.push(f) })
}
