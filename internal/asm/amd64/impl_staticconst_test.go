package amd64

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/asm"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestConstPool_addConst(t *testing.T) {
	p := newConstPool()
	cons := []byte{1, 2, 3, 4}

	// Loop twice to ensure that the same constant is cached and not added twice.
	for i := 0; i < 2; i++ {
		p.addConst(cons)
		require.Equal(t, 1, len(p.consts))
		require.Equal(t, len(cons), p.poolSizeInBytes)
		_, ok := p.offsetFinalizedCallbacks[asm.StaticConstKey(cons)]
		require.True(t, ok)
	}
}

func TestAssemblerImpl_CompileLoadStaticConstToRegister(t *testing.T) {
	a := NewAssemblerImpl()
	t.Run("odd count of bytes", func(t *testing.T) {
		err := a.CompileLoadStaticConstToRegister(MOVDQU, []byte{1}, RegAX)
		require.Error(t, err)
	})
	t.Run("ok", func(t *testing.T) {
		cons := []byte{1, 2, 3, 4}
		err := a.CompileLoadStaticConstToRegister(MOVDQU, cons, RegAX)
		require.NoError(t, err)
		actualNode := a.Current
		require.Equal(t, MOVDQU, actualNode.Instruction)
		require.Equal(t, OperandTypeStaticConst, actualNode.Types.src)
		require.Equal(t, OperandTypeRegister, actualNode.Types.dst)
		require.Equal(t, cons, actualNode.staticConst)
	})
}

func TestAssemblerImpl_maybeFlushConstants(t *testing.T) {
	t.Run("no consts", func(t *testing.T) {
		a := NewAssemblerImpl()
		// Invoking maybeFlushConstants before encoding consts usage should not panic.
		a.maybeFlushConstants(false)
		a.maybeFlushConstants(true)
	})

	largeData := make([]byte, 256)

	tests := []struct {
		name                    string
		endOfFunction           bool
		dummyBodyBeforeFlush    []byte
		firstUseOffsetInBinary  uint64
		consts                  []asm.StaticConst
		expectedOffsetForConsts []int
		exp                     []byte
		maxDisplacement         int
	}{
		{
			name:                    "end of function",
			endOfFunction:           true,
			dummyBodyBeforeFlush:    []byte{'?', '?', '?', '?'},
			consts:                  []asm.StaticConst{{1, 2, 3, 4, 5, 6, 7, 8}, {10, 11, 12, 13}},
			expectedOffsetForConsts: []int{4, 4 + 8}, // 4 = len(dummyBodyBeforeFlush)
			firstUseOffsetInBinary:  0,
			exp:                     []byte{'?', '?', '?', '?', 1, 2, 3, 4, 5, 6, 7, 8, 10, 11, 12, 13},
			maxDisplacement:         1 << 31, // large displacement will emit the consts at the end of function.
		},
		{
			name:                   "not flush",
			endOfFunction:          false,
			dummyBodyBeforeFlush:   []byte{'?', '?', '?', '?'},
			consts:                 []asm.StaticConst{{1, 2, 3, 4, 5, 6, 7, 8}, {10, 11, 12, 13}},
			firstUseOffsetInBinary: 0,
			exp:                    []byte{'?', '?', '?', '?'},
			maxDisplacement:        1 << 31, // large displacement will emit the consts at the end of function.
		},
		{
			name:                    "not end of function but flush - short jump",
			endOfFunction:           false,
			dummyBodyBeforeFlush:    []byte{'?', '?', '?', '?'},
			consts:                  []asm.StaticConst{{1, 2, 3, 4, 5, 6, 7, 8}, {10, 11, 12, 13}},
			expectedOffsetForConsts: []int{4 + 2, 4 + 2 + 8}, // 4 = len(dummyBodyBeforeFlush), 2 = the size of jump
			firstUseOffsetInBinary:  0,
			exp: []byte{'?', '?', '?', '?',
				0xeb, 0x0c, // short jump with offset = len(consts[0]) + len(consts[1]) = 12 = 0xc.
				1, 2, 3, 4, 5, 6, 7, 8, 10, 11, 12, 13},
			maxDisplacement: 0, // small displacement flushes the const immediately, not at the end of function.
		},
		{
			name:                    "not end of function but flush - long jump",
			endOfFunction:           false,
			dummyBodyBeforeFlush:    []byte{'?', '?', '?', '?'},
			consts:                  []asm.StaticConst{largeData},
			expectedOffsetForConsts: []int{4 + 5}, // 4 = len(dummyBodyBeforeFlush), 5 = the size of jump
			firstUseOffsetInBinary:  0,
			exp: append([]byte{'?', '?', '?', '?',
				0xe9, 0x0, 0x1, 0x0, 0x0, // short jump with offset = 256 = 0x0, 0x1, 0x0, 0x0 (in Little Endian).
			}, largeData...),
			maxDisplacement: 0, // small displacement flushes the const immediately, not at the end of function.
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := NewAssemblerImpl()
			a.MaxDisplacementForConstantPool = tc.maxDisplacement
			a.Buf.Write(tc.dummyBodyBeforeFlush)

			for i, c := range tc.consts {
				a.pool.addConst(c)
				key := asm.StaticConstKey(c)
				i := i
				a.pool.offsetFinalizedCallbacks[key] = append(a.pool.offsetFinalizedCallbacks[key], func(offsetOfConstInBinary int) {
					require.Equal(t, tc.expectedOffsetForConsts[i], offsetOfConstInBinary)
				})
			}

			a.pool.firstUseOffsetInBinary = &tc.firstUseOffsetInBinary
			a.maybeFlushConstants(tc.endOfFunction)

			require.Equal(t, tc.exp, a.Buf.Bytes())
		})
	}
}

func TestAssemblerImpl_encodeStaticConstToRegister(t *testing.T) {
	consts := []asm.StaticConst{
		{0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11},
		{0x22, 0x22, 0x22, 0x22},
		{0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33},
	}
	a := NewAssemblerImpl()

	a.CompileStandAlone(UD2) // insert any dummy instruction before MOVDQUs.
	err := a.CompileLoadStaticConstToRegister(MOVDQU, consts[0], RegX12)
	require.NoError(t, err)
	err = a.CompileLoadStaticConstToRegister(MOVDQU, consts[1], RegX0)
	require.NoError(t, err)
	err = a.CompileLoadStaticConstToRegister(MOVDQU, consts[0], RegX0)
	require.NoError(t, err)
	err = a.CompileLoadStaticConstToRegister(MOVDQU, consts[2], RegX12)
	require.NoError(t, err)

	actual, err := a.Assemble()
	require.NoError(t, err)

	require.Equal(t, []byte{
		0x0f, 0x0b, // dummy instruction.
		// 0x2: movdqu xmm12, xmmword ptr [rip + 0x19]
		// where rip = 0x0b, therefore [rip + 0x19] = [0x24] = consts[0].
		0xf3, 0x44, 0x0f, 0x6f, 0x25, 0x19, 0x00, 0x00, 0x00,
		// 0x0b: movdqu xmm0, xmmword ptr [rip + 0x19]
		// where rip = 0x13, therefore [rip + 0x19] = [0x2c] = consts[1].
		0xf3, 0x0f, 0x6f, 0x05, 0x19, 0x00, 0x00, 0x00,
		// 0x13: movdqu xmm0, xmmword ptr [rip + 0x9]
		// where rip = 0x1b, therefore [rip + 0x9] = [0x24] = consts[0].
		0xf3, 0x0f, 0x6f, 0x05, 0x09, 0x00, 0x00, 0x00,
		// 0x1b: movdqu xmm12, xmmword ptr [rip + 0xc]
		// where rip = 0x24, therefore [rip + 0xc] = [0x30] = consts[2].
		0xf3, 0x44, 0x0f, 0x6f, 0x25, 0x0c, 0x00, 0x00, 0x00,
		// 0x24: consts[0]
		0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x11,
		// 0x2c: consts[1]
		0x22, 0x22, 0x22, 0x22,
		// 0x30: consts[2]
		0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33, 0x33,
	}, actual)
}
