package wasm

import (
	"bytes"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModule_resolveImports(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		for _, c := range []struct {
			module        *Module
			externModules map[string]*Module
		}{
			{
				module: &Module{SecImports: []*ImportSegment{
					{Module: "a", Name: "b"},
				}},
			},
			{
				module: &Module{SecImports: []*ImportSegment{
					{Module: "a", Name: "b"},
				}},
				externModules: map[string]*Module{
					"a": {},
				},
			},
			{
				module: &Module{SecImports: []*ImportSegment{
					{Module: "a", Name: "b", Desc: &ImportDesc{}},
				}},
				externModules: map[string]*Module{
					"a": {SecExports: map[string]*ExportSegment{
						"b": {
							Name: "a",
							Desc: &ExportDesc{Kind: 1},
						},
					}},
				},
			},
		} {
			err := c.module.resolveImports(c.externModules)
			assert.Error(t, err)
			t.Log(err)
		}
	})

	t.Run("ok", func(t *testing.T) {
		m := &Module{
			SecImports: []*ImportSegment{
				{Module: "a", Name: "b", Desc: &ImportDesc{Kind: 0x03}},
			},
			IndexSpace: new(ModuleIndexSpace),
		}
		ems := map[string]*Module{
			"a": {
				SecExports: map[string]*ExportSegment{
					"b": {
						Name: "a",
						Desc: &ExportDesc{Kind: 0x03},
					},
				},
				IndexSpace: &ModuleIndexSpace{
					Globals: []*Global{{
						Type: &GlobalType{},
						Val:  1,
					}},
				},
			},
		}

		err := m.resolveImports(ems)
		require.NoError(t, err)
		assert.Equal(t, 1, m.IndexSpace.Globals[0].Val)
	})
}

func TestModule_applyFunctionImport(t *testing.T) {
	t.Run("ok", func(t *testing.T) {
		m := Module{
			SecTypes:   []*FunctionType{{ReturnTypes: []ValueType{ValueTypeF64}}},
			IndexSpace: new(ModuleIndexSpace),
		}
		is := &ImportSegment{Desc: &ImportDesc{TypeIndexPtr: uint32Ptr(0)}}
		em := &Module{IndexSpace: &ModuleIndexSpace{Function: []VirtualMachineFunction{
			&NativeFunction{
				Signature: &FunctionType{ReturnTypes: []ValueType{ValueTypeF64}}},
		}}}
		es := &ExportSegment{Desc: &ExportDesc{}}
		err := m.applyFunctionImport(is, em, es)
		require.NoError(t, err)
		assert.Equal(t, em.IndexSpace.Function[0], m.IndexSpace.Function[0])
	})

	t.Run("error", func(t *testing.T) {
		for _, c := range []struct {
			module          Module
			importSegment   *ImportSegment
			exportedModule  *Module
			exportedSegment *ExportSegment
		}{
			{
				module:          Module{IndexSpace: new(ModuleIndexSpace)},
				exportedModule:  &Module{IndexSpace: new(ModuleIndexSpace)},
				exportedSegment: &ExportSegment{Desc: &ExportDesc{Index: 10}},
			},
			{
				module:          Module{IndexSpace: new(ModuleIndexSpace)},
				exportedModule:  &Module{IndexSpace: new(ModuleIndexSpace)},
				exportedSegment: &ExportSegment{Desc: &ExportDesc{}},
			},
			{
				module:          Module{SecTypes: []*FunctionType{{InputTypes: []ValueType{ValueTypeF64}}}},
				importSegment:   &ImportSegment{Desc: &ImportDesc{TypeIndexPtr: uint32Ptr(0)}},
				exportedModule:  &Module{IndexSpace: &ModuleIndexSpace{Function: []VirtualMachineFunction{&NativeFunction{Signature: &FunctionType{}}}}},
				exportedSegment: &ExportSegment{Desc: &ExportDesc{}},
			},
			{
				module:          Module{SecTypes: []*FunctionType{{ReturnTypes: []ValueType{ValueTypeF64}}}},
				importSegment:   &ImportSegment{Desc: &ImportDesc{TypeIndexPtr: uint32Ptr(0)}},
				exportedModule:  &Module{IndexSpace: &ModuleIndexSpace{Function: []VirtualMachineFunction{&NativeFunction{Signature: &FunctionType{}}}}},
				exportedSegment: &ExportSegment{Desc: &ExportDesc{}},
			},
			{
				module:        Module{SecTypes: []*FunctionType{{}}},
				importSegment: &ImportSegment{Desc: &ImportDesc{TypeIndexPtr: uint32Ptr(0)}},
				exportedModule: &Module{IndexSpace: &ModuleIndexSpace{Function: []VirtualMachineFunction{&NativeFunction{
					Signature: &FunctionType{InputTypes: []ValueType{ValueTypeF64}}}},
				}},
				exportedSegment: &ExportSegment{Desc: &ExportDesc{}},
			},
			{
				module:        Module{SecTypes: []*FunctionType{{}}},
				importSegment: &ImportSegment{Desc: &ImportDesc{TypeIndexPtr: uint32Ptr(0)}},
				exportedModule: &Module{IndexSpace: &ModuleIndexSpace{Function: []VirtualMachineFunction{&NativeFunction{
					Signature: &FunctionType{ReturnTypes: []ValueType{ValueTypeF64}}}},
				}},
				exportedSegment: &ExportSegment{Desc: &ExportDesc{}},
			},
		} {
			assert.Error(t, (&c.module).applyFunctionImport(c.importSegment, c.exportedModule, c.exportedSegment))
		}
	})
}

func TestModule_applyTableImport(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		es := &ExportSegment{Desc: &ExportDesc{Index: 10}}
		em := &Module{IndexSpace: new(ModuleIndexSpace)}
		err := (&Module{}).applyTableImport(em, es)
		assert.Error(t, err)
	})

	t.Run("ok", func(t *testing.T) {
		es := &ExportSegment{Desc: &ExportDesc{}}

		var exp uint32 = 10
		em := &Module{
			IndexSpace: &ModuleIndexSpace{Table: [][]*uint32{{&exp}}},
		}
		m := &Module{IndexSpace: new(ModuleIndexSpace)}
		err := m.applyTableImport(em, es)
		require.NoError(t, err)
		assert.Equal(t, exp, *m.IndexSpace.Table[0][0])
	})
}

func TestModule_applyMemoryImport(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		es := &ExportSegment{Desc: &ExportDesc{Index: 10}}
		em := &Module{IndexSpace: new(ModuleIndexSpace)}
		err := (&Module{}).applyMemoryImport(em, es)
		assert.Error(t, err)
	})

	t.Run("ok", func(t *testing.T) {
		es := &ExportSegment{Desc: &ExportDesc{}}
		em := &Module{
			IndexSpace: &ModuleIndexSpace{Memory: [][]byte{{0x01}}},
		}
		m := &Module{IndexSpace: new(ModuleIndexSpace)}
		err := m.applyMemoryImport(em, es)
		require.NoError(t, err)
		assert.Equal(t, byte(0x01), m.IndexSpace.Memory[0][0])
	})
}

func TestModule_applyGlobalImport(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		for _, c := range []struct {
			exportedModule  *Module
			exportedSegment *ExportSegment
		}{
			{
				exportedModule:  &Module{IndexSpace: new(ModuleIndexSpace)},
				exportedSegment: &ExportSegment{Desc: &ExportDesc{Index: 10}},
			},
			{
				exportedModule: &Module{IndexSpace: &ModuleIndexSpace{Globals: []*Global{{Type: &GlobalType{
					Mutable: true,
				}}}}},
				exportedSegment: &ExportSegment{Desc: &ExportDesc{}},
			},
		} {
			m := Module{}
			assert.Error(t, m.applyGlobalImport(c.exportedModule, c.exportedSegment))
		}
	})

	t.Run("ok", func(t *testing.T) {
		m := Module{IndexSpace: new(ModuleIndexSpace)}
		em := &Module{
			IndexSpace: &ModuleIndexSpace{
				Globals: []*Global{{Type: &GlobalType{}, Val: 1}},
			},
		}
		es := &ExportSegment{Desc: &ExportDesc{}}

		err := m.applyGlobalImport(em, es)
		require.NoError(t, err)
		assert.Equal(t, 1, m.IndexSpace.Globals[0].Val)
	})
}

func TestModule_buildGlobalIndexSpace(t *testing.T) {
	m := &Module{SecGlobals: []*GlobalSegment{{Type: nil, Init: &ConstantExpression{
		optCode: OptCodeI64Const,
		data:    []byte{0x01},
	}}}, IndexSpace: new(ModuleIndexSpace)}
	require.NoError(t, m.buildGlobalIndexSpace())
	assert.Equal(t, &Global{Type: nil, Val: int64(1)}, m.IndexSpace.Globals[0])
}

func TestModule_buildFunctionIndexSpace(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		for _, m := range []*Module{
			{SecFunctions: []uint32{1000}, IndexSpace: new(ModuleIndexSpace)},
			{SecFunctions: []uint32{0}, SecTypes: []*FunctionType{{}}, IndexSpace: new(ModuleIndexSpace)},
		} {
			assert.Error(t, m.buildFunctionIndexSpace())
		}
	})
	t.Run("ok", func(t *testing.T) {
		m := &Module{
			SecTypes:     []*FunctionType{{ReturnTypes: []ValueType{ValueTypeF32}}},
			SecFunctions: []uint32{0},
			SecCodes:     []*CodeSegment{{Body: []byte{0x01}}},
			IndexSpace:   new(ModuleIndexSpace),
		}
		assert.NoError(t, m.buildFunctionIndexSpace())
		f := m.IndexSpace.Function[0].(*NativeFunction)
		assert.Equal(t, ValueTypeF32, f.Signature.ReturnTypes[0])
		assert.Equal(t, byte(0x01), f.Body[0])
	})
}

func TestModule_buildMemoryIndexSpace(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		for _, m := range []*Module{
			{SecData: []*DataSegment{{MemoryIndex: 1}}, IndexSpace: new(ModuleIndexSpace)},
			{SecData: []*DataSegment{{MemoryIndex: 0}}, IndexSpace: &ModuleIndexSpace{
				Memory: [][]byte{{}},
			}},

			{
				SecData:    []*DataSegment{{OffsetExpression: &ConstantExpression{}}},
				SecMemory:  []*MemoryType{{}},
				IndexSpace: &ModuleIndexSpace{Memory: [][]byte{{}}},
			},
			{
				SecData: []*DataSegment{
					{
						OffsetExpression: &ConstantExpression{
							optCode: OptCodeI32Const, data: []byte{0x01},
						},
						Init: []byte{0x01, 0x02},
					},
				},
				SecMemory:  []*MemoryType{{Max: uint32Ptr(0)}},
				IndexSpace: &ModuleIndexSpace{Memory: [][]byte{{}}},
			},
		} {
			err := m.buildMemoryIndexSpace()
			assert.Error(t, err)
			t.Log(err)
		}
	})

	t.Run("ok", func(t *testing.T) {
		for _, c := range []struct {
			m   *Module
			exp [][]byte
		}{
			{
				m: &Module{
					SecData: []*DataSegment{
						{
							OffsetExpression: &ConstantExpression{
								optCode: OptCodeI32Const,
								data:    []byte{0x00},
							},
							Init: []byte{0x01, 0x01},
						},
					},
					SecMemory:  []*MemoryType{{}},
					IndexSpace: &ModuleIndexSpace{Memory: [][]byte{{}}},
				},
				exp: [][]byte{{0x01, 0x01}},
			},
			{
				m: &Module{
					SecData: []*DataSegment{
						{
							OffsetExpression: &ConstantExpression{
								optCode: OptCodeI32Const,
								data:    []byte{0x00},
							},
							Init: []byte{0x01, 0x01},
						},
					},
					SecMemory:  []*MemoryType{{}},
					IndexSpace: &ModuleIndexSpace{Memory: [][]byte{{0x00, 0x00, 0x00}}},
				},
				exp: [][]byte{{0x01, 0x01, 0x00}},
			},
			{
				m: &Module{
					SecData: []*DataSegment{
						{
							OffsetExpression: &ConstantExpression{
								optCode: OptCodeI32Const,
								data:    []byte{0x01},
							},
							Init: []byte{0x01, 0x01},
						},
					},
					SecMemory:  []*MemoryType{{}},
					IndexSpace: &ModuleIndexSpace{Memory: [][]byte{{0x00, 0x00, 0x00}}},
				},
				exp: [][]byte{{0x00, 0x01, 0x01}},
			},
			{
				m: &Module{
					SecData: []*DataSegment{
						{
							OffsetExpression: &ConstantExpression{
								optCode: OptCodeI32Const,
								data:    []byte{0x02},
							},
							Init: []byte{0x01, 0x01},
						},
					},
					SecMemory:  []*MemoryType{{}},
					IndexSpace: &ModuleIndexSpace{Memory: [][]byte{{0x00, 0x00, 0x00}}},
				},
				exp: [][]byte{{0x00, 0x00, 0x01, 0x01}},
			},
			{
				m: &Module{
					SecData: []*DataSegment{
						{
							OffsetExpression: &ConstantExpression{
								optCode: OptCodeI32Const,
								data:    []byte{0x01},
							},
							Init: []byte{0x01, 0x01},
						},
					},
					SecMemory:  []*MemoryType{{}},
					IndexSpace: &ModuleIndexSpace{Memory: [][]byte{{0x00, 0x00, 0x00, 0x00}}},
				},
				exp: [][]byte{{0x00, 0x01, 0x01, 0x00}},
			},
			{
				m: &Module{
					SecData: []*DataSegment{
						{
							OffsetExpression: &ConstantExpression{
								optCode: OptCodeI32Const,
								data:    []byte{0x01},
							},
							Init:        []byte{0x01, 0x01},
							MemoryIndex: 1,
						},
					},
					SecMemory:  []*MemoryType{{}, {}},
					IndexSpace: &ModuleIndexSpace{Memory: [][]byte{{}, {0x00, 0x00, 0x00, 0x00}}},
				},
				exp: [][]byte{{}, {0x00, 0x01, 0x01, 0x00}},
			},
		} {
			require.NoError(t, c.m.buildMemoryIndexSpace())
			assert.Equal(t, c.exp, c.m.IndexSpace.Memory)
		}
	})
}

func TestModule_buildTableIndexSpace(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		for _, m := range []*Module{
			{
				SecElements: []*ElementSegment{{TableIndex: 10}},
				IndexSpace:  new(ModuleIndexSpace),
			},
			{
				SecElements: []*ElementSegment{{TableIndex: 0}},
				IndexSpace:  &ModuleIndexSpace{Table: [][]*uint32{{}}},
			},
			{
				SecElements: []*ElementSegment{{TableIndex: 0, OffsetExpr: &ConstantExpression{}}},
				SecTables:   []*TableType{{}},
				IndexSpace:  &ModuleIndexSpace{Table: [][]*uint32{{}}},
			},
			{
				SecElements: []*ElementSegment{{
					TableIndex: 0,
					OffsetExpr: &ConstantExpression{
						optCode: OptCodeI32Const,
						data:    []byte{0x0},
					},
					Init: []uint32{0x0, 0x0},
				}},
				SecTables: []*TableType{{Limit: &LimitsType{
					Max: uint32Ptr(1),
				}}},
				IndexSpace: &ModuleIndexSpace{Table: [][]*uint32{{}}},
			},
		} {
			err := m.buildTableIndexSpace()
			assert.Error(t, err)
			t.Log(err)
		}
	})

	t.Run("ok", func(t *testing.T) {
		for _, c := range []struct {
			m   *Module
			exp [][]*uint32
		}{
			{
				m: &Module{
					SecElements: []*ElementSegment{{
						TableIndex: 0,
						OffsetExpr: &ConstantExpression{
							optCode: OptCodeI32Const,
							data:    []byte{0x0},
						},
						Init: []uint32{0x1, 0x1},
					}},
					SecTables:  []*TableType{{Limit: &LimitsType{}}},
					IndexSpace: &ModuleIndexSpace{Table: [][]*uint32{{}}},
				},
				exp: [][]*uint32{{uint32Ptr(0x01), uint32Ptr(0x01)}},
			},
			{
				m: &Module{
					SecElements: []*ElementSegment{{
						TableIndex: 0,
						OffsetExpr: &ConstantExpression{
							optCode: OptCodeI32Const,
							data:    []byte{0x0},
						},
						Init: []uint32{0x1, 0x1},
					}},
					SecTables: []*TableType{{Limit: &LimitsType{}}},
					IndexSpace: &ModuleIndexSpace{
						Table: [][]*uint32{{uint32Ptr(0x0), uint32Ptr(0x0)}},
					},
				},
				exp: [][]*uint32{{uint32Ptr(0x01), uint32Ptr(0x01)}},
			},
			{
				m: &Module{
					SecElements: []*ElementSegment{{
						TableIndex: 0,
						OffsetExpr: &ConstantExpression{
							optCode: OptCodeI32Const,
							data:    []byte{0x1},
						},
						Init: []uint32{0x1, 0x1},
					}},
					SecTables: []*TableType{{Limit: &LimitsType{}}},
					IndexSpace: &ModuleIndexSpace{
						Table: [][]*uint32{{nil, uint32Ptr(0x0), uint32Ptr(0x0)}},
					},
				},
				exp: [][]*uint32{{nil, uint32Ptr(0x01), uint32Ptr(0x01)}},
			},
			{
				m: &Module{
					SecElements: []*ElementSegment{{
						TableIndex: 0,
						OffsetExpr: &ConstantExpression{
							optCode: OptCodeI32Const,
							data:    []byte{0x1},
						},
						Init: []uint32{0x1},
					}},
					SecTables: []*TableType{{Limit: &LimitsType{}}},
					IndexSpace: &ModuleIndexSpace{
						Table: [][]*uint32{{nil, nil, nil}},
					},
				},
				exp: [][]*uint32{{nil, uint32Ptr(0x01), nil}},
			},
			{
				m: &Module{
					SecElements: []*ElementSegment{{
						TableIndex: 0,
						OffsetExpr: &ConstantExpression{
							optCode: OptCodeI32Const,
							data:    []byte{0x0},
						},
						Init: []uint32{0x1, 0x2},
					}},
					SecTables: []*TableType{{Limit: &LimitsType{}}},
					IndexSpace: &ModuleIndexSpace{
						Table: [][]*uint32{{}},
					},
				},
				exp: [][]*uint32{{uint32Ptr(0x01), uint32Ptr(0x02)}},
			},
		} {
			require.NoError(t, c.m.buildTableIndexSpace())
			require.Len(t, c.m.IndexSpace.Table, len(c.exp))
			for i, actualTable := range c.m.IndexSpace.Table {
				expTable := c.exp[i]
				require.Len(t, actualTable, len(expTable))
				for i, exp := range expTable {
					if exp == nil {
						assert.Nil(t, actualTable[i])
					} else {
						assert.Equal(t, *exp, *actualTable[i])
					}
				}
			}
		}
	})
}
func TestModule_readBlockType(t *testing.T) {
	for _, c := range []struct {
		bytes []byte
		exp   *FunctionType
	}{
		{bytes: []byte{0x40}, exp: &FunctionType{}},
		{bytes: []byte{0x7f}, exp: &FunctionType{ReturnTypes: []ValueType{ValueTypeI32}}},
		{bytes: []byte{0x7e}, exp: &FunctionType{ReturnTypes: []ValueType{ValueTypeI64}}},
		{bytes: []byte{0x7d}, exp: &FunctionType{ReturnTypes: []ValueType{ValueTypeF32}}},
		{bytes: []byte{0x7c}, exp: &FunctionType{ReturnTypes: []ValueType{ValueTypeF64}}},
	} {
		m := &Module{}
		actual, num, err := m.readBlockType(bytes.NewBuffer(c.bytes))
		require.NoError(t, err)
		assert.Equal(t, uint64(1), num)
		assert.Equal(t, c.exp, actual)
	}
	m := &Module{SecTypes: []*FunctionType{{}, {InputTypes: []ValueType{ValueTypeI32}}}}
	actual, num, err := m.readBlockType(bytes.NewBuffer([]byte{0x01}))
	require.NoError(t, err)
	assert.Equal(t, uint64(1), num)
	assert.Equal(t, &FunctionType{InputTypes: []ValueType{ValueTypeI32}}, actual)
}

func TestModule_parseBlocks(t *testing.T) {
	m := &Module{SecTypes: []*FunctionType{{}, {}}}
	for i, c := range []struct {
		body []byte
		exp  map[uint64]*NativeFunctionBlock
	}{
		{
			body: []byte{byte(OptCodeBlock), 0x1, 0x0, byte(OptCodeEnd)},
			exp: map[uint64]*NativeFunctionBlock{
				0: {
					StartAt:        0,
					EndAt:          3,
					BlockType:      &FunctionType{},
					BlockTypeBytes: 1,
				},
			},
		},
		{
			body: []byte{byte(OptCodeBlock), 0x1, byte(OptCodeI32Load), 0x00, 0x0, byte(OptCodeEnd)},
			exp: map[uint64]*NativeFunctionBlock{
				0: {
					StartAt:        0,
					EndAt:          5,
					BlockType:      &FunctionType{},
					BlockTypeBytes: 1,
				},
			},
		},
		{
			body: []byte{byte(OptCodeBlock), 0x1, byte(OptCodeI64Store32), 0x00, 0x0, byte(OptCodeEnd)},
			exp: map[uint64]*NativeFunctionBlock{
				0: {
					StartAt:        0,
					EndAt:          5,
					BlockType:      &FunctionType{},
					BlockTypeBytes: 1,
				},
			},
		},
		{
			body: []byte{byte(OptCodeBlock), 0x1, byte(OptCodeMemoryGrow), 0x00, byte(OptCodeEnd)},
			exp: map[uint64]*NativeFunctionBlock{
				0: {
					StartAt:        0,
					EndAt:          4,
					BlockType:      &FunctionType{},
					BlockTypeBytes: 1,
				},
			},
		},
		{
			body: []byte{byte(OptCodeBlock), 0x1, byte(OptCodeMemorySize), 0x00, byte(OptCodeEnd)},
			exp: map[uint64]*NativeFunctionBlock{
				0: {
					StartAt:        0,
					EndAt:          4,
					BlockType:      &FunctionType{},
					BlockTypeBytes: 1,
				},
			},
		},
		{
			body: []byte{byte(OptCodeBlock), 0x1, byte(OptCodeI32Const), 0x02, byte(OptCodeEnd)},
			exp: map[uint64]*NativeFunctionBlock{
				0: {
					StartAt:        0,
					EndAt:          4,
					BlockType:      &FunctionType{},
					BlockTypeBytes: 1,
				},
			},
		},
		{
			body: []byte{byte(OptCodeBlock), 0x1, byte(OptCodeI64Const), 0x02, byte(OptCodeEnd)},
			exp: map[uint64]*NativeFunctionBlock{
				0: {
					StartAt:        0,
					EndAt:          4,
					BlockType:      &FunctionType{},
					BlockTypeBytes: 1,
				},
			},
		},
		{
			body: []byte{byte(OptCodeBlock), 0x1,
				byte(OptCodeF32Const), 0x02, 0x02, 0x02, 0x02,
				byte(OptCodeEnd),
			},
			exp: map[uint64]*NativeFunctionBlock{
				0: {
					StartAt:        0,
					EndAt:          7,
					BlockType:      &FunctionType{},
					BlockTypeBytes: 1,
				},
			},
		},
		{
			body: []byte{byte(OptCodeBlock), 0x1,
				byte(OptCodeF64Const), 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02, 0x02,
				byte(OptCodeEnd),
			},
			exp: map[uint64]*NativeFunctionBlock{
				0: {
					StartAt:        0,
					EndAt:          11,
					BlockType:      &FunctionType{},
					BlockTypeBytes: 1,
				},
			},
		},
		{
			body: []byte{byte(OptCodeBlock), 0x1, byte(OptCodeLocalGet), 0x02, byte(OptCodeEnd)},
			exp: map[uint64]*NativeFunctionBlock{
				0: {
					StartAt:        0,
					EndAt:          4,
					BlockType:      &FunctionType{},
					BlockTypeBytes: 1,
				},
			},
		},
		{
			body: []byte{byte(OptCodeBlock), 0x1, byte(OptCodeGlobalSet), 0x03, byte(OptCodeEnd)},
			exp: map[uint64]*NativeFunctionBlock{
				0: {
					StartAt:        0,
					EndAt:          4,
					BlockType:      &FunctionType{},
					BlockTypeBytes: 1,
				},
			},
		},
		{
			body: []byte{byte(OptCodeBlock), 0x1, byte(OptCodeGlobalSet), 0x03, byte(OptCodeEnd)},
			exp: map[uint64]*NativeFunctionBlock{
				0: {
					StartAt:        0,
					EndAt:          4,
					BlockType:      &FunctionType{},
					BlockTypeBytes: 1,
				},
			},
		},
		{
			body: []byte{byte(OptCodeBlock), 0x1, byte(OptCodeBr), 0x03, byte(OptCodeEnd)},
			exp: map[uint64]*NativeFunctionBlock{
				0: {
					StartAt:        0,
					EndAt:          4,
					BlockType:      &FunctionType{},
					BlockTypeBytes: 1,
				},
			},
		},
		{
			body: []byte{byte(OptCodeBlock), 0x1, byte(OptCodeBrIf), 0x03, byte(OptCodeEnd)},
			exp: map[uint64]*NativeFunctionBlock{
				0: {
					StartAt:        0,
					EndAt:          4,
					BlockType:      &FunctionType{},
					BlockTypeBytes: 1,
				},
			},
		},
		{
			body: []byte{byte(OptCodeBlock), 0x1, byte(OptCodeCall), 0x03, byte(OptCodeEnd)},
			exp: map[uint64]*NativeFunctionBlock{
				0: {
					StartAt:        0,
					EndAt:          4,
					BlockType:      &FunctionType{},
					BlockTypeBytes: 1,
				},
			},
		},
		{
			body: []byte{byte(OptCodeBlock), 0x1, byte(OptCodeCallIndirect), 0x03, 0x00, byte(OptCodeEnd)},
			exp: map[uint64]*NativeFunctionBlock{
				0: {
					StartAt:        0,
					EndAt:          5,
					BlockType:      &FunctionType{},
					BlockTypeBytes: 1,
				},
			},
		},
		{
			body: []byte{byte(OptCodeBlock), 0x1, byte(OptCodeBrTable),
				0x03, 0x01, 0x01, 0x01, 0x01, byte(OptCodeEnd),
			},
			exp: map[uint64]*NativeFunctionBlock{
				0: {
					StartAt:        0,
					EndAt:          8,
					BlockType:      &FunctionType{},
					BlockTypeBytes: 1,
				},
			},
		},
		{
			body: []byte{byte(OptCodeNop),
				byte(OptCodeBlock), 0x1, byte(OptCodeCallIndirect), 0x03, 0x00, byte(OptCodeEnd),
				byte(OptCodeIf), 0x1, byte(OptCodeLocalGet), 0x02,
				byte(OptCodeElse), byte(OptCodeLocalGet), 0x02,
				byte(OptCodeEnd),
			},
			exp: map[uint64]*NativeFunctionBlock{
				1: {
					StartAt:        1,
					EndAt:          6,
					BlockType:      &FunctionType{},
					BlockTypeBytes: 1,
				},
				7: {
					StartAt:        7,
					ElseAt:         11,
					EndAt:          14,
					BlockType:      &FunctionType{},
					BlockTypeBytes: 1,
				},
			},
		},
		{
			body: []byte{byte(OptCodeNop),
				byte(OptCodeBlock), 0x1, byte(OptCodeCallIndirect), 0x03, 0x00, byte(OptCodeEnd),
				byte(OptCodeIf), 0x1, byte(OptCodeLocalGet), 0x02,
				byte(OptCodeElse), byte(OptCodeLocalGet), 0x02,
				byte(OptCodeIf), 0x01, byte(OptCodeLocalGet), 0x02, byte(OptCodeEnd),
				byte(OptCodeEnd),
			},
			exp: map[uint64]*NativeFunctionBlock{
				1: {
					StartAt:        1,
					EndAt:          6,
					BlockType:      &FunctionType{},
					BlockTypeBytes: 1,
				},
				7: {
					StartAt:        7,
					ElseAt:         11,
					EndAt:          19,
					BlockType:      &FunctionType{},
					BlockTypeBytes: 1,
				},
				14: {
					StartAt:        14,
					EndAt:          18,
					BlockType:      &FunctionType{},
					BlockTypeBytes: 1,
				},
			},
		},
	} {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			actual, err := m.parseBlocks(c.body)
			require.NoError(t, err)
			assert.Equal(t, c.exp, actual)
		})
	}
}
