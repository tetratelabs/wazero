package wasm

import (
	"math"
	"reflect"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestMemoryPageConsts(t *testing.T) {
	require.Equal(t, MemoryPageSize, uint32(1)<<MemoryPageSizeInBits)
	require.Equal(t, MemoryPageSize, uint32(1<<16))
	require.Equal(t, MemoryLimitPages, uint32(1<<16))
}

func TestMemoryPagesToBytesNum(t *testing.T) {
	for _, numPage := range []uint32{0, 1, 5, 10} {
		require.Equal(t, uint64(numPage*MemoryPageSize), MemoryPagesToBytesNum(numPage))
	}
}

func TestMemoryBytesNumToPages(t *testing.T) {
	for _, numbytes := range []uint32{0, MemoryPageSize * 1, MemoryPageSize * 10} {
		require.Equal(t, numbytes/MemoryPageSize, memoryBytesNumToPages(uint64(numbytes)))
	}
}

func TestMemoryInstance_Grow_Size(t *testing.T) {
	tests := []struct {
		name          string
		capEqualsMax  bool
		expAllocator  bool
		failAllocator bool
	}{
		{name: ""},
		{name: "capEqualsMax", capEqualsMax: true},
		{name: "expAllocator", expAllocator: true},
		{name: "failAllocator", failAllocator: true},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			max := uint32(10)
			maxBytes := MemoryPagesToBytesNum(max)
			me := &mockModuleEngine{}
			var m *MemoryInstance
			switch {
			default:
				m = &MemoryInstance{Max: max, Buffer: make([]byte, 0)}
			case tc.capEqualsMax:
				m = &MemoryInstance{Cap: max, Max: max, Buffer: make([]byte, 0, maxBytes)}
			case tc.expAllocator:
				expBuffer := sliceAllocator(0, maxBytes)
				m = &MemoryInstance{Max: max, Buffer: expBuffer.Reallocate(0), expBuffer: expBuffer}
			case tc.failAllocator:
				expBuffer := sliceAllocator(0, maxBytes)
				m = &MemoryInstance{Max: max * 2, Buffer: expBuffer.Reallocate(0), expBuffer: expBuffer}
			}
			m.ownerModuleEngine = me

			res, ok := m.Grow(5)
			require.True(t, ok)
			require.Equal(t, uint32(0), res)
			require.Equal(t, uint32(5), m.Pages())

			// Zero page grow is well-defined, should return the current page correctly.
			res, ok = m.Grow(0)
			require.True(t, ok)
			require.Equal(t, uint32(5), res)
			require.Equal(t, uint32(5), m.Pages())

			res, ok = m.Grow(4)
			require.True(t, ok)
			require.Equal(t, uint32(5), res)
			require.Equal(t, uint32(9), m.Pages())

			res, ok = m.Grow(0)
			require.True(t, ok)
			require.Equal(t, uint32(9), res)
			require.Equal(t, uint32(9), m.Pages())

			// At this point, the page size equal 9,
			// so trying to grow two pages should result in failure.
			_, ok = m.Grow(2)
			require.False(t, ok)
			require.Equal(t, uint32(9), m.Pages())

			// But growing one page is still permitted.
			res, ok = m.Grow(1)
			require.True(t, ok)
			require.Equal(t, uint32(9), res)

			// Ensure that the current page size equals the max.
			require.Equal(t, max, m.Pages())

			// Growing zero and beyond max won't notify the module engine.
			// So in total, the memoryGrown should be called 3 times.
			require.Equal(t, 3, me.memoryGrown)

			if tc.capEqualsMax { // Ensure the capacity isn't more than max.
				require.Equal(t, maxBytes, uint64(cap(m.Buffer)))
			} else { // Slice doubles, so it should have a higher capacity than max.
				require.True(t, maxBytes < uint64(cap(m.Buffer)))
			}
		})
	}
}

func TestMemoryInstance_NegativeDelta(t *testing.T) {
	m := &MemoryInstance{Buffer: make([]byte, 2*MemoryPageSize)}
	_negative := -1
	negativeu32 := uint32(_negative)
	_, ok := m.Grow(negativeu32)
	// If the negative page size is given, current_page+delta might overflow, and it can result in accidentally shrinking the memory,
	// which is obviously not spec compliant.
	require.False(t, ok)
}

func TestMemoryInstance_ReadByte(t *testing.T) {
	mem := &MemoryInstance{Buffer: []byte{0, 0, 0, 0, 0, 0, 0, 16}, Min: 1}
	v, ok := mem.ReadByte(7)
	require.True(t, ok)
	require.Equal(t, byte(16), v)

	_, ok = mem.ReadByte(8)
	require.False(t, ok)

	_, ok = mem.ReadByte(9)
	require.False(t, ok)
}

func TestPagesToUnitOfBytes(t *testing.T) {
	tests := []struct {
		name     string
		pages    uint32
		expected string
	}{
		{
			name:     "zero",
			pages:    0,
			expected: "0 Ki",
		},
		{
			name:     "one",
			pages:    1,
			expected: "64 Ki",
		},
		{
			name:     "megs",
			pages:    100,
			expected: "6 Mi",
		},
		{
			name:     "max memory",
			pages:    MemoryLimitPages,
			expected: "4 Gi",
		},
		{
			name:     "max uint32",
			pages:    math.MaxUint32,
			expected: "3 Ti",
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, PagesToUnitOfBytes(tc.pages))
		})
	}
}

func TestMemoryInstance_HasSize(t *testing.T) {
	memory := &MemoryInstance{Buffer: make([]byte, MemoryPageSize)}

	tests := []struct {
		name        string
		offset      uint32
		sizeInBytes uint64
		expected    bool
	}{
		{
			name:        "simple valid arguments",
			offset:      0, // arbitrary valid offset
			sizeInBytes: 8, // arbitrary valid size
			expected:    true,
		},
		{
			name:        "maximum valid sizeInBytes",
			offset:      memory.Size() - 8,
			sizeInBytes: 8,
			expected:    true,
		},
		{
			name:        "sizeInBytes exceeds the valid size by 1",
			offset:      100, // arbitrary valid offset
			sizeInBytes: uint64(memory.Size() - 99),
			expected:    false,
		},
		{
			name:        "offset exceeds the memory size",
			offset:      memory.Size(),
			sizeInBytes: 1, // arbitrary size
			expected:    false,
		},
		{
			name:        "offset + sizeInBytes overflows in uint32",
			offset:      math.MaxUint32 - 1, // invalid too large offset
			sizeInBytes: 4,                  // if there's overflow, offset + sizeInBytes is 3, and it may pass the check
			expected:    false,
		},
		{
			name:        "address.wast:200",
			offset:      4294967295,
			sizeInBytes: 1,
			expected:    false,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, memory.hasSize(tc.offset, tc.sizeInBytes))
		})
	}
}

func TestMemoryInstance_ReadUint16Le(t *testing.T) {
	tests := []struct {
		name       string
		memory     []byte
		offset     uint32
		expected   uint16
		expectedOk bool
	}{
		{
			name:       "valid offset with an endian-insensitive v",
			memory:     []byte{0xff, 0xff},
			offset:     0, // arbitrary valid offset.
			expected:   math.MaxUint16,
			expectedOk: true,
		},
		{
			name:       "valid offset with an endian-sensitive v",
			memory:     []byte{0xfe, 0xff},
			offset:     0, // arbitrary valid offset.
			expected:   math.MaxUint16 - 1,
			expectedOk: true,
		},
		{
			name:       "maximum boundary valid offset",
			offset:     1,
			memory:     []byte{0x00, 0x1, 0x00},
			expected:   1, // arbitrary valid v
			expectedOk: true,
		},
		{
			name:   "offset exceeds the maximum valid offset by 1",
			memory: []byte{0xff, 0xff},
			offset: 1,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			memory := &MemoryInstance{Buffer: tc.memory}

			v, ok := memory.ReadUint16Le(tc.offset)
			require.Equal(t, tc.expectedOk, ok)
			require.Equal(t, tc.expected, v)
		})
	}
}

func TestMemoryInstance_ReadUint32Le(t *testing.T) {
	tests := []struct {
		name       string
		memory     []byte
		offset     uint32
		expected   uint32
		expectedOk bool
	}{
		{
			name:       "valid offset with an endian-insensitive v",
			memory:     []byte{0xff, 0xff, 0xff, 0xff},
			offset:     0, // arbitrary valid offset.
			expected:   math.MaxUint32,
			expectedOk: true,
		},
		{
			name:       "valid offset with an endian-sensitive v",
			memory:     []byte{0xfe, 0xff, 0xff, 0xff},
			offset:     0, // arbitrary valid offset.
			expected:   math.MaxUint32 - 1,
			expectedOk: true,
		},
		{
			name:       "maximum boundary valid offset",
			offset:     1,
			memory:     []byte{0x00, 0x1, 0x00, 0x00, 0x00},
			expected:   1, // arbitrary valid v
			expectedOk: true,
		},
		{
			name:   "offset exceeds the maximum valid offset by 1",
			memory: []byte{0xff, 0xff, 0xff, 0xff},
			offset: 1,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			memory := &MemoryInstance{Buffer: tc.memory}

			v, ok := memory.ReadUint32Le(tc.offset)
			require.Equal(t, tc.expectedOk, ok)
			require.Equal(t, tc.expected, v)
		})
	}
}

func TestMemoryInstance_ReadUint64Le(t *testing.T) {
	tests := []struct {
		name       string
		memory     []byte
		offset     uint32
		expected   uint64
		expectedOk bool
	}{
		{
			name:       "valid offset with an endian-insensitive v",
			memory:     []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			offset:     0, // arbitrary valid offset.
			expected:   math.MaxUint64,
			expectedOk: true,
		},
		{
			name:       "valid offset with an endian-sensitive v",
			memory:     []byte{0xfe, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			offset:     0, // arbitrary valid offset.
			expected:   math.MaxUint64 - 1,
			expectedOk: true,
		},
		{
			name:       "maximum boundary valid offset",
			offset:     1,
			memory:     []byte{0x00, 0x1, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
			expected:   1, // arbitrary valid v
			expectedOk: true,
		},
		{
			name:   "offset exceeds the maximum valid offset by 1",
			memory: []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			offset: 1,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			memory := &MemoryInstance{Buffer: tc.memory}

			v, ok := memory.ReadUint64Le(tc.offset)
			require.Equal(t, tc.expectedOk, ok)
			require.Equal(t, tc.expected, v)
		})
	}
}

func TestMemoryInstance_ReadFloat32Le(t *testing.T) {
	tests := []struct {
		name       string
		memory     []byte
		offset     uint32
		expected   float32
		expectedOk bool
	}{
		{
			name:       "valid offset with an endian-insensitive v",
			memory:     []byte{0xff, 0x00, 0x00, 0xff},
			offset:     0, // arbitrary valid offset.
			expected:   math.Float32frombits(uint32(0xff0000ff)),
			expectedOk: true,
		},
		{
			name:       "valid offset with an endian-sensitive v",
			memory:     []byte{0xfe, 0x00, 0x00, 0xff},
			offset:     0, // arbitrary valid offset.
			expected:   math.Float32frombits(uint32(0xff0000fe)),
			expectedOk: true,
		},
		{
			name:       "maximum boundary valid offset",
			offset:     1,
			memory:     []byte{0x00, 0xcd, 0xcc, 0xcc, 0x3d},
			expected:   0.1, // arbitrary valid v
			expectedOk: true,
		},
		{
			name:   "offset exceeds the maximum valid offset by 1",
			memory: []byte{0xff, 0xff, 0xff, 0xff},
			offset: 1,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			memory := &MemoryInstance{Buffer: tc.memory}

			v, ok := memory.ReadFloat32Le(tc.offset)
			require.Equal(t, tc.expectedOk, ok)
			require.Equal(t, tc.expected, v)
		})
	}
}

func TestMemoryInstance_ReadFloat64Le(t *testing.T) {
	tests := []struct {
		name       string
		memory     []byte
		offset     uint32
		expected   float64
		expectedOk bool
	}{
		{
			name:       "valid offset with an endian-insensitive v",
			memory:     []byte{0xff, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff},
			offset:     0, // arbitrary valid offset.
			expected:   math.Float64frombits(uint64(0xff000000000000ff)),
			expectedOk: true,
		},
		{
			name:       "valid offset with an endian-sensitive v",
			memory:     []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xef, 0x7f},
			offset:     0,               // arbitrary valid offset.
			expected:   math.MaxFloat64, // arbitrary valid v
			expectedOk: true,
		},
		{
			name:       "maximum boundary valid offset",
			offset:     1,
			memory:     []byte{0x00, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xef, 0x7f},
			expected:   math.MaxFloat64, // arbitrary valid v
			expectedOk: true,
		},
		{
			name:   "offset exceeds the maximum valid offset by 1",
			memory: []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			offset: 1,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			memory := &MemoryInstance{Buffer: tc.memory}

			v, ok := memory.ReadFloat64Le(tc.offset)
			require.Equal(t, tc.expectedOk, ok)
			require.Equal(t, tc.expected, v)
		})
	}
}

func TestMemoryInstance_Read(t *testing.T) {
	mem := &MemoryInstance{Buffer: []byte{0, 0, 0, 0, 16, 0, 0, 0}, Min: 1}

	buf, ok := mem.Read(4, 4)
	require.True(t, ok)
	require.Equal(t, []byte{16, 0, 0, 0}, buf)

	// Test write-through
	buf[3] = 4
	require.Equal(t, []byte{16, 0, 0, 4}, buf)
	require.Equal(t, []byte{0, 0, 0, 0, 16, 0, 0, 4}, mem.Buffer)

	_, ok = mem.Read(5, 4)
	require.False(t, ok)

	_, ok = mem.Read(9, 4)
	require.False(t, ok)
}

func TestMemoryInstance_WriteUint16Le(t *testing.T) {
	memory := &MemoryInstance{Buffer: make([]byte, 100)}

	tests := []struct {
		name          string
		offset        uint32
		v             uint16
		expectedOk    bool
		expectedBytes []byte
	}{
		{
			name:          "valid offset with an endian-insensitive v",
			offset:        0, // arbitrary valid offset.
			v:             math.MaxUint16,
			expectedOk:    true,
			expectedBytes: []byte{0xff, 0xff},
		},
		{
			name:          "valid offset with an endian-sensitive v",
			offset:        0, // arbitrary valid offset.
			v:             math.MaxUint16 - 1,
			expectedOk:    true,
			expectedBytes: []byte{0xfe, 0xff},
		},
		{
			name:          "maximum boundary valid offset",
			offset:        memory.Size() - 2, // 2 is the size of uint16
			v:             1,                 // arbitrary valid v
			expectedOk:    true,
			expectedBytes: []byte{0x1, 0x00},
		},
		{
			name:          "offset exceeds the maximum valid offset by 1",
			offset:        memory.Size() - 2 + 1, // 2 is the size of uint16
			v:             1,                     // arbitrary valid v
			expectedBytes: []byte{0xff, 0xff},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expectedOk, memory.WriteUint16Le(tc.offset, tc.v))
			if tc.expectedOk {
				require.Equal(t, tc.expectedBytes, memory.Buffer[tc.offset:tc.offset+2]) // 2 is the size of uint16
			}
		})
	}
}

func TestMemoryInstance_WriteUint32Le(t *testing.T) {
	memory := &MemoryInstance{Buffer: make([]byte, 100)}

	tests := []struct {
		name          string
		offset        uint32
		v             uint32
		expectedOk    bool
		expectedBytes []byte
	}{
		{
			name:          "valid offset with an endian-insensitive v",
			offset:        0, // arbitrary valid offset.
			v:             math.MaxUint32,
			expectedOk:    true,
			expectedBytes: []byte{0xff, 0xff, 0xff, 0xff},
		},
		{
			name:          "valid offset with an endian-sensitive v",
			offset:        0, // arbitrary valid offset.
			v:             math.MaxUint32 - 1,
			expectedOk:    true,
			expectedBytes: []byte{0xfe, 0xff, 0xff, 0xff},
		},
		{
			name:          "maximum boundary valid offset",
			offset:        memory.Size() - 4, // 4 is the size of uint32
			v:             1,                 // arbitrary valid v
			expectedOk:    true,
			expectedBytes: []byte{0x1, 0x00, 0x00, 0x00},
		},
		{
			name:          "offset exceeds the maximum valid offset by 1",
			offset:        memory.Size() - 4 + 1, // 4 is the size of uint32
			v:             1,                     // arbitrary valid v
			expectedBytes: []byte{0xff, 0xff, 0xff, 0xff},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expectedOk, memory.WriteUint32Le(tc.offset, tc.v))
			if tc.expectedOk {
				require.Equal(t, tc.expectedBytes, memory.Buffer[tc.offset:tc.offset+4]) // 4 is the size of uint32
			}
		})
	}
}

func TestMemoryInstance_WriteUint64Le(t *testing.T) {
	memory := &MemoryInstance{Buffer: make([]byte, 100)}
	tests := []struct {
		name          string
		offset        uint32
		v             uint64
		expectedOk    bool
		expectedBytes []byte
	}{
		{
			name:          "valid offset with an endian-insensitive v",
			offset:        0, // arbitrary valid offset.
			v:             math.MaxUint64,
			expectedOk:    true,
			expectedBytes: []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		},
		{
			name:          "valid offset with an endian-sensitive v",
			offset:        0, // arbitrary valid offset.
			v:             math.MaxUint64 - 1,
			expectedOk:    true,
			expectedBytes: []byte{0xfe, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		},
		{
			name:          "maximum boundary valid offset",
			offset:        memory.Size() - 8, // 8 is the size of uint64
			v:             1,                 // arbitrary valid v
			expectedOk:    true,
			expectedBytes: []byte{0x1, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		},
		{
			name:       "offset exceeds the maximum valid offset by 1",
			offset:     memory.Size() - 8 + 1, // 8 is the size of uint64
			v:          1,                     // arbitrary valid v
			expectedOk: false,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expectedOk, memory.WriteUint64Le(tc.offset, tc.v))
			if tc.expectedOk {
				require.Equal(t, tc.expectedBytes, memory.Buffer[tc.offset:tc.offset+8]) // 8 is the size of uint64
			}
		})
	}
}

func TestMemoryInstance_WriteFloat32Le(t *testing.T) {
	memory := &MemoryInstance{Buffer: make([]byte, 100)}

	tests := []struct {
		name          string
		offset        uint32
		v             float32
		expectedOk    bool
		expectedBytes []byte
	}{
		{
			name:          "valid offset with an endian-insensitive v",
			offset:        0, // arbitrary valid offset.
			v:             math.Float32frombits(uint32(0xff0000ff)),
			expectedOk:    true,
			expectedBytes: []byte{0xff, 0x00, 0x00, 0xff},
		},
		{
			name:          "valid offset with an endian-sensitive v",
			offset:        0,                                        // arbitrary valid offset.
			v:             math.Float32frombits(uint32(0xff0000fe)), // arbitrary valid v
			expectedOk:    true,
			expectedBytes: []byte{0xfe, 0x00, 0x00, 0xff},
		},
		{
			name:          "maximum boundary valid offset",
			offset:        memory.Size() - 4, // 4 is the size of float32
			v:             0.1,               // arbitrary valid v
			expectedOk:    true,
			expectedBytes: []byte{0xcd, 0xcc, 0xcc, 0x3d},
		},
		{
			name:          "offset exceeds the maximum valid offset by 1",
			offset:        memory.Size() - 4 + 1, // 4 is the size of float32
			v:             math.MaxFloat32,       // arbitrary valid v
			expectedBytes: []byte{0xff, 0xff, 0xff, 0xff},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expectedOk, memory.WriteFloat32Le(tc.offset, tc.v))
			if tc.expectedOk {
				require.Equal(t, tc.expectedBytes, memory.Buffer[tc.offset:tc.offset+4]) // 4 is the size of float32
			}
		})
	}
}

func TestMemoryInstance_WriteFloat64Le(t *testing.T) {
	memory := &MemoryInstance{Buffer: make([]byte, 100)}
	tests := []struct {
		name          string
		offset        uint32
		v             float64
		expectedOk    bool
		expectedBytes []byte
	}{
		{
			name:          "valid offset with an endian-insensitive v",
			offset:        0, // arbitrary valid offset.
			v:             math.Float64frombits(uint64(0xff000000000000ff)),
			expectedOk:    true,
			expectedBytes: []byte{0xff, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff},
		},
		{
			name:          "valid offset with an endian-sensitive v",
			offset:        0,               // arbitrary valid offset.
			v:             math.MaxFloat64, // arbitrary valid v
			expectedOk:    true,
			expectedBytes: []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xef, 0x7f},
		},
		{
			name:          "maximum boundary valid offset",
			offset:        memory.Size() - 8, // 8 is the size of float64
			v:             math.MaxFloat64,   // arbitrary valid v
			expectedOk:    true,
			expectedBytes: []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xef, 0x7f},
		},
		{
			name:       "offset exceeds the maximum valid offset by 1",
			offset:     memory.Size() - 8 + 1, // 8 is the size of float64
			v:          math.MaxFloat64,       // arbitrary valid v
			expectedOk: false,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expectedOk, memory.WriteFloat64Le(tc.offset, tc.v))
			if tc.expectedOk {
				require.Equal(t, tc.expectedBytes, memory.Buffer[tc.offset:tc.offset+8]) // 8 is the size of float64
			}
		})
	}
}

func TestMemoryInstance_Write(t *testing.T) {
	mem := &MemoryInstance{Buffer: []byte{0, 0, 0, 0, 16, 0, 0, 0}, Min: 1}

	buf := []byte{16, 0, 0, 4}
	require.True(t, mem.Write(4, buf))
	require.Equal(t, []byte{0, 0, 0, 0, 16, 0, 0, 4}, mem.Buffer)

	// Test it isn't write-through
	buf[3] = 0
	require.Equal(t, []byte{16, 0, 0, 0}, buf)
	require.Equal(t, []byte{0, 0, 0, 0, 16, 0, 0, 4}, mem.Buffer)

	ok := mem.Write(5, buf)
	require.False(t, ok)

	ok = mem.Write(9, buf)
	require.False(t, ok)
}

func TestMemoryInstance_Write_overflow(t *testing.T) {
	mem := &MemoryInstance{Buffer: []byte{0, 0, 0, 0, 16, 0, 0, 0}, Min: 1}

	// Test overflow
	huge := uint64(math.MaxUint32 + 1 + 4)
	if huge != uint64(int(huge)) {
		t.Skip("Skipping on 32-bit")
	}

	buf := []byte{16, 0, 0, 4}
	//nolint:staticcheck
	header := (*reflect.SliceHeader)(unsafe.Pointer(&buf))
	header.Len = int(huge)
	header.Cap = int(huge)

	require.False(t, mem.Write(4, buf))
}

func TestMemoryInstance_WriteString(t *testing.T) {
	mem := &MemoryInstance{Buffer: []byte{0, 0, 0, 0, 16, 0, 0, 0}, Min: 1}

	s := "bear"
	require.True(t, mem.WriteString(4, s))
	require.Equal(t, []byte{0, 0, 0, 0, 'b', 'e', 'a', 'r'}, mem.Buffer)

	ok := mem.WriteString(5, s)
	require.False(t, ok)

	ok = mem.WriteString(9, s)
	require.False(t, ok)
}

func BenchmarkWriteString(b *testing.B) {
	tests := []string{
		"",
		"bear",
		"hello world",
		strings.Repeat("hello ", 10),
	}
	//nolint intentionally testing interface access
	var mem api.Memory
	mem = &MemoryInstance{Buffer: make([]byte, 1000), Min: 1}
	for _, tt := range tests {
		b.Run("", func(b *testing.B) {
			b.Run("Write", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					if !mem.Write(0, []byte(tt)) {
						b.Fail()
					}
				}
			})
			b.Run("WriteString", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					if !mem.WriteString(0, tt) {
						b.Fail()
					}
				}
			})
		})
	}
}

func Test_atomicStoreLength(t *testing.T) {
	// Doesn't verify atomicity, but at least we're updating the correct thing.
	slice := make([]byte, 10, 20)
	atomicStoreLength(&slice, 15)
	require.Equal(t, 15, len(slice))
}

func Test_atomicStoreLengthAndCap(t *testing.T) {
	// Doesn't verify atomicity, but at least we're updating the correct thing.
	slice := make([]byte, 10, 20)
	atomicStoreLengthAndCap(&slice, 12, 18)
	require.Equal(t, 12, len(slice))
	require.Equal(t, 18, cap(slice))
}

func TestNewMemoryInstance_Shared(t *testing.T) {
	tests := []struct {
		name string
		mem  *Memory
	}{
		{
			name: "min 0, max 1",
			mem:  &Memory{Min: 0, Max: 1, IsMaxEncoded: true, IsShared: true},
		},
		{
			name: "min 0, max 0",
			mem:  &Memory{Min: 0, Max: 0, IsMaxEncoded: true, IsShared: true},
		},
	}

	me := &mockModuleEngine{}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			m := NewMemoryInstance(tc.mem, nil, me)
			require.Equal(t, tc.mem.Min, m.Min)
			require.Equal(t, tc.mem.Max, m.Max)
			require.Equal(t, me, m.ownerModuleEngine)
			require.True(t, m.Shared)
		})
	}
}

func TestMemoryInstance_WaitNotifyOnce(t *testing.T) {
	reader := func(mem *MemoryInstance, offset uint32) uint32 {
		val, _ := mem.ReadUint32Le(offset)
		return val
	}
	t.Run("no waiters", func(t *testing.T) {
		mem := &MemoryInstance{Buffer: []byte{0, 0, 0, 0, 16, 0, 0, 0}, Min: 1, Shared: true}

		notifyWaiters(t, mem, 0, 1, 0)
	})

	t.Run("single wait, notify", func(t *testing.T) {
		mem := &MemoryInstance{Buffer: []byte{0, 0, 0, 0, 16, 0, 0, 0}, Min: 1, Shared: true}

		ch := make(chan string)
		// Reuse same offset 3 times to verify reuse
		for i := 0; i < 3; i++ {
			go func() {
				res := mem.Wait32(0, 0, -1, reader)
				propagateWaitResult(t, ch, res)
			}()

			requireChannelEmpty(t, ch)
			notifyWaiters(t, mem, 0, 1, 1)
			require.Equal(t, "", <-ch)

			notifyWaiters(t, mem, 0, 1, 0)
		}
	})

	t.Run("multiple waiters, notify all", func(t *testing.T) {
		mem := &MemoryInstance{Buffer: []byte{0, 0, 0, 0, 16, 0, 0, 0}, Min: 1, Shared: true}

		ch := make(chan string)
		go func() {
			res := mem.Wait32(0, 0, -1, reader)
			propagateWaitResult(t, ch, res)
		}()
		go func() {
			res := mem.Wait32(0, 0, -1, reader)
			propagateWaitResult(t, ch, res)
		}()

		requireChannelEmpty(t, ch)

		notifyWaiters(t, mem, 0, 2, 2)
		require.Equal(t, "", <-ch)
		require.Equal(t, "", <-ch)
	})

	t.Run("multiple waiters, notify one", func(t *testing.T) {
		mem := &MemoryInstance{Buffer: []byte{0, 0, 0, 0, 16, 0, 0, 0}, Min: 1, Shared: true}

		ch := make(chan string)
		go func() {
			res := mem.Wait32(0, 0, -1, reader)
			propagateWaitResult(t, ch, res)
		}()
		go func() {
			res := mem.Wait32(0, 0, -1, reader)
			propagateWaitResult(t, ch, res)
		}()

		requireChannelEmpty(t, ch)
		notifyWaiters(t, mem, 0, 1, 1)
		require.Equal(t, "", <-ch)
		requireChannelEmpty(t, ch)
		notifyWaiters(t, mem, 0, 1, 1)
		require.Equal(t, "", <-ch)
	})

	t.Run("multiple offsets", func(t *testing.T) {
		mem := &MemoryInstance{Buffer: []byte{0, 0, 0, 0, 16, 0, 0, 0}, Min: 1, Shared: true}

		ch := make(chan string)
		go func() {
			res := mem.Wait32(0, 0, -1, reader)
			propagateWaitResult(t, ch, res)
		}()
		go func() {
			res := mem.Wait32(1, 268435456, -1, reader)
			propagateWaitResult(t, ch, res)
		}()

		requireChannelEmpty(t, ch)
		notifyWaiters(t, mem, 0, 2, 1)
		require.Equal(t, "", <-ch)
		requireChannelEmpty(t, ch)
		notifyWaiters(t, mem, 1, 2, 1)
		require.Equal(t, "", <-ch)
	})

	t.Run("timeout", func(t *testing.T) {
		mem := &MemoryInstance{Buffer: []byte{0, 0, 0, 0, 16, 0, 0, 0}, Min: 1, Shared: true}

		ch := make(chan string)
		go func() {
			res := mem.Wait32(0, 0, 10 /* ns */, reader)
			propagateWaitResult(t, ch, res)
		}()

		require.Equal(t, "timeout", <-ch)
	})
}

func notifyWaiters(t *testing.T, mem *MemoryInstance, offset, count, exp int) {
	t.Helper()
	cur := 0
	tries := 0
	for cur < exp {
		if tries > 100 {
			t.Fatal("too many tries waiting for wait and notify to converge")
		}
		n := mem.Notify(uint32(offset), uint32(count))
		cur += int(n)
		time.Sleep(1 * time.Millisecond)
		tries++
	}
}

func propagateWaitResult(t *testing.T, ch chan string, res uint64) {
	t.Helper()
	switch res {
	case 2:
		ch <- "timeout"
	default:
		ch <- ""
	}
}

func requireChannelEmpty(t *testing.T, ch chan string) {
	t.Helper()
	select {
	case <-ch:
		t.Fatal("channel should be empty")
	default:
		// fallthrough
	}
}

func sliceAllocator(cap, max uint64) experimental.LinearMemory {
	return &sliceBuffer{make([]byte, cap), max}
}

type sliceBuffer struct {
	buf []byte
	max uint64
}

func (b *sliceBuffer) Free() {}

func (b *sliceBuffer) Reallocate(size uint64) []byte {
	if size > b.max {
		return nil
	}
	if cap := uint64(cap(b.buf)); size > cap {
		b.buf = append(b.buf[:cap], make([]byte, size-cap)...)
	} else {
		b.buf = b.buf[:size]
	}
	return b.buf
}
