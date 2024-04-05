package arm64

import (
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestMachine_ResolveRelocations(t *testing.T) {
	// ResolveRelocations takes the address of the "body" function
	// so we pre-allocate a buffer here to reuse so that the value
	// will be stable across executions.
	// When we test we just copy over data to the buffer
	// before encoding.

	buf := make([]byte, 128)
	base := uint(uintptr(unsafe.Pointer(&buf[0])))

	tests := []struct {
		name string

		refToBinaryOffset map[ssa.FuncRef]int
		relocations       []backend.RelocationInfo

		expBinary func() []byte
	}{
		{
			name:        "no trampolines",
			relocations: []backend.RelocationInfo{{Caller: 1, Offset: 8, TrampolineOffset: 0, FuncRef: 3}},
			expBinary: func() []byte {
				copy(buf[8:16], []byte{0xfe, 0xff, 0xff, 0x97})
				return append([]byte{}, buf...)
			},
		},
		{
			name: "1 trampoline",
			refToBinaryOffset: map[ssa.FuncRef]int{
				3: (1<<25)*4 + 100,
			},
			relocations: []backend.RelocationInfo{{Offset: 8, TrampolineOffset: 16, FuncRef: 3}},
			expBinary: func() []byte {
				prefix := []byte{
					0, 0, 0, 0,
					0, 0, 0, 0,
					0x2, 0x0, 0x0, 0x94,
					0, 0, 0, 0,
				}
				copy(buf, prefix)
				encodeTrampoline(base+uint(1<<25)*4+100, buf, 16)
				return append([]byte{}, buf...)
			},
		},
		{
			name: "multiple trampolines",

			relocations: []backend.RelocationInfo{
				{Caller: 1, Offset: 8, TrampolineOffset: 16 + relocationTrampolineSize, FuncRef: 3},
				{Caller: 1, Offset: 12, TrampolineOffset: 16 + 2*relocationTrampolineSize, FuncRef: 4},
			},
			refToBinaryOffset: map[ssa.FuncRef]int{
				1: 500,
				3: (1<<25)*4 + 100,
				4: -(1<<25)*4 - 100,
			},
			expBinary: func() []byte {
				prefix := []byte{
					0, 0, 0, 0,
					0, 0, 0, 0,
					0x7, 0x0, 0x0, 0x94,
					0xb, 0x0, 0x0, 0x94,
				}
				copy(buf, prefix)
				addr1 := base + (1<<25)*4 + 100
				addr2 := base - (1<<25)*4 - 100
				encodeTrampoline(addr1, buf, 16+relocationTrampolineSize)
				encodeTrampoline(addr2, buf, 16+2*relocationTrampolineSize)
				return append([]byte{}, buf...)
			},
		},
		{
			name: "mixed trampolines + within range",
			relocations: []backend.RelocationInfo{
				{Caller: 1, Offset: 4, FuncRef: 3},
				{Caller: 1, Offset: 8, TrampolineOffset: 20 + relocationTrampolineSize, FuncRef: 4},
				{Caller: 1, Offset: 12, TrampolineOffset: 20 + 2*relocationTrampolineSize, FuncRef: 5},
			},
			refToBinaryOffset: map[ssa.FuncRef]int{
				1: 500,
				3: 400,
				4: (1<<25)*4 + 100,
				5: -(1<<25)*4 - 300,
			},
			expBinary: func() []byte {
				prefix := []byte{
					0, 0, 0, 0,
					0x63, 0x0, 0x0, 0x94,
					0x8, 0x0, 0x0, 0x94,
					0xc, 0, 0, 0x94,
				}
				copy(buf, prefix)
				addr1 := base + (1<<25)*4 + 100
				addr2 := base - (1<<25)*4 - 300
				encodeTrampoline(addr1, buf, 20+relocationTrampolineSize)
				encodeTrampoline(addr2, buf, 20+2*relocationTrampolineSize)
				return append([]byte{}, buf...)
			},
		},
		{
			name: "mixed trampolines + within range",
			relocations: []backend.RelocationInfo{
				{Caller: 1, Offset: 4, FuncRef: 3},
				{Caller: 1, Offset: 8, TrampolineOffset: 20 + relocationTrampolineSize, FuncRef: 4},
				{Caller: 1, Offset: 12, TrampolineOffset: 20 + 2*relocationTrampolineSize, FuncRef: 5},
			},
			refToBinaryOffset: map[ssa.FuncRef]int{
				1: 500,
				3: 400,
				4: (1<<25)*4 + 100,
				5: -(1<<25)*4 - 300,
			},
			expBinary: func() []byte {
				prefix := []byte{
					0, 0, 0, 0,
					0x63, 0x0, 0x0, 0x94,
					0x8, 0x0, 0x0, 0x94,
					0xc, 0, 0, 0x94,
				}
				copy(buf, prefix)
				addr1 := base + (1<<25)*4 + 100
				addr2 := base - (1<<25)*4 - 300
				encodeTrampoline(addr1, buf, 20+relocationTrampolineSize)
				encodeTrampoline(addr2, buf, 20+2*relocationTrampolineSize)
				return append([]byte{}, buf...)
			},
		},
		{
			name: "extra rel entries + mixed trampolines + within range on arm64",
			refToBinaryOffset: map[ssa.FuncRef]int{
				1: 400,
				2: 500,
				3: 600,
				4: (1<<25)*4 + 1000,
				5: -(1<<25)*4 - 300,
			},
			relocations: []backend.RelocationInfo{
				{Caller: 1, Offset: 4, FuncRef: 3},
				{Caller: 2, Offset: 8, FuncRef: 1},
				{Caller: 3, Offset: 12, FuncRef: 2},
				{Caller: 10, Offset: 80, FuncRef: 3},
				{Caller: 10, Offset: 84, TrampolineOffset: 20 + relocationTrampolineSize, FuncRef: 4},
				{Caller: 10, Offset: 88, TrampolineOffset: 20 + 2*relocationTrampolineSize, FuncRef: 5},
				{Caller: 12, Offset: 120, FuncRef: 2},
			},
			expBinary: func() []byte {
				prefix := []byte{
					0, 0, 0, 0,
					0x95, 0x0, 0x0, 0x94,
					0x62, 0x0, 0x0, 0x94,
					0x7a, 0, 0, 0x94,
				}
				copy(buf, prefix)
				addr1 := base + (1<<25)*4 + 1000
				addr2 := base - (1<<25)*4 - 300
				encodeTrampoline(addr1, buf, 20+relocationTrampolineSize)
				encodeTrampoline(addr2, buf, 20+2*relocationTrampolineSize)
				return append([]byte{}, buf...)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &machine{}
			buf = buf[0:]
			m.ResolveRelocations(tt.refToBinaryOffset, buf, tt.relocations)
			result := append([]byte{}, buf...)
			buf = buf[0:]
			expBinary := tt.expBinary()
			require.Equal(t, expBinary, result)

			for i := 0; i < len(expBinary); i++ {
				require.Equal(t, expBinary[i], result[i], i)
			}
		})
	}
}
