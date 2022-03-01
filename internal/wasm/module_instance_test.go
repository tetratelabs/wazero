package internalwasm

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestModuleInstance_Memory(t *testing.T) {
	t.Skip() // TODO fix
	tests := []struct {
		name        string
		input       *Module
		expected    bool
		expectedLen uint32
	}{
		{
			name:  "no memory",
			input: &Module{},
		},
		{
			name:  "memory not exported",
			input: &Module{MemorySection: []*MemoryType{{1, nil}}},
		},
		{
			name:  "memory not exported, one page",
			input: &Module{MemorySection: []*MemoryType{{1, nil}}},
		},
		{
			name: "memory exported, different name",
			input: &Module{
				MemorySection: []*MemoryType{{1, nil}},
				ExportSection: map[string]*Export{"momory": {Type: ExternTypeMemory, Name: "momory", Index: 0}},
			},
		},
		{
			name: "memory exported, but zero length",
			input: &Module{
				MemorySection: []*MemoryType{{0, nil}},
				ExportSection: map[string]*Export{"memory": {Type: ExternTypeMemory, Name: "memory", Index: 0}},
			},
			expected: true,
		},
		{
			name: "memory exported, one page",
			input: &Module{
				MemorySection: []*MemoryType{{1, nil}},
				ExportSection: map[string]*Export{"memory": {Type: ExternTypeMemory, Name: "memory", Index: 0}},
			},
			expected:    true,
			expectedLen: 65536,
		},
		{
			name: "memory exported, two pages",
			input: &Module{
				MemorySection: []*MemoryType{{2, nil}},
				ExportSection: map[string]*Export{"memory": {Type: ExternTypeMemory, Name: "memory", Index: 0}},
			},
			expected:    true,
			expectedLen: 65536 * 2,
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			s := NewStore(context.Background(), &catchContext{})

			instance, err := s.Instantiate(tc.input, "test")
			require.NoError(t, err)

			mem := instance.Memory()
			if tc.expected {
				require.Equal(t, tc.expectedLen, mem.Size())
			} else {
				require.Nil(t, mem)
			}
		})
	}
}

func TestModuleInstance_validateData(t *testing.T) {
	m := &ModuleInstance{MemoryInstance: &MemoryInstance{Buffer: make([]byte, 5)}}
	for _, tc := range []struct {
		name   string
		data   []*DataSegment
		expErr bool
	}{
		{
			name: "ok",
			data: []*DataSegment{
				{OffsetExpression: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x1}}, Init: []byte{0}},
				{OffsetExpression: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x2}}, Init: []byte{0}},
			},
		},
		{
			name: "out of bounds - single one byte",
			data: []*DataSegment{
				{OffsetExpression: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x5}}, Init: []byte{0}},
			},
			expErr: true,
		},
		{
			name: "out of bounds - multi bytes",
			data: []*DataSegment{
				{OffsetExpression: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x3}}, Init: []byte{0, 1, 2}},
			},
			expErr: true,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := m.validateData(tc.data)
			if tc.expErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestModuleInstance_applyData(t *testing.T) {
	m := &ModuleInstance{MemoryInstance: &MemoryInstance{Buffer: make([]byte, 10)}}
	m.applyData([]*DataSegment{
		{OffsetExpression: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x0}}, Init: []byte{0xa, 0xf}},
		{OffsetExpression: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x8}}, Init: []byte{0x1, 0x5}},
	})
	require.Equal(t, []byte{0xa, 0xf, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x5}, m.MemoryInstance.Buffer)
}

func TestModuleInstance_validateElements(t *testing.T) {
	functionCounts := uint32(0xa)
	m := &ModuleInstance{
		TableInstance: &TableInstance{Table: make([]TableElement, 10)},
		Functions:     make([]*FunctionInstance, 10),
	}
	for _, tc := range []struct {
		name     string
		elements []*ElementSegment
		expErr   bool
	}{
		{
			name: "ok",
			elements: []*ElementSegment{
				{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x0}}, Init: []uint32{0, functionCounts - 1}},
			},
		},
		{
			name: "ok on edge",
			elements: []*ElementSegment{
				{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x8}}, Init: []uint32{0, functionCounts - 1}},
			},
		},
		{
			name: "out of bounds",
			elements: []*ElementSegment{
				{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x9}}, Init: []uint32{0, functionCounts - 1}},
			},
			expErr: true,
		},
		{
			name: "unknown function",
			elements: []*ElementSegment{
				{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{0x0}}, Init: []uint32{0, functionCounts}},
			},
			expErr: true,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := m.validateElements(tc.elements)
			if tc.expErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestModuleInstance_applyElements(t *testing.T) {
	functionCounts := uint32(0xa)
	m := &ModuleInstance{
		TableInstance: &TableInstance{Table: make([]TableElement, 10)},
		Functions:     make([]*FunctionInstance, 10),
	}
	targetAddr, targetOffset := uint32(1), byte(0)
	targetAddr2, targetOffset2 := functionCounts-1, byte(0x8)
	m.Functions[targetAddr] = &FunctionInstance{FunctionType: &TypeInstance{}, Index: FunctionIndex(targetAddr)}
	m.Functions[targetAddr2] = &FunctionInstance{FunctionType: &TypeInstance{}, Index: FunctionIndex(targetAddr2)}
	m.applyElements([]*ElementSegment{
		{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{targetOffset}}, Init: []uint32{uint32(targetAddr)}},
		{OffsetExpr: &ConstantExpression{Opcode: OpcodeI32Const, Data: []byte{targetOffset2}}, Init: []uint32{targetAddr2}},
	})
	require.Equal(t, FunctionIndex(targetAddr), m.TableInstance.Table[targetOffset].FunctionIndex)
	require.Equal(t, FunctionIndex(targetAddr2), m.TableInstance.Table[targetOffset2].FunctionIndex)
}

func Test_newModuleInstance(t *testing.T) {

}
