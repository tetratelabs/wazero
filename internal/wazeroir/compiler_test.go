package wazeroir

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

var (
	f32, f64, i32 = wasm.ValueTypeF32, wasm.ValueTypeF64, wasm.ValueTypeI32
	f32_i32       = wasm.FunctionType{
		Params: []wasm.ValueType{f32}, Results: []wasm.ValueType{i32},
		ParamNumInUint64:  1,
		ResultNumInUint64: 1,
	}
	i32_i32 = wasm.FunctionType{
		Params: []wasm.ValueType{i32}, Results: []wasm.ValueType{i32},
		ParamNumInUint64:  1,
		ResultNumInUint64: 1,
	}
	i32i32_i32 = wasm.FunctionType{
		Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32},
		ParamNumInUint64:  2,
		ResultNumInUint64: 1,
	}
	v_v      = wasm.FunctionType{}
	v_f64f64 = wasm.FunctionType{Results: []wasm.ValueType{f64, f64}, ResultNumInUint64: 2}
)

func TestCompile(t *testing.T) {
	tests := []struct {
		name            string
		module          *wasm.Module
		expected        *CompilationResult
		enabledFeatures api.CoreFeatures
	}{
		{
			name: "nullary",
			module: &wasm.Module{
				TypeSection:     []wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []wasm.Code{{Body: []byte{wasm.OpcodeEnd}}},
			},
			expected: &CompilationResult{
				Operations: []Operation{ // begin with params: []
					OperationBr{Target: Label{Kind: LabelKindReturn}}, // return!
				},
				LabelCallers: map[LabelID]uint32{},
				Functions:    []uint32{0},
				Types:        []wasm.FunctionType{v_v},
				Signature:    &v_v,
				TableTypes:   []wasm.RefType{},
			},
		},
		{
			name: "host wasm nullary",
			module: &wasm.Module{
				TypeSection:     []wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []wasm.Code{{Body: []byte{wasm.OpcodeEnd}}},
			},
			expected: &CompilationResult{
				Operations: []Operation{ // begin with params: []
					OperationBr{Target: Label{Kind: LabelKindReturn}}, // return!
				},
				LabelCallers: map[LabelID]uint32{},
				Functions:    []uint32{0},
				Types:        []wasm.FunctionType{v_v},
				Signature:    &v_v,
				TableTypes:   []wasm.RefType{},
			},
		},
		{
			name: "host go nullary",
			module: &wasm.Module{
				TypeSection:     []wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []wasm.Code{wasm.MustParseGoReflectFuncCode(func() {})},
			},
			expected: &CompilationResult{},
		},
		{
			name: "host go context.Context api.Module uses memory",
			module: &wasm.Module{
				TypeSection:     []wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []wasm.Code{wasm.MustParseGoReflectFuncCode(func(context.Context, api.Module) {})},
			},
			expected: &CompilationResult{UsesMemory: true},
		},
		{
			name: "identity",
			module: &wasm.Module{
				TypeSection:     []wasm.FunctionType{i32_i32},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []wasm.Code{{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeEnd}}},
			},
			expected: &CompilationResult{
				Operations: []Operation{ // begin with params: [$x]
					OperationPick{Depth: 0},                                 // [$x, $x]
					OperationDrop{Depth: &InclusiveRange{Start: 1, End: 1}}, // [$x]
					OperationBr{Target: Label{Kind: LabelKindReturn}},       // return!
				},
				LabelCallers: map[LabelID]uint32{},
				Types: []wasm.FunctionType{
					{
						Params: []wasm.ValueType{i32}, Results: []wasm.ValueType{i32},
						ParamNumInUint64:  1,
						ResultNumInUint64: 1,
					},
				},
				Functions: []uint32{0},
				Signature: &wasm.FunctionType{
					Params: []wasm.ValueType{wasm.ValueTypeI32}, Results: []wasm.ValueType{wasm.ValueTypeI32},
					ParamNumInUint64:  1,
					ResultNumInUint64: 1,
				},
				TableTypes: []wasm.RefType{},
			},
		},
		{
			name: "uses memory",
			module: &wasm.Module{
				TypeSection:     []wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0},
				CodeSection: []wasm.Code{{Body: []byte{
					wasm.OpcodeI32Const, 8, // memory offset to load
					wasm.OpcodeI32Load, 0x2, 0x0, // load alignment=2 (natural alignment) staticOffset=0
					wasm.OpcodeDrop,
					wasm.OpcodeEnd,
				}}},
			},
			expected: &CompilationResult{
				Operations: []Operation{ // begin with params: []
					OperationConstI32{Value: 8}, // [8]
					OperationLoad{Type: UnsignedTypeI32, Arg: MemoryArg{Alignment: 2, Offset: 0}}, // [x]
					OperationDrop{Depth: &InclusiveRange{}},                                       // []
					OperationBr{Target: Label{Kind: LabelKindReturn}},                             // return!
				},
				LabelCallers: map[LabelID]uint32{},
				Types:        []wasm.FunctionType{v_v},
				Functions:    []uint32{0},
				Signature:    &v_v,
				TableTypes:   []wasm.RefType{},
				UsesMemory:   true,
			},
		},
		{
			name: "host uses memory",
			module: &wasm.Module{
				TypeSection:     []wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0},
				CodeSection: []wasm.Code{{Body: []byte{
					wasm.OpcodeI32Const, 8, // memory offset to load
					wasm.OpcodeI32Load, 0x2, 0x0, // load alignment=2 (natural alignment) staticOffset=0
					wasm.OpcodeDrop,
					wasm.OpcodeEnd,
				}}},
			},
			expected: &CompilationResult{
				Operations: []Operation{ // begin with params: []
					OperationConstI32{Value: 8}, // [8]
					OperationLoad{Type: UnsignedTypeI32, Arg: MemoryArg{Alignment: 2, Offset: 0}}, // [x]
					OperationDrop{Depth: &InclusiveRange{}},                                       // []
					OperationBr{Target: Label{Kind: LabelKindReturn}},                             // return!
				},
				LabelCallers: map[LabelID]uint32{},
				Types:        []wasm.FunctionType{v_v},
				Functions:    []uint32{0},
				Signature:    &v_v,
				TableTypes:   []wasm.RefType{},
				UsesMemory:   true,
			},
		},
		{
			name: "memory.grow", // Ex to expose ops to grow memory
			module: &wasm.Module{
				TypeSection:     []wasm.FunctionType{i32_i32},
				FunctionSection: []wasm.Index{0},
				CodeSection: []wasm.Code{{Body: []byte{
					wasm.OpcodeLocalGet, 0, wasm.OpcodeMemoryGrow, 0, wasm.OpcodeEnd,
				}}},
			},
			expected: &CompilationResult{
				Operations: []Operation{ // begin with params: [$delta]
					OperationPick{Depth: 0}, // [$delta, $delta]
					OperationMemoryGrow{},   // [$delta, $old_size]
					OperationDrop{Depth: &InclusiveRange{Start: 1, End: 1}}, // [$old_size]
					OperationBr{Target: Label{Kind: LabelKindReturn}},       // return!
				},
				LabelCallers: map[LabelID]uint32{},
				Types: []wasm.FunctionType{{
					Params: []wasm.ValueType{i32}, Results: []wasm.ValueType{i32},
					ParamNumInUint64:  1,
					ResultNumInUint64: 1,
				}},
				Functions: []uint32{0},
				Signature: &wasm.FunctionType{
					Params: []wasm.ValueType{wasm.ValueTypeI32}, Results: []wasm.ValueType{wasm.ValueTypeI32},
					ParamNumInUint64:  1,
					ResultNumInUint64: 1,
				},
				TableTypes: []wasm.RefType{},
				UsesMemory: true,
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			enabledFeatures := tc.enabledFeatures
			if enabledFeatures == 0 {
				enabledFeatures = api.CoreFeaturesV2
			}
			for _, tp := range tc.module.TypeSection {
				tp.CacheNumInUint64()
			}
			res, err := CompileFunctions(enabledFeatures, 0, tc.module, false)
			require.NoError(t, err)

			fn := res[0]
			if fn.GoFunc != nil { // can't compare functions
				// Special case because reflect.Value can't be compared with Equals
				require.Equal(t, tc.expected.UsesMemory, fn.UsesMemory)
				require.Equal(t, &tc.module.CodeSection[0].GoFunc, &fn.GoFunc)
			} else {
				require.Equal(t, tc.expected, fn)
			}
		})
	}
}

func TestCompile_Block(t *testing.T) {
	tests := []struct {
		name            string
		module          *wasm.Module
		expected        *CompilationResult
		enabledFeatures api.CoreFeatures
	}{
		{
			name: "type-i32-i32",
			module: &wasm.Module{
				TypeSection:     []wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0},
				CodeSection: []wasm.Code{{Body: []byte{
					wasm.OpcodeBlock, 0x40,
					wasm.OpcodeBr, 0,
					wasm.OpcodeI32Add,
					wasm.OpcodeDrop,
					wasm.OpcodeEnd,
					wasm.OpcodeEnd,
				}}},
			},
			// Above set manually until the text compiler supports this:
			// (func (export "type-i32-i32") (block (drop (i32.add (br 0)))))
			expected: &CompilationResult{
				Operations: []Operation{ // begin with params: []
					OperationBr{
						Target: Label{FrameID: 2, Kind: LabelKindContinuation}, // arbitrary FrameID
					},
					OperationLabel{
						Label: Label{FrameID: 2, Kind: LabelKindContinuation}, // arbitrary FrameID
					},
					OperationBr{Target: Label{Kind: LabelKindReturn}},
				},
				// Note: i32.add comes after br 0 so is unreachable. Compilation succeeds when it feels like it
				// shouldn't because the br instruction is stack-polymorphic. In other words, (br 0) substitutes for the
				// two i32 parameters to add.
				LabelCallers: map[LabelID]uint32{Label{Kind: LabelKindContinuation, FrameID: 2}.ID(): 1},
				Functions:    []uint32{0},
				Types:        []wasm.FunctionType{v_v},
				Signature:    &v_v,
				TableTypes:   []wasm.RefType{},
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			requireCompilationResult(t, tc.enabledFeatures, tc.expected, tc.module)
		})
	}
}

// TestCompile_BulkMemoryOperations uses the example from the "bulk-memory-operations" overview.
func TestCompile_BulkMemoryOperations(t *testing.T) {
	// Set manually until the text compiler supports this:
	// (module
	//  (memory 1)
	//  (data (i32.const 0) "hello")   ;; data segment 0, is active so always copied
	//  (data "goodbye")               ;; data segment 1, is passive
	//
	//  (func $start
	//    ;; copy data segment 1 into memory 0 (the 0 is implicit)
	//    (memory.init 1
	//      (i32.const 16)    ;; target offset
	//      (i32.const 0)     ;; source offset
	//      (i32.const 7))    ;; length
	//
	//    ;; The memory used by this segment is no longer needed, so this segment can
	//    ;; be dropped.
	//    (data.drop 1)
	//  )
	//  (start $start)
	// )
	two := uint32(2)
	module := &wasm.Module{
		TypeSection:     []wasm.FunctionType{v_v},
		FunctionSection: []wasm.Index{0},
		MemorySection:   &wasm.Memory{Min: 1},
		DataSection: []wasm.DataSegment{
			{
				OffsetExpression: wasm.ConstantExpression{
					Opcode: wasm.OpcodeI32Const,
					Data:   []byte{0x00},
				},
				Init: []byte("hello"),
			},
			{
				Passive: true,
				Init:    []byte("goodbye"),
			},
		},
		DataCountSection: &two,
		CodeSection: []wasm.Code{{Body: []byte{
			wasm.OpcodeI32Const, 16,
			wasm.OpcodeI32Const, 0,
			wasm.OpcodeI32Const, 7,
			wasm.OpcodeMiscPrefix, wasm.OpcodeMiscMemoryInit, 1, 0, // segment 1, memory 0
			wasm.OpcodeMiscPrefix, wasm.OpcodeMiscDataDrop, 1,
			wasm.OpcodeEnd,
		}}},
	}

	expected := &CompilationResult{
		Operations: []Operation{ // begin with params: []
			OperationConstI32{16},                             // [16]
			OperationConstI32{0},                              // [16, 0]
			OperationConstI32{7},                              // [16, 0, 7]
			OperationMemoryInit{1},                            // []
			OperationDataDrop{1},                              // []
			OperationBr{Target: Label{Kind: LabelKindReturn}}, // return!
		},
		HasMemory:        true,
		UsesMemory:       true,
		HasDataInstances: true,
		LabelCallers:     map[LabelID]uint32{},
		Signature:        &v_v,
		Functions:        []wasm.Index{0},
		Types:            []wasm.FunctionType{v_v},
		TableTypes:       []wasm.RefType{},
	}

	res, err := CompileFunctions(api.CoreFeatureBulkMemoryOperations, 0, module, false)
	require.NoError(t, err)
	require.Equal(t, expected, res[0])
}

func TestCompile_MultiValue(t *testing.T) {
	i32i32_i32i32 := wasm.FunctionType{
		Params:            []wasm.ValueType{wasm.ValueTypeI32, wasm.ValueTypeI32},
		Results:           []wasm.ValueType{wasm.ValueTypeI32, wasm.ValueTypeI32},
		ParamNumInUint64:  2,
		ResultNumInUint64: 2,
	}
	_i32i64 := wasm.FunctionType{
		Results:           []wasm.ValueType{wasm.ValueTypeI32, wasm.ValueTypeI64},
		ParamNumInUint64:  0,
		ResultNumInUint64: 2,
	}

	tests := []struct {
		name            string
		module          *wasm.Module
		expected        *CompilationResult
		enabledFeatures api.CoreFeatures
	}{
		{
			name: "swap",
			module: &wasm.Module{
				TypeSection:     []wasm.FunctionType{i32i32_i32i32},
				FunctionSection: []wasm.Index{0},
				CodeSection: []wasm.Code{{Body: []byte{
					// (func (param $x i32) (param $y i32) (result i32 i32) local.get 1 local.get 0)
					wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 0, wasm.OpcodeEnd,
				}}},
			},
			expected: &CompilationResult{
				Operations: []Operation{ // begin with params: [$x, $y]
					OperationPick{Depth: 0},                                 // [$x, $y, $y]
					OperationPick{Depth: 2},                                 // [$x, $y, $y, $x]
					OperationDrop{Depth: &InclusiveRange{Start: 2, End: 3}}, // [$y, $x]
					OperationBr{Target: Label{Kind: LabelKindReturn}},       // return!
				},
				LabelCallers: map[LabelID]uint32{},
				Signature:    &i32i32_i32i32,
				Functions:    []wasm.Index{0},
				Types:        []wasm.FunctionType{i32i32_i32i32},
				TableTypes:   []wasm.RefType{},
			},
		},
		{
			name: "br.wast - type-f64-f64-value",
			module: &wasm.Module{
				TypeSection:     []wasm.FunctionType{v_f64f64},
				FunctionSection: []wasm.Index{0},
				CodeSection: []wasm.Code{{Body: []byte{
					wasm.OpcodeBlock, 0, // (block (result f64 f64)
					wasm.OpcodeF64Const, 0, 0, 0, 0, 0, 0, 0x10, 0x40, // (f64.const 4)
					wasm.OpcodeF64Const, 0, 0, 0, 0, 0, 0, 0x14, 0x40, // (f64.const 5)
					wasm.OpcodeBr, 0,
					wasm.OpcodeF64Add,
					wasm.OpcodeF64Const, 0, 0, 0, 0, 0, 0, 0x18, 0x40, // (f64.const 6)
					wasm.OpcodeEnd,
					wasm.OpcodeEnd,
				}}},
			},
			// Above set manually until the text compiler supports this:
			// (func $type-f64-f64-value (result f64 f64)
			//   (block (result f64 f64)
			//     (f64.add (br 0 (f64.const 4) (f64.const 5))) (f64.const 6)
			//   )
			// )
			expected: &CompilationResult{
				Operations: []Operation{ // begin with params: []
					OperationConstF64{Value: 4}, // [4]
					OperationConstF64{Value: 5}, // [4, 5]
					OperationBr{
						Target: Label{FrameID: 2, Kind: LabelKindContinuation}, // arbitrary FrameID
					},
					OperationLabel{
						Label: Label{FrameID: 2, Kind: LabelKindContinuation}, // arbitrary FrameID
					},
					OperationBr{Target: Label{Kind: LabelKindReturn}}, // return!
				},
				// Note: f64.add comes after br 0 so is unreachable. This is why neither the add, nor its other operand
				// are in the above compilation result.
				LabelCallers: map[LabelID]uint32{Label{Kind: LabelKindContinuation, FrameID: 2}.ID(): 1}, // arbitrary label
				Signature:    &v_f64f64,
				Functions:    []wasm.Index{0},
				Types:        []wasm.FunctionType{v_f64f64},
				TableTypes:   []wasm.RefType{},
			},
		},
		{
			name: "call.wast - $const-i32-i64",
			module: &wasm.Module{
				TypeSection:     []wasm.FunctionType{_i32i64},
				FunctionSection: []wasm.Index{0},
				CodeSection: []wasm.Code{{Body: []byte{
					//  (func $const-i32-i64 (result i32 i64) i32.const 306 i64.const 356)
					wasm.OpcodeI32Const, 0xb2, 0x2, wasm.OpcodeI64Const, 0xe4, 0x2, wasm.OpcodeEnd,
				}}},
			},
			expected: &CompilationResult{
				Operations: []Operation{ // begin with params: []
					OperationConstI32{Value: 306},                     // [306]
					OperationConstI64{Value: 356},                     // [306, 356]
					OperationBr{Target: Label{Kind: LabelKindReturn}}, // return!
				},
				LabelCallers: map[LabelID]uint32{},
				Signature:    &_i32i64,
				Functions:    []wasm.Index{0},
				Types:        []wasm.FunctionType{_i32i64},
				TableTypes:   []wasm.RefType{},
			},
		},
		{
			name: "if.wast - param",
			module: &wasm.Module{
				TypeSection:     []wasm.FunctionType{i32_i32}, // (func (param i32) (result i32)
				FunctionSection: []wasm.Index{0},
				CodeSection: []wasm.Code{{Body: []byte{
					wasm.OpcodeI32Const, 1, // (i32.const 1)
					wasm.OpcodeLocalGet, 0, wasm.OpcodeIf, 0, // (if (param i32) (result i32) (local.get 0)
					wasm.OpcodeI32Const, 2, wasm.OpcodeI32Add, // (then (i32.const 2) (i32.add))
					wasm.OpcodeElse, wasm.OpcodeI32Const, 0x7e, wasm.OpcodeI32Add, // (else (i32.const -2) (i32.add))
					wasm.OpcodeEnd, // )
					wasm.OpcodeEnd, // )
				}}},
			},
			// Above set manually until the text compiler supports this:
			//	(func (export "param") (param i32) (result i32)
			//	  (i32.const 1)
			//	  (if (param i32) (result i32) (local.get 0)
			//	    (then (i32.const 2) (i32.add))
			//	    (else (i32.const -2) (i32.add))
			//	  )
			//	)
			expected: &CompilationResult{
				Operations: []Operation{ // begin with params: [$0]
					OperationConstI32{Value: 1}, // [$0, 1]
					OperationPick{Depth: 1},     // [$0, 1, $0]
					OperationBrIf{ // [$0, 1]
						Then: BranchTargetDrop{Target: Label{FrameID: 2, Kind: LabelKindHeader}},
						Else: BranchTargetDrop{Target: Label{FrameID: 2, Kind: LabelKindElse}},
					},
					OperationLabel{Label: Label{FrameID: 2, Kind: LabelKindHeader}},
					OperationConstI32{Value: 2},      // [$0, 1, 2]
					NewOperationAdd(UnsignedTypeI32), // [$0, 3]
					OperationBr{Target: Label{FrameID: 2, Kind: LabelKindContinuation}},
					OperationLabel{Label: Label{FrameID: 2, Kind: LabelKindElse}},
					OperationConstI32{Value: uint32(api.EncodeI32(-2))}, // [$0, 1, -2]
					NewOperationAdd(UnsignedTypeI32),                    // [$0, -1]
					OperationBr{Target: Label{FrameID: 2, Kind: LabelKindContinuation}},
					OperationLabel{Label: Label{FrameID: 2, Kind: LabelKindContinuation}},
					OperationDrop{Depth: &InclusiveRange{Start: 1, End: 1}}, // .L2 = [3], .L2_else = [-1]
					OperationBr{Target: Label{Kind: LabelKindReturn}},
				},
				LabelCallers: map[LabelID]uint32{
					Label{Kind: LabelKindHeader, FrameID: 2}.ID():       1,
					Label{Kind: LabelKindContinuation, FrameID: 2}.ID(): 2,
					Label{Kind: LabelKindElse, FrameID: 2}.ID():         1,
				},
				Signature:  &i32_i32,
				Functions:  []wasm.Index{0},
				Types:      []wasm.FunctionType{i32_i32},
				TableTypes: []wasm.RefType{},
			},
		},
		{
			name: "if.wast - params",
			module: &wasm.Module{
				TypeSection: []wasm.FunctionType{
					i32_i32,    // (func (param i32) (result i32)
					i32i32_i32, // (if (param i32 i32) (result i32)
				},
				FunctionSection: []wasm.Index{0},
				CodeSection: []wasm.Code{{Body: []byte{
					wasm.OpcodeI32Const, 1, // (i32.const 1)
					wasm.OpcodeI32Const, 2, // (i32.const 2)
					wasm.OpcodeLocalGet, 0, wasm.OpcodeIf, 1, // (if (param i32) (result i32) (local.get 0)
					wasm.OpcodeI32Add,                  // (then (i32.add))
					wasm.OpcodeElse, wasm.OpcodeI32Sub, // (else (i32.sub))
					wasm.OpcodeEnd, // )
					wasm.OpcodeEnd, // )
				}}},
			},
			// Above set manually until the text compiler supports this:
			//	(func (export "params") (param i32) (result i32)
			//	  (i32.const 1)
			//	  (i32.const 2)
			//	  (if (param i32 i32) (result i32) (local.get 0)
			//	    (then (i32.add))
			//	    (else (i32.sub))
			//	  )
			//	)
			expected: &CompilationResult{
				Operations: []Operation{ // begin with params: [$0]
					OperationConstI32{Value: 1}, // [$0, 1]
					OperationConstI32{Value: 2}, // [$0, 1, 2]
					OperationPick{Depth: 2},     // [$0, 1, 2, $0]
					OperationBrIf{ // [$0, 1, 2]
						Then: BranchTargetDrop{Target: Label{FrameID: 2, Kind: LabelKindHeader}},
						Else: BranchTargetDrop{Target: Label{FrameID: 2, Kind: LabelKindElse}},
					},
					OperationLabel{Label: Label{FrameID: 2, Kind: LabelKindHeader}},
					NewOperationAdd(UnsignedTypeI32), // [$0, 3]
					OperationBr{Target: Label{FrameID: 2, Kind: LabelKindContinuation}},
					OperationLabel{Label: Label{FrameID: 2, Kind: LabelKindElse}},
					NewOperationSub(UnsignedTypeI32), // [$0, -1]
					OperationBr{Target: Label{FrameID: 2, Kind: LabelKindContinuation}},
					OperationLabel{Label: Label{FrameID: 2, Kind: LabelKindContinuation}},
					OperationDrop{Depth: &InclusiveRange{Start: 1, End: 1}}, // .L2 = [3], .L2_else = [-1]
					OperationBr{Target: Label{Kind: LabelKindReturn}},
				},
				LabelCallers: map[LabelID]uint32{
					Label{Kind: LabelKindHeader, FrameID: 2}.ID():       1,
					Label{Kind: LabelKindContinuation, FrameID: 2}.ID(): 2,
					Label{Kind: LabelKindElse, FrameID: 2}.ID():         1,
				},
				Signature:  &i32_i32,
				Functions:  []wasm.Index{0},
				Types:      []wasm.FunctionType{i32_i32, i32i32_i32},
				TableTypes: []wasm.RefType{},
			},
		},
		{
			name: "if.wast - params-break",
			module: &wasm.Module{
				TypeSection: []wasm.FunctionType{
					i32_i32,    // (func (param i32) (result i32)
					i32i32_i32, // (if (param i32 i32) (result i32)
				},
				FunctionSection: []wasm.Index{0},
				CodeSection: []wasm.Code{{Body: []byte{
					wasm.OpcodeI32Const, 1, // (i32.const 1)
					wasm.OpcodeI32Const, 2, // (i32.const 2)
					wasm.OpcodeLocalGet, 0, wasm.OpcodeIf, 1, // (if (param i32) (result i32) (local.get 0)
					wasm.OpcodeI32Add, wasm.OpcodeBr, 0, // (then (i32.add) (br 0))
					wasm.OpcodeElse, wasm.OpcodeI32Sub, wasm.OpcodeBr, 0, // (else (i32.sub) (br 0))
					wasm.OpcodeEnd, // )
					wasm.OpcodeEnd, // )
				}}},
			},
			// Above set manually until the text compiler supports this:
			//	(func (export "params-break") (param i32) (result i32)
			//	  (i32.const 1)
			//	  (i32.const 2)
			//	  (if (param i32 i32) (result i32) (local.get 0)
			//	    (then (i32.add) (br 0))
			//	    (else (i32.sub) (br 0))
			//	  )
			//	)
			expected: &CompilationResult{
				Operations: []Operation{ // begin with params: [$0]
					OperationConstI32{Value: 1}, // [$0, 1]
					OperationConstI32{Value: 2}, // [$0, 1, 2]
					OperationPick{Depth: 2},     // [$0, 1, 2, $0]
					OperationBrIf{ // [$0, 1, 2]
						Then: BranchTargetDrop{Target: Label{FrameID: 2, Kind: LabelKindHeader}},
						Else: BranchTargetDrop{Target: Label{FrameID: 2, Kind: LabelKindElse}},
					},
					OperationLabel{Label: Label{FrameID: 2, Kind: LabelKindHeader}},
					NewOperationAdd(UnsignedTypeI32), // [$0, 3]
					OperationBr{Target: Label{FrameID: 2, Kind: LabelKindContinuation}},
					OperationLabel{Label: Label{FrameID: 2, Kind: LabelKindElse}},
					NewOperationSub(UnsignedTypeI32), // [$0, -1]
					OperationBr{Target: Label{FrameID: 2, Kind: LabelKindContinuation}},
					OperationLabel{Label: Label{FrameID: 2, Kind: LabelKindContinuation}},
					OperationDrop{Depth: &InclusiveRange{Start: 1, End: 1}}, // .L2 = [3], .L2_else = [-1]
					OperationBr{Target: Label{Kind: LabelKindReturn}},
				},
				LabelCallers: map[LabelID]uint32{
					Label{Kind: LabelKindHeader, FrameID: 2}.ID():       1,
					Label{Kind: LabelKindContinuation, FrameID: 2}.ID(): 2,
					Label{Kind: LabelKindElse, FrameID: 2}.ID():         1,
				},
				Signature:  &i32_i32,
				Functions:  []wasm.Index{0},
				Types:      []wasm.FunctionType{i32_i32, i32i32_i32},
				TableTypes: []wasm.RefType{},
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			enabledFeatures := tc.enabledFeatures
			if enabledFeatures == 0 {
				enabledFeatures = api.CoreFeaturesV2
			}
			for _, tp := range tc.module.TypeSection {
				tp.CacheNumInUint64()
			}
			res, err := CompileFunctions(enabledFeatures, 0, tc.module, false)
			require.NoError(t, err)
			require.Equal(t, tc.expected, res[0])
		})
	}
}

// TestCompile_NonTrappingFloatToIntConversion picks an arbitrary operator from "nontrapping-float-to-int-conversion".
func TestCompile_NonTrappingFloatToIntConversion(t *testing.T) {
	module := &wasm.Module{
		TypeSection:     []wasm.FunctionType{f32_i32},
		FunctionSection: []wasm.Index{0},
		// (func (param f32) (result i32) local.get 0 i32.trunc_sat_f32_s)
		CodeSection: []wasm.Code{{Body: []byte{
			wasm.OpcodeLocalGet, 0, wasm.OpcodeMiscPrefix, wasm.OpcodeMiscI32TruncSatF32S, wasm.OpcodeEnd,
		}}},
	}

	expected := &CompilationResult{
		Operations: []Operation{ // begin with params: [$0]
			OperationPick{Depth: 0}, // [$0, $0]
			OperationITruncFromF{ // [$0, i32.trunc_sat_f32_s($0)]
				InputType:   Float32,
				OutputType:  SignedInt32,
				NonTrapping: true,
			},
			OperationDrop{Depth: &InclusiveRange{Start: 1, End: 1}}, // [i32.trunc_sat_f32_s($0)]
			OperationBr{Target: Label{Kind: LabelKindReturn}},       // return!
		},
		LabelCallers: map[LabelID]uint32{},
		Signature:    &f32_i32,
		Functions:    []wasm.Index{0},
		Types:        []wasm.FunctionType{f32_i32},
		TableTypes:   []wasm.RefType{},
	}
	for _, tp := range module.TypeSection {
		tp.CacheNumInUint64()
	}
	res, err := CompileFunctions(api.CoreFeatureNonTrappingFloatToIntConversion, 0, module, false)
	require.NoError(t, err)
	require.Equal(t, expected, res[0])
}

// TestCompile_SignExtensionOps picks an arbitrary operator from "sign-extension-ops".
func TestCompile_SignExtensionOps(t *testing.T) {
	module := &wasm.Module{
		TypeSection:     []wasm.FunctionType{i32_i32},
		FunctionSection: []wasm.Index{0},
		CodeSection: []wasm.Code{{Body: []byte{
			wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Extend8S, wasm.OpcodeEnd,
		}}},
	}

	expected := &CompilationResult{
		Operations: []Operation{ // begin with params: [$0]
			OperationPick{Depth: 0},                                 // [$0, $0]
			OperationSignExtend32From8{},                            // [$0, i32.extend8_s($0)]
			OperationDrop{Depth: &InclusiveRange{Start: 1, End: 1}}, // [i32.extend8_s($0)]
			OperationBr{Target: Label{Kind: LabelKindReturn}},       // return!
		},
		LabelCallers: map[LabelID]uint32{},
		Signature:    &i32_i32,
		Functions:    []wasm.Index{0},
		Types:        []wasm.FunctionType{i32_i32},
		TableTypes:   []wasm.RefType{},
	}
	for _, tp := range module.TypeSection {
		tp.CacheNumInUint64()
	}
	res, err := CompileFunctions(api.CoreFeatureSignExtensionOps, 0, module, false)
	require.NoError(t, err)
	require.Equal(t, expected, res[0])
}

func requireCompilationResult(t *testing.T, enabledFeatures api.CoreFeatures, expected *CompilationResult, module *wasm.Module) {
	if enabledFeatures == 0 {
		enabledFeatures = api.CoreFeaturesV2
	}
	res, err := CompileFunctions(enabledFeatures, 0, module, false)
	require.NoError(t, err)
	require.Equal(t, expected, res[0])
}

func TestCompile_CallIndirectNonZeroTableIndex(t *testing.T) {
	module := &wasm.Module{
		TypeSection:     []wasm.FunctionType{v_v, v_v, v_v},
		FunctionSection: []wasm.Index{0},
		CodeSection: []wasm.Code{{Body: []byte{
			wasm.OpcodeI32Const, 0, // call indirect offset
			wasm.OpcodeCallIndirect,
			2, // Type index for call_indirect.
			5, // Non-zero table index for call_indirect.
			wasm.OpcodeEnd,
		}}},
		TableSection: []wasm.Table{
			{Type: wasm.RefTypeExternref},
			{Type: wasm.RefTypeFuncref},
			{Type: wasm.RefTypeFuncref},
			{Type: wasm.RefTypeFuncref},
			{Type: wasm.RefTypeFuncref},
			{Type: wasm.RefTypeFuncref, Min: 100},
		},
	}

	expected := &CompilationResult{
		Operations: []Operation{ // begin with params: []
			OperationConstI32{},
			OperationCallIndirect{TypeIndex: 2, TableIndex: 5},
			OperationBr{Target: Label{Kind: LabelKindReturn}}, // return!
		},
		HasTable:     true,
		LabelCallers: map[LabelID]uint32{},
		Signature:    &v_v,
		Functions:    []wasm.Index{0},
		TableTypes: []wasm.RefType{
			wasm.RefTypeExternref, wasm.RefTypeFuncref, wasm.RefTypeFuncref, wasm.RefTypeFuncref, wasm.RefTypeFuncref, wasm.RefTypeFuncref,
		},
		Types: []wasm.FunctionType{v_v, v_v, v_v},
	}

	res, err := CompileFunctions(api.CoreFeatureBulkMemoryOperations, 0, module, false)
	require.NoError(t, err)
	require.Equal(t, expected, res[0])
}

func TestCompile_Refs(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		expected []Operation
	}{
		{
			name: "ref.func",
			body: []byte{
				wasm.OpcodeRefFunc, 100,
				wasm.OpcodeDrop,
				wasm.OpcodeEnd,
			},
			expected: []Operation{
				OperationRefFunc{FunctionIndex: 100},
				OperationDrop{Depth: &InclusiveRange{Start: 0, End: 0}},
				OperationBr{Target: Label{Kind: LabelKindReturn}}, // return!
			},
		},
		{
			name: "ref.null (externref)",
			body: []byte{
				wasm.OpcodeRefNull, wasm.ValueTypeExternref,
				wasm.OpcodeDrop,
				wasm.OpcodeEnd,
			},
			expected: []Operation{
				OperationConstI64{Value: 0},
				OperationDrop{Depth: &InclusiveRange{Start: 0, End: 0}},
				OperationBr{Target: Label{Kind: LabelKindReturn}}, // return!
			},
		},
		{
			name: "ref.null (funcref)",
			body: []byte{
				wasm.OpcodeRefNull, wasm.ValueTypeFuncref,
				wasm.OpcodeDrop,
				wasm.OpcodeEnd,
			},
			expected: []Operation{
				OperationConstI64{Value: 0},
				OperationDrop{Depth: &InclusiveRange{Start: 0, End: 0}},
				OperationBr{Target: Label{Kind: LabelKindReturn}}, // return!
			},
		},
		{
			name: "ref.is_null",
			body: []byte{
				wasm.OpcodeRefFunc, 100,
				wasm.OpcodeRefIsNull,
				wasm.OpcodeDrop,
				wasm.OpcodeEnd,
			},
			expected: []Operation{
				OperationRefFunc{FunctionIndex: 100},
				NewOperationEqz(UnsignedInt64),
				OperationDrop{Depth: &InclusiveRange{Start: 0, End: 0}},
				OperationBr{Target: Label{Kind: LabelKindReturn}}, // return!
			},
		},
		{
			name: "ref.is_null (externref)",
			body: []byte{
				wasm.OpcodeRefNull, wasm.ValueTypeExternref,
				wasm.OpcodeRefIsNull,
				wasm.OpcodeDrop,
				wasm.OpcodeEnd,
			},
			expected: []Operation{
				OperationConstI64{Value: 0},
				NewOperationEqz(UnsignedInt64),
				OperationDrop{Depth: &InclusiveRange{Start: 0, End: 0}},
				OperationBr{Target: Label{Kind: LabelKindReturn}}, // return!
			},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			module := &wasm.Module{
				TypeSection:     []wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []wasm.Code{{Body: tc.body}},
			}
			res, err := CompileFunctions(api.CoreFeaturesV2, 0, module, false)
			require.NoError(t, err)
			require.Equal(t, tc.expected, res[0].Operations)
		})
	}
}

func TestCompile_TableGetOrSet(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		expected []Operation
	}{
		{
			name: "table.get",
			body: []byte{
				wasm.OpcodeI32Const, 10,
				wasm.OpcodeTableGet, 0,
				wasm.OpcodeDrop,
				wasm.OpcodeEnd,
			},
			expected: []Operation{
				OperationConstI32{Value: 10},
				OperationTableGet{TableIndex: 0},
				OperationDrop{Depth: &InclusiveRange{Start: 0, End: 0}},
				OperationBr{Target: Label{Kind: LabelKindReturn}}, // return!
			},
		},
		{
			name: "table.set (externref)",
			body: []byte{
				wasm.OpcodeI32Const, 10,
				wasm.OpcodeRefNull, wasm.ValueTypeExternref,
				wasm.OpcodeTableSet, 0,
				wasm.OpcodeEnd,
			},
			expected: []Operation{
				OperationConstI32{Value: 10},
				OperationConstI64{Value: 0},
				OperationTableSet{TableIndex: 0},
				OperationBr{Target: Label{Kind: LabelKindReturn}}, // return!
			},
		},
		{
			name: "table.set (funcref)",
			body: []byte{
				wasm.OpcodeI32Const, 10,
				wasm.OpcodeRefFunc, 1,
				wasm.OpcodeTableSet, 0,
				wasm.OpcodeEnd,
			},
			expected: []Operation{
				OperationConstI32{Value: 10},
				OperationRefFunc{FunctionIndex: 1},
				OperationTableSet{TableIndex: 0},
				OperationBr{Target: Label{Kind: LabelKindReturn}}, // return!
			},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			module := &wasm.Module{
				TypeSection:     []wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []wasm.Code{{Body: tc.body}},
				TableSection:    []wasm.Table{{}},
			}
			res, err := CompileFunctions(api.CoreFeaturesV2, 0, module, false)
			require.NoError(t, err)
			require.Equal(t, tc.expected, res[0].Operations)
		})
	}
}

func TestCompile_TableGrowFillSize(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		expected []Operation
	}{
		{
			name: "table.grow",
			body: []byte{
				wasm.OpcodeRefNull, wasm.RefTypeFuncref,
				wasm.OpcodeI32Const, 1,
				wasm.OpcodeMiscPrefix, wasm.OpcodeMiscTableGrow, 1,
				wasm.OpcodeEnd,
			},
			expected: []Operation{
				OperationConstI64{Value: 0}, // Null ref.
				OperationConstI32{Value: 1},
				OperationTableGrow{TableIndex: 1},
				OperationDrop{Depth: &InclusiveRange{Start: 0, End: 0}},
				OperationBr{Target: Label{Kind: LabelKindReturn}}, // return!
			},
		},
		{
			name: "table.fill",
			body: []byte{
				wasm.OpcodeI32Const, 10,
				wasm.OpcodeRefNull, wasm.RefTypeFuncref,
				wasm.OpcodeI32Const, 1,
				wasm.OpcodeMiscPrefix, wasm.OpcodeMiscTableFill, 1,
				wasm.OpcodeEnd,
			},
			expected: []Operation{
				OperationConstI32{Value: 10},
				OperationConstI64{Value: 0}, // Null ref.
				OperationConstI32{Value: 1},
				OperationTableFill{TableIndex: 1},
				OperationBr{Target: Label{Kind: LabelKindReturn}}, // return!
			},
		},
		{
			name: "table.size",
			body: []byte{
				wasm.OpcodeMiscPrefix, wasm.OpcodeMiscTableSize, 1,
				wasm.OpcodeEnd,
			},
			expected: []Operation{
				OperationTableSize{TableIndex: 1},
				OperationDrop{Depth: &InclusiveRange{Start: 0, End: 0}},
				OperationBr{Target: Label{Kind: LabelKindReturn}}, // return!
			},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			module := &wasm.Module{
				TypeSection:     []wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []wasm.Code{{Body: tc.body}},
				TableSection:    []wasm.Table{{}},
			}
			res, err := CompileFunctions(api.CoreFeaturesV2, 0, module, false)
			require.NoError(t, err)
			require.Equal(t, tc.expected, res[0].Operations)
			require.True(t, res[0].HasTable)
		})
	}
}

func TestCompile_Locals(t *testing.T) {
	tests := []struct {
		name     string
		mod      *wasm.Module
		expected []Operation
	}{
		{
			name: "local.get - func param - v128",
			mod: &wasm.Module{
				TypeSection:     []wasm.FunctionType{{Params: []wasm.ValueType{wasm.ValueTypeV128}}},
				FunctionSection: []wasm.Index{0},
				CodeSection: []wasm.Code{{Body: []byte{
					wasm.OpcodeLocalGet, 0,
					wasm.OpcodeEnd,
				}}},
			},
			expected: []Operation{
				OperationPick{Depth: 1, IsTargetVector: true}, // [param[0].low, param[0].high] -> [param[0].low, param[0].high, param[0].low, param[0].high]
				OperationDrop{Depth: &InclusiveRange{Start: 0, End: 3}},
				OperationBr{Target: Label{Kind: LabelKindReturn}}, // return!
			},
		},
		{
			name: "local.get - func param - i64",
			mod: &wasm.Module{
				TypeSection:     []wasm.FunctionType{{Params: []wasm.ValueType{wasm.ValueTypeI64}}},
				FunctionSection: []wasm.Index{0},
				CodeSection: []wasm.Code{{Body: []byte{
					wasm.OpcodeLocalGet, 0,
					wasm.OpcodeEnd,
				}}},
			},
			expected: []Operation{
				OperationPick{Depth: 0, IsTargetVector: false}, // [param[0]] -> [param[0], param[0]]
				OperationDrop{Depth: &InclusiveRange{Start: 0, End: 1}},
				OperationBr{Target: Label{Kind: LabelKindReturn}}, // return!
			},
		},
		{
			name: "local.get - non func param - v128",
			mod: &wasm.Module{
				TypeSection:     []wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0},
				CodeSection: []wasm.Code{{
					Body: []byte{
						wasm.OpcodeLocalGet, 0,
						wasm.OpcodeEnd,
					},
					LocalTypes: []wasm.ValueType{wasm.ValueTypeV128},
				}},
			},
			expected: []Operation{
				OperationV128Const{Lo: 0, Hi: 0},
				OperationPick{Depth: 1, IsTargetVector: true}, // [p[0].low, p[0].high] -> [p[0].low, p[0].high, p[0].low, p[0].high]
				OperationDrop{Depth: &InclusiveRange{Start: 0, End: 3}},
				OperationBr{Target: Label{Kind: LabelKindReturn}}, // return!
			},
		},
		{
			name: "local.set - func param - v128",
			mod: &wasm.Module{
				TypeSection:     []wasm.FunctionType{{Params: []wasm.ValueType{wasm.ValueTypeV128}}},
				FunctionSection: []wasm.Index{0},
				CodeSection: []wasm.Code{{Body: []byte{
					wasm.OpcodeVecPrefix, wasm.OpcodeVecV128Const, // [] -> [0x01, 0x02]
					1, 0, 0, 0, 0, 0, 0, 0,
					2, 0, 0, 0, 0, 0, 0, 0,
					wasm.OpcodeLocalSet, 0, // [0x01, 0x02] -> []
					wasm.OpcodeEnd,
				}}},
			},
			expected: []Operation{
				// [p[0].lo, p[1].hi] -> [p[0].lo, p[1].hi, 0x01, 0x02]
				OperationV128Const{Lo: 0x01, Hi: 0x02},
				// [p[0].lo, p[1].hi, 0x01, 0x02] -> [0x01, 0x02]
				OperationSet{Depth: 3, IsTargetVector: true},
				OperationDrop{Depth: &InclusiveRange{Start: 0, End: 1}},
				OperationBr{Target: Label{Kind: LabelKindReturn}}, // return!
			},
		},
		{
			name: "local.set - func param - i32",
			mod: &wasm.Module{
				TypeSection:     []wasm.FunctionType{{Params: []wasm.ValueType{wasm.ValueTypeI32}}},
				FunctionSection: []wasm.Index{0},
				CodeSection: []wasm.Code{{Body: []byte{
					wasm.OpcodeI32Const, 0x1, // [] -> [0x01]
					wasm.OpcodeLocalSet, 0, // [0x01] -> []
					wasm.OpcodeEnd,
				}}},
			},
			expected: []Operation{
				OperationConstI32{Value: 0x1},
				OperationSet{Depth: 1, IsTargetVector: false},
				OperationDrop{Depth: &InclusiveRange{Start: 0, End: 0}},
				OperationBr{Target: Label{Kind: LabelKindReturn}}, // return!
			},
		},
		{
			name: "local.set - non func param - v128",
			mod: &wasm.Module{
				TypeSection:     []wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0},
				CodeSection: []wasm.Code{{
					Body: []byte{
						wasm.OpcodeVecPrefix, wasm.OpcodeVecV128Const, // [] -> [0x01, 0x02]
						1, 0, 0, 0, 0, 0, 0, 0,
						2, 0, 0, 0, 0, 0, 0, 0,
						wasm.OpcodeLocalSet, 0, // [0x01, 0x02] -> []
						wasm.OpcodeEnd,
					},
					LocalTypes: []wasm.ValueType{wasm.ValueTypeV128},
				}},
			},
			expected: []Operation{
				OperationV128Const{Lo: 0, Hi: 0},
				// [p[0].lo, p[1].hi] -> [p[0].lo, p[1].hi, 0x01, 0x02]
				OperationV128Const{Lo: 0x01, Hi: 0x02},
				// [p[0].lo, p[1].hi, 0x01, 0x02] -> [0x01, 0x02]
				OperationSet{Depth: 3, IsTargetVector: true},
				OperationDrop{Depth: &InclusiveRange{Start: 0, End: 1}},
				OperationBr{Target: Label{Kind: LabelKindReturn}}, // return!
			},
		},
		{
			name: "local.tee - func param - v128",
			mod: &wasm.Module{
				TypeSection:     []wasm.FunctionType{{Params: []wasm.ValueType{wasm.ValueTypeV128}}},
				FunctionSection: []wasm.Index{0},
				CodeSection: []wasm.Code{{Body: []byte{
					wasm.OpcodeVecPrefix, wasm.OpcodeVecV128Const, // [] -> [0x01, 0x02]
					1, 0, 0, 0, 0, 0, 0, 0,
					2, 0, 0, 0, 0, 0, 0, 0,
					wasm.OpcodeLocalTee, 0, // [0x01, 0x02] ->  [0x01, 0x02]
					wasm.OpcodeEnd,
				}}},
			},
			expected: []Operation{
				// [p[0].lo, p[1].hi] -> [p[0].lo, p[1].hi, 0x01, 0x02]
				OperationV128Const{Lo: 0x01, Hi: 0x02},
				// [p[0].lo, p[1].hi, 0x01, 0x02] -> [p[0].lo, p[1].hi, 0x01, 0x02, 0x01, 0x02]
				OperationPick{Depth: 1, IsTargetVector: true},
				// [p[0].lo, p[1].hi, 0x01, 0x02, 0x01, 0x02] -> [0x01, 0x02, 0x01, 0x02]
				OperationSet{Depth: 5, IsTargetVector: true},
				OperationDrop{Depth: &InclusiveRange{Start: 0, End: 3}},
				OperationBr{Target: Label{Kind: LabelKindReturn}}, // return!
			},
		},
		{
			name: "local.tee - func param - f32",
			mod: &wasm.Module{
				TypeSection:     []wasm.FunctionType{{Params: []wasm.ValueType{wasm.ValueTypeF32}}},
				FunctionSection: []wasm.Index{0},
				CodeSection: []wasm.Code{{Body: []byte{
					wasm.OpcodeF32Const, 1, 0, 0, 0,
					wasm.OpcodeLocalTee, 0, // [0x01, 0x02] ->  [0x01, 0x02]
					wasm.OpcodeEnd,
				}}},
			},
			expected: []Operation{
				OperationConstF32{math.Float32frombits(1)},
				OperationPick{Depth: 0, IsTargetVector: false},
				OperationSet{Depth: 2, IsTargetVector: false},
				OperationDrop{Depth: &InclusiveRange{Start: 0, End: 1}},
				OperationBr{Target: Label{Kind: LabelKindReturn}}, // return!
			},
		},
		{
			name: "local.tee - non func param",
			mod: &wasm.Module{
				TypeSection:     []wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0},
				CodeSection: []wasm.Code{{
					Body: []byte{
						wasm.OpcodeVecPrefix, wasm.OpcodeVecV128Const, // [] -> [0x01, 0x02]
						1, 0, 0, 0, 0, 0, 0, 0,
						2, 0, 0, 0, 0, 0, 0, 0,
						wasm.OpcodeLocalTee, 0, // [0x01, 0x02] ->  [0x01, 0x02]
						wasm.OpcodeEnd,
					},
					LocalTypes: []wasm.ValueType{wasm.ValueTypeV128},
				}},
			},
			expected: []Operation{
				OperationV128Const{Lo: 0, Hi: 0},
				// [p[0].lo, p[1].hi] -> [p[0].lo, p[1].hi, 0x01, 0x02]
				OperationV128Const{Lo: 0x01, Hi: 0x02},
				// [p[0].lo, p[1].hi, 0x01, 0x02] -> [p[0].lo, p[1].hi, 0x01, 0x02, 0x01, 0x02]
				OperationPick{Depth: 1, IsTargetVector: true},
				// [p[0].lo, p[1].hi, 0x01, 0x02, 0x01, 0x2] -> [0x01, 0x02, 0x01, 0x02]
				OperationSet{Depth: 5, IsTargetVector: true},
				OperationDrop{Depth: &InclusiveRange{Start: 0, End: 3}},
				OperationBr{Target: Label{Kind: LabelKindReturn}}, // return!
			},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			res, err := CompileFunctions(api.CoreFeaturesV2, 0, tc.mod, false)
			require.NoError(t, err)
			msg := fmt.Sprintf("\nhave:\n\t%s\nwant:\n\t%s", Format(res[0].Operations), Format(tc.expected))
			require.Equal(t, tc.expected, res[0].Operations, msg)
		})
	}
}

func TestCompile_Vec(t *testing.T) {
	addV128Const := func(in []byte) []byte {
		return append(in, wasm.OpcodeVecPrefix,
			wasm.OpcodeVecV128Const,
			1, 1, 1, 1, 1, 1, 1, 1,
			1, 1, 1, 1, 1, 1, 1, 1)
	}

	vvv2v := func(vec wasm.OpcodeVec) (ret []byte) {
		ret = addV128Const(ret)
		ret = addV128Const(ret)
		ret = addV128Const(ret)
		return append(ret, wasm.OpcodeVecPrefix, vec, wasm.OpcodeDrop, wasm.OpcodeEnd)
	}

	vv2v := func(vec wasm.OpcodeVec) (ret []byte) {
		ret = addV128Const(ret)
		ret = addV128Const(ret)
		return append(ret, wasm.OpcodeVecPrefix, vec, wasm.OpcodeDrop, wasm.OpcodeEnd)
	}

	vi2v := func(vec wasm.OpcodeVec) (ret []byte) {
		ret = addV128Const(ret)
		return append(ret, wasm.OpcodeI32Const, 1,
			wasm.OpcodeVecPrefix, vec,
			wasm.OpcodeDrop, wasm.OpcodeEnd)
	}

	v2v := func(vec wasm.OpcodeVec) (ret []byte) {
		ret = addV128Const(ret)
		return append(ret, wasm.OpcodeVecPrefix, vec, wasm.OpcodeDrop, wasm.OpcodeEnd)
	}

	load := func(vec wasm.OpcodeVec, offset, align uint32) (ret []byte) {
		ret = []byte{wasm.OpcodeI32Const, 1, 1, 1, 1, wasm.OpcodeVecPrefix, vec}
		ret = append(ret, leb128.EncodeUint32(align)...)
		ret = append(ret, leb128.EncodeUint32(offset)...)
		ret = append(ret, wasm.OpcodeDrop, wasm.OpcodeEnd)
		return
	}

	loadLane := func(vec wasm.OpcodeVec, offset, align uint32, lane byte) (ret []byte) {
		ret = addV128Const([]byte{wasm.OpcodeI32Const, 1, 1, 1, 1})
		ret = append(ret, wasm.OpcodeVecPrefix, vec)
		ret = append(ret, leb128.EncodeUint32(align)...)
		ret = append(ret, leb128.EncodeUint32(offset)...)
		ret = append(ret, lane, wasm.OpcodeDrop, wasm.OpcodeEnd)
		return
	}

	storeLane := func(vec wasm.OpcodeVec, offset, align uint32, lane byte) (ret []byte) {
		ret = addV128Const([]byte{wasm.OpcodeI32Const, 0})
		ret = append(ret, wasm.OpcodeVecPrefix, vec)
		ret = append(ret, leb128.EncodeUint32(align)...)
		ret = append(ret, leb128.EncodeUint32(offset)...)
		ret = append(ret, lane, wasm.OpcodeEnd)
		return
	}

	extractLane := func(vec wasm.OpcodeVec, lane byte) (ret []byte) {
		ret = addV128Const(ret)
		ret = append(ret, wasm.OpcodeVecPrefix, vec, lane, wasm.OpcodeDrop, wasm.OpcodeEnd)
		return
	}

	replaceLane := func(vec wasm.OpcodeVec, lane byte) (ret []byte) {
		ret = addV128Const(ret)

		switch vec {
		case wasm.OpcodeVecI8x16ReplaceLane, wasm.OpcodeVecI16x8ReplaceLane, wasm.OpcodeVecI32x4ReplaceLane:
			ret = append(ret, wasm.OpcodeI32Const, 0)
		case wasm.OpcodeVecI64x2ReplaceLane:
			ret = append(ret, wasm.OpcodeI64Const, 0)
		case wasm.OpcodeVecF32x4ReplaceLane:
			ret = append(ret, wasm.OpcodeF32Const, 0, 0, 0, 0)
		case wasm.OpcodeVecF64x2ReplaceLane:
			ret = append(ret, wasm.OpcodeF64Const, 0, 0, 0, 0, 0, 0, 0, 0)
		}

		ret = append(ret, wasm.OpcodeVecPrefix, vec, lane, wasm.OpcodeDrop, wasm.OpcodeEnd)
		return
	}

	splat := func(vec wasm.OpcodeVec) (ret []byte) {
		switch vec {
		case wasm.OpcodeVecI8x16Splat, wasm.OpcodeVecI16x8Splat, wasm.OpcodeVecI32x4Splat:
			ret = append(ret, wasm.OpcodeI32Const, 0)
		case wasm.OpcodeVecI64x2Splat:
			ret = append(ret, wasm.OpcodeI64Const, 0)
		case wasm.OpcodeVecF32x4Splat:
			ret = append(ret, wasm.OpcodeF32Const, 0, 0, 0, 0)
		case wasm.OpcodeVecF64x2Splat:
			ret = append(ret, wasm.OpcodeF64Const, 0, 0, 0, 0, 0, 0, 0, 0)
		}
		ret = append(ret, wasm.OpcodeVecPrefix, vec, wasm.OpcodeDrop, wasm.OpcodeEnd)
		return
	}

	tests := []struct {
		name                 string
		body                 []byte
		expected             Operation
		needDropBeforeReturn bool
	}{
		{
			name:                 "i8x16 add",
			needDropBeforeReturn: true,
			body:                 vv2v(wasm.OpcodeVecI8x16Add),
			expected:             OperationV128Add{Shape: ShapeI8x16},
		},
		{
			name:                 "i8x16 add",
			needDropBeforeReturn: true,
			body:                 vv2v(wasm.OpcodeVecI16x8Add),
			expected:             OperationV128Add{Shape: ShapeI16x8},
		},
		{
			name:                 "i32x2 add",
			needDropBeforeReturn: true,
			body:                 vv2v(wasm.OpcodeVecI64x2Add),
			expected:             OperationV128Add{Shape: ShapeI64x2},
		},
		{
			name:                 "i64x2 add",
			needDropBeforeReturn: true,
			body:                 vv2v(wasm.OpcodeVecI64x2Add),
			expected:             OperationV128Add{Shape: ShapeI64x2},
		},
		{
			name:                 "i8x16 sub",
			needDropBeforeReturn: true,
			body:                 vv2v(wasm.OpcodeVecI8x16Sub),
			expected:             OperationV128Sub{Shape: ShapeI8x16},
		},
		{
			name:                 "i16x8 sub",
			needDropBeforeReturn: true,
			body:                 vv2v(wasm.OpcodeVecI16x8Sub),
			expected:             OperationV128Sub{Shape: ShapeI16x8},
		},
		{
			name:                 "i32x2 sub",
			needDropBeforeReturn: true,
			body:                 vv2v(wasm.OpcodeVecI64x2Sub),
			expected:             OperationV128Sub{Shape: ShapeI64x2},
		},
		{
			name:                 "i64x2 sub",
			needDropBeforeReturn: true,
			body:                 vv2v(wasm.OpcodeVecI64x2Sub),
			expected:             OperationV128Sub{Shape: ShapeI64x2},
		},
		{
			name: wasm.OpcodeVecV128LoadName, body: load(wasm.OpcodeVecV128Load, 0, 0),
			needDropBeforeReturn: true,
			expected:             OperationV128Load{Type: V128LoadType128, Arg: MemoryArg{Alignment: 0, Offset: 0}},
		},
		{
			name: wasm.OpcodeVecV128LoadName + "/align=4", body: load(wasm.OpcodeVecV128Load, 0, 4),
			needDropBeforeReturn: true,
			expected:             OperationV128Load{Type: V128LoadType128, Arg: MemoryArg{Alignment: 4, Offset: 0}},
		},
		{
			name: wasm.OpcodeVecV128Load8x8SName, body: load(wasm.OpcodeVecV128Load8x8s, 1, 0),
			needDropBeforeReturn: true,
			expected:             OperationV128Load{Type: V128LoadType8x8s, Arg: MemoryArg{Alignment: 0, Offset: 1}},
		},
		{
			name: wasm.OpcodeVecV128Load8x8SName + "/align=1", body: load(wasm.OpcodeVecV128Load8x8s, 0, 1),
			needDropBeforeReturn: true,
			expected:             OperationV128Load{Type: V128LoadType8x8s, Arg: MemoryArg{Alignment: 1, Offset: 0}},
		},
		{
			name: wasm.OpcodeVecV128Load8x8UName, body: load(wasm.OpcodeVecV128Load8x8u, 0, 0),
			needDropBeforeReturn: true,
			expected:             OperationV128Load{Type: V128LoadType8x8u, Arg: MemoryArg{Alignment: 0, Offset: 0}},
		},
		{
			name: wasm.OpcodeVecV128Load8x8UName + "/align=1", body: load(wasm.OpcodeVecV128Load8x8u, 0, 1),
			needDropBeforeReturn: true,
			expected:             OperationV128Load{Type: V128LoadType8x8u, Arg: MemoryArg{Alignment: 1, Offset: 0}},
		},
		{
			name: wasm.OpcodeVecV128Load16x4SName, body: load(wasm.OpcodeVecV128Load16x4s, 1, 0),
			needDropBeforeReturn: true,
			expected:             OperationV128Load{Type: V128LoadType16x4s, Arg: MemoryArg{Alignment: 0, Offset: 1}},
		},
		{
			name: wasm.OpcodeVecV128Load16x4SName + "/align=2", body: load(wasm.OpcodeVecV128Load16x4s, 0, 2),
			needDropBeforeReturn: true,
			expected:             OperationV128Load{Type: V128LoadType16x4s, Arg: MemoryArg{Alignment: 2, Offset: 0}},
		},
		{
			name: wasm.OpcodeVecV128Load16x4UName, body: load(wasm.OpcodeVecV128Load16x4u, 0, 0),
			needDropBeforeReturn: true,
			expected:             OperationV128Load{Type: V128LoadType16x4u, Arg: MemoryArg{Alignment: 0, Offset: 0}},
		},
		{
			name: wasm.OpcodeVecV128Load16x4UName + "/align=2", body: load(wasm.OpcodeVecV128Load16x4u, 0, 2),
			needDropBeforeReturn: true,
			expected:             OperationV128Load{Type: V128LoadType16x4u, Arg: MemoryArg{Alignment: 2, Offset: 0}},
		},
		{
			name: wasm.OpcodeVecV128Load32x2SName, body: load(wasm.OpcodeVecV128Load32x2s, 1, 0),
			needDropBeforeReturn: true,
			expected:             OperationV128Load{Type: V128LoadType32x2s, Arg: MemoryArg{Alignment: 0, Offset: 1}},
		},
		{
			name: wasm.OpcodeVecV128Load32x2SName + "/align=3", body: load(wasm.OpcodeVecV128Load32x2s, 0, 3),
			needDropBeforeReturn: true,
			expected:             OperationV128Load{Type: V128LoadType32x2s, Arg: MemoryArg{Alignment: 3, Offset: 0}},
		},
		{
			name: wasm.OpcodeVecV128Load32x2UName, body: load(wasm.OpcodeVecV128Load32x2u, 0, 0),
			needDropBeforeReturn: true,
			expected:             OperationV128Load{Type: V128LoadType32x2u, Arg: MemoryArg{Alignment: 0, Offset: 0}},
		},
		{
			name: wasm.OpcodeVecV128Load32x2UName + "/align=3", body: load(wasm.OpcodeVecV128Load32x2u, 0, 3),
			needDropBeforeReturn: true,
			expected:             OperationV128Load{Type: V128LoadType32x2u, Arg: MemoryArg{Alignment: 3, Offset: 0}},
		},
		{
			name: wasm.OpcodeVecV128Load8SplatName, body: load(wasm.OpcodeVecV128Load8Splat, 2, 0),
			needDropBeforeReturn: true,
			expected:             OperationV128Load{Type: V128LoadType8Splat, Arg: MemoryArg{Alignment: 0, Offset: 2}},
		},
		{
			name: wasm.OpcodeVecV128Load16SplatName, body: load(wasm.OpcodeVecV128Load16Splat, 0, 1),
			needDropBeforeReturn: true,
			expected:             OperationV128Load{Type: V128LoadType16Splat, Arg: MemoryArg{Alignment: 1, Offset: 0}},
		},
		{
			name: wasm.OpcodeVecV128Load32SplatName, body: load(wasm.OpcodeVecV128Load32Splat, 3, 2),
			needDropBeforeReturn: true,
			expected:             OperationV128Load{Type: V128LoadType32Splat, Arg: MemoryArg{Alignment: 2, Offset: 3}},
		},
		{
			name: wasm.OpcodeVecV128Load64SplatName, body: load(wasm.OpcodeVecV128Load64Splat, 0, 3),
			needDropBeforeReturn: true,
			expected:             OperationV128Load{Type: V128LoadType64Splat, Arg: MemoryArg{Alignment: 3, Offset: 0}},
		},
		{
			name: wasm.OpcodeVecV128Load32zeroName, body: load(wasm.OpcodeVecV128Load32zero, 0, 2),
			needDropBeforeReturn: true,
			expected:             OperationV128Load{Type: V128LoadType32zero, Arg: MemoryArg{Alignment: 2, Offset: 0}},
		},
		{
			name: wasm.OpcodeVecV128Load64zeroName, body: load(wasm.OpcodeVecV128Load64zero, 5, 3),
			needDropBeforeReturn: true,
			expected:             OperationV128Load{Type: V128LoadType64zero, Arg: MemoryArg{Alignment: 3, Offset: 5}},
		},
		{
			name: wasm.OpcodeVecV128Load8LaneName, needDropBeforeReturn: true,
			body:     loadLane(wasm.OpcodeVecV128Load8Lane, 5, 0, 10),
			expected: OperationV128LoadLane{LaneIndex: 10, LaneSize: 8, Arg: MemoryArg{Alignment: 0, Offset: 5}},
		},
		{
			name: wasm.OpcodeVecV128Load16LaneName, needDropBeforeReturn: true,
			body:     loadLane(wasm.OpcodeVecV128Load16Lane, 100, 1, 7),
			expected: OperationV128LoadLane{LaneIndex: 7, LaneSize: 16, Arg: MemoryArg{Alignment: 1, Offset: 100}},
		},
		{
			name: wasm.OpcodeVecV128Load32LaneName, needDropBeforeReturn: true,
			body:     loadLane(wasm.OpcodeVecV128Load32Lane, 0, 2, 3),
			expected: OperationV128LoadLane{LaneIndex: 3, LaneSize: 32, Arg: MemoryArg{Alignment: 2, Offset: 0}},
		},
		{
			name: wasm.OpcodeVecV128Load64LaneName, needDropBeforeReturn: true,
			body:     loadLane(wasm.OpcodeVecV128Load64Lane, 0, 3, 1),
			expected: OperationV128LoadLane{LaneIndex: 1, LaneSize: 64, Arg: MemoryArg{Alignment: 3, Offset: 0}},
		},
		{
			name: wasm.OpcodeVecV128StoreName, body: []byte{
				wasm.OpcodeI32Const,
				1, 1, 1, 1,
				wasm.OpcodeVecPrefix,
				wasm.OpcodeVecV128Const,
				1, 1, 1, 1, 1, 1, 1, 1,
				1, 1, 1, 1, 1, 1, 1, 1,
				wasm.OpcodeVecPrefix,
				wasm.OpcodeVecV128Store,
				4,  // alignment
				10, // offset
				wasm.OpcodeEnd,
			},
			expected: OperationV128Store{Arg: MemoryArg{Alignment: 4, Offset: 10}},
		},
		{
			name:     wasm.OpcodeVecV128Store8LaneName,
			body:     storeLane(wasm.OpcodeVecV128Store8Lane, 0, 0, 0),
			expected: OperationV128StoreLane{LaneIndex: 0, LaneSize: 8, Arg: MemoryArg{Alignment: 0, Offset: 0}},
		},
		{
			name:     wasm.OpcodeVecV128Store8LaneName + "/lane=15",
			body:     storeLane(wasm.OpcodeVecV128Store8Lane, 100, 0, 15),
			expected: OperationV128StoreLane{LaneIndex: 15, LaneSize: 8, Arg: MemoryArg{Alignment: 0, Offset: 100}},
		},
		{
			name:     wasm.OpcodeVecV128Store16LaneName,
			body:     storeLane(wasm.OpcodeVecV128Store16Lane, 0, 0, 0),
			expected: OperationV128StoreLane{LaneIndex: 0, LaneSize: 16, Arg: MemoryArg{Alignment: 0, Offset: 0}},
		},
		{
			name:     wasm.OpcodeVecV128Store16LaneName + "/lane=7/align=1",
			body:     storeLane(wasm.OpcodeVecV128Store16Lane, 100, 1, 7),
			expected: OperationV128StoreLane{LaneIndex: 7, LaneSize: 16, Arg: MemoryArg{Alignment: 1, Offset: 100}},
		},
		{
			name:     wasm.OpcodeVecV128Store32LaneName,
			body:     storeLane(wasm.OpcodeVecV128Store32Lane, 0, 0, 0),
			expected: OperationV128StoreLane{LaneIndex: 0, LaneSize: 32, Arg: MemoryArg{Alignment: 0, Offset: 0}},
		},
		{
			name:     wasm.OpcodeVecV128Store32LaneName + "/lane=3/align=2",
			body:     storeLane(wasm.OpcodeVecV128Store32Lane, 100, 2, 3),
			expected: OperationV128StoreLane{LaneIndex: 3, LaneSize: 32, Arg: MemoryArg{Alignment: 2, Offset: 100}},
		},
		{
			name:     wasm.OpcodeVecV128Store64LaneName,
			body:     storeLane(wasm.OpcodeVecV128Store64Lane, 0, 0, 0),
			expected: OperationV128StoreLane{LaneIndex: 0, LaneSize: 64, Arg: MemoryArg{Alignment: 0, Offset: 0}},
		},
		{
			name:     wasm.OpcodeVecV128Store64LaneName + "/lane=1/align=3",
			body:     storeLane(wasm.OpcodeVecV128Store64Lane, 50, 3, 1),
			expected: OperationV128StoreLane{LaneIndex: 1, LaneSize: 64, Arg: MemoryArg{Alignment: 3, Offset: 50}},
		},
		{
			name:                 wasm.OpcodeVecI8x16ExtractLaneSName,
			body:                 extractLane(wasm.OpcodeVecI8x16ExtractLaneS, 0),
			expected:             OperationV128ExtractLane{LaneIndex: 0, Signed: true, Shape: ShapeI8x16},
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI8x16ExtractLaneSName + "/lane=15",
			body:                 extractLane(wasm.OpcodeVecI8x16ExtractLaneS, 15),
			expected:             OperationV128ExtractLane{LaneIndex: 15, Signed: true, Shape: ShapeI8x16},
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI8x16ExtractLaneUName,
			body:                 extractLane(wasm.OpcodeVecI8x16ExtractLaneU, 0),
			expected:             OperationV128ExtractLane{LaneIndex: 0, Signed: false, Shape: ShapeI8x16},
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI8x16ExtractLaneUName + "/lane=15",
			body:                 extractLane(wasm.OpcodeVecI8x16ExtractLaneU, 15),
			expected:             OperationV128ExtractLane{LaneIndex: 15, Signed: false, Shape: ShapeI8x16},
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI16x8ExtractLaneSName,
			body:                 extractLane(wasm.OpcodeVecI16x8ExtractLaneS, 0),
			expected:             OperationV128ExtractLane{LaneIndex: 0, Signed: true, Shape: ShapeI16x8},
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI16x8ExtractLaneSName + "/lane=7",
			body:                 extractLane(wasm.OpcodeVecI16x8ExtractLaneS, 7),
			expected:             OperationV128ExtractLane{LaneIndex: 7, Signed: true, Shape: ShapeI16x8},
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI16x8ExtractLaneUName,
			body:                 extractLane(wasm.OpcodeVecI16x8ExtractLaneU, 0),
			expected:             OperationV128ExtractLane{LaneIndex: 0, Signed: false, Shape: ShapeI16x8},
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI16x8ExtractLaneUName + "/lane=7",
			body:                 extractLane(wasm.OpcodeVecI16x8ExtractLaneU, 7),
			expected:             OperationV128ExtractLane{LaneIndex: 7, Signed: false, Shape: ShapeI16x8},
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI32x4ExtractLaneName,
			body:                 extractLane(wasm.OpcodeVecI32x4ExtractLane, 0),
			expected:             OperationV128ExtractLane{LaneIndex: 0, Shape: ShapeI32x4},
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI32x4ExtractLaneName + "/lane=3",
			body:                 extractLane(wasm.OpcodeVecI32x4ExtractLane, 3),
			expected:             OperationV128ExtractLane{LaneIndex: 3, Shape: ShapeI32x4},
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI64x2ExtractLaneName,
			body:                 extractLane(wasm.OpcodeVecI64x2ExtractLane, 0),
			expected:             OperationV128ExtractLane{LaneIndex: 0, Shape: ShapeI64x2},
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI64x2ExtractLaneName + "/lane=1",
			body:                 extractLane(wasm.OpcodeVecI64x2ExtractLane, 1),
			expected:             OperationV128ExtractLane{LaneIndex: 1, Shape: ShapeI64x2},
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecF32x4ExtractLaneName,
			body:                 extractLane(wasm.OpcodeVecF32x4ExtractLane, 0),
			expected:             OperationV128ExtractLane{LaneIndex: 0, Shape: ShapeF32x4},
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecF32x4ExtractLaneName + "/lane=3",
			body:                 extractLane(wasm.OpcodeVecF32x4ExtractLane, 3),
			expected:             OperationV128ExtractLane{LaneIndex: 3, Shape: ShapeF32x4},
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecF64x2ExtractLaneName,
			body:                 extractLane(wasm.OpcodeVecF64x2ExtractLane, 0),
			expected:             OperationV128ExtractLane{LaneIndex: 0, Shape: ShapeF64x2},
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecF64x2ExtractLaneName + "/lane=1",
			body:                 extractLane(wasm.OpcodeVecF64x2ExtractLane, 1),
			expected:             OperationV128ExtractLane{LaneIndex: 1, Shape: ShapeF64x2},
			needDropBeforeReturn: true,
		},

		{
			name:                 wasm.OpcodeVecI8x16ReplaceLaneName,
			body:                 replaceLane(wasm.OpcodeVecI8x16ReplaceLane, 0),
			expected:             OperationV128ReplaceLane{LaneIndex: 0, Shape: ShapeI8x16},
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI8x16ReplaceLaneName + "/lane=15",
			body:                 replaceLane(wasm.OpcodeVecI8x16ReplaceLane, 15),
			expected:             OperationV128ReplaceLane{LaneIndex: 15, Shape: ShapeI8x16},
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI16x8ReplaceLaneName,
			body:                 replaceLane(wasm.OpcodeVecI16x8ReplaceLane, 0),
			expected:             OperationV128ReplaceLane{LaneIndex: 0, Shape: ShapeI16x8},
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI16x8ReplaceLaneName + "/lane=7",
			body:                 replaceLane(wasm.OpcodeVecI16x8ReplaceLane, 7),
			expected:             OperationV128ReplaceLane{LaneIndex: 7, Shape: ShapeI16x8},
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI32x4ReplaceLaneName,
			body:                 replaceLane(wasm.OpcodeVecI32x4ReplaceLane, 0),
			expected:             OperationV128ReplaceLane{LaneIndex: 0, Shape: ShapeI32x4},
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI32x4ReplaceLaneName + "/lane=3",
			body:                 replaceLane(wasm.OpcodeVecI32x4ReplaceLane, 3),
			expected:             OperationV128ReplaceLane{LaneIndex: 3, Shape: ShapeI32x4},
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI64x2ReplaceLaneName,
			body:                 replaceLane(wasm.OpcodeVecI64x2ReplaceLane, 0),
			expected:             OperationV128ReplaceLane{LaneIndex: 0, Shape: ShapeI64x2},
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI64x2ReplaceLaneName + "/lane=1",
			body:                 replaceLane(wasm.OpcodeVecI64x2ReplaceLane, 1),
			expected:             OperationV128ReplaceLane{LaneIndex: 1, Shape: ShapeI64x2},
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecF32x4ReplaceLaneName,
			body:                 replaceLane(wasm.OpcodeVecF32x4ReplaceLane, 0),
			expected:             OperationV128ReplaceLane{LaneIndex: 0, Shape: ShapeF32x4},
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecF32x4ReplaceLaneName + "/lane=3",
			body:                 replaceLane(wasm.OpcodeVecF32x4ReplaceLane, 3),
			expected:             OperationV128ReplaceLane{LaneIndex: 3, Shape: ShapeF32x4},
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecF64x2ReplaceLaneName,
			body:                 replaceLane(wasm.OpcodeVecF64x2ReplaceLane, 0),
			expected:             OperationV128ReplaceLane{LaneIndex: 0, Shape: ShapeF64x2},
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecF64x2ReplaceLaneName + "/lane=1",
			body:                 replaceLane(wasm.OpcodeVecF64x2ReplaceLane, 1),
			expected:             OperationV128ReplaceLane{LaneIndex: 1, Shape: ShapeF64x2},
			needDropBeforeReturn: true,
		},
		{
			name: wasm.OpcodeVecI8x16SplatName, body: splat(wasm.OpcodeVecI8x16Splat),
			expected:             OperationV128Splat{Shape: ShapeI8x16},
			needDropBeforeReturn: true,
		},
		{
			name: wasm.OpcodeVecI16x8SplatName, body: splat(wasm.OpcodeVecI16x8Splat),
			expected:             OperationV128Splat{Shape: ShapeI16x8},
			needDropBeforeReturn: true,
		},
		{
			name: wasm.OpcodeVecI32x4SplatName, body: splat(wasm.OpcodeVecI32x4Splat),
			expected:             OperationV128Splat{Shape: ShapeI32x4},
			needDropBeforeReturn: true,
		},
		{
			name: wasm.OpcodeVecI64x2SplatName, body: splat(wasm.OpcodeVecI64x2Splat),
			expected:             OperationV128Splat{Shape: ShapeI64x2},
			needDropBeforeReturn: true,
		},
		{
			name: wasm.OpcodeVecF32x4SplatName, body: splat(wasm.OpcodeVecF32x4Splat),
			expected:             OperationV128Splat{Shape: ShapeF32x4},
			needDropBeforeReturn: true,
		},
		{
			name: wasm.OpcodeVecF64x2SplatName, body: splat(wasm.OpcodeVecF64x2Splat),
			expected:             OperationV128Splat{Shape: ShapeF64x2},
			needDropBeforeReturn: true,
		},
		{
			name: wasm.OpcodeVecI8x16SwizzleName, body: vv2v(wasm.OpcodeVecI8x16Swizzle),
			expected:             OperationV128Swizzle{},
			needDropBeforeReturn: true,
		},
		{
			name: wasm.OpcodeVecV128i8x16ShuffleName, body: []byte{
				wasm.OpcodeVecPrefix,
				wasm.OpcodeVecV128Const, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
				wasm.OpcodeVecPrefix,
				wasm.OpcodeVecV128Const, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1,
				wasm.OpcodeVecPrefix,
				wasm.OpcodeVecV128i8x16Shuffle,
				1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
				wasm.OpcodeDrop,
				wasm.OpcodeEnd,
			},
			expected: OperationV128Shuffle{Lanes: [16]byte{
				1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
			}},
			needDropBeforeReturn: true,
		},
		{
			name: wasm.OpcodeVecV128NotName, body: v2v(wasm.OpcodeVecV128Not),
			needDropBeforeReturn: true,
			expected:             OperationV128Not{},
		},
		{
			name: wasm.OpcodeVecV128AndName, body: vv2v(wasm.OpcodeVecV128And),
			needDropBeforeReturn: true,
			expected:             OperationV128And{},
		},
		{
			name: wasm.OpcodeVecV128AndNotName, body: vv2v(wasm.OpcodeVecV128AndNot),
			needDropBeforeReturn: true,
			expected:             OperationV128AndNot{},
		},
		{
			name: wasm.OpcodeVecV128OrName, body: vv2v(wasm.OpcodeVecV128Or),
			needDropBeforeReturn: true,
			expected:             OperationV128Or{},
		},
		{
			name: wasm.OpcodeVecV128XorName, body: vv2v(wasm.OpcodeVecV128Xor),
			needDropBeforeReturn: true,
			expected:             OperationV128Xor{},
		},
		{
			name: wasm.OpcodeVecV128BitselectName, body: vvv2v(wasm.OpcodeVecV128Bitselect),
			needDropBeforeReturn: true,
			expected:             OperationV128Bitselect{},
		},
		{
			name: wasm.OpcodeVecI8x16ShlName, body: vi2v(wasm.OpcodeVecI8x16Shl),
			needDropBeforeReturn: true,
			expected:             OperationV128Shl{Shape: ShapeI8x16},
		},
		{
			name: wasm.OpcodeVecI8x16ShrSName, body: vi2v(wasm.OpcodeVecI8x16ShrS),
			needDropBeforeReturn: true,
			expected:             OperationV128Shr{Shape: ShapeI8x16, Signed: true},
		},
		{
			name: wasm.OpcodeVecI8x16ShrUName, body: vi2v(wasm.OpcodeVecI8x16ShrU),
			needDropBeforeReturn: true,
			expected:             OperationV128Shr{Shape: ShapeI8x16, Signed: false},
		},
		{
			name: wasm.OpcodeVecI16x8ShlName, body: vi2v(wasm.OpcodeVecI16x8Shl),
			needDropBeforeReturn: true,
			expected:             OperationV128Shl{Shape: ShapeI16x8},
		},
		{
			name: wasm.OpcodeVecI16x8ShrSName, body: vi2v(wasm.OpcodeVecI16x8ShrS),
			needDropBeforeReturn: true,
			expected:             OperationV128Shr{Shape: ShapeI16x8, Signed: true},
		},
		{
			name: wasm.OpcodeVecI16x8ShrUName, body: vi2v(wasm.OpcodeVecI16x8ShrU),
			needDropBeforeReturn: true,
			expected:             OperationV128Shr{Shape: ShapeI16x8, Signed: false},
		},
		{
			name: wasm.OpcodeVecI32x4ShlName, body: vi2v(wasm.OpcodeVecI32x4Shl),
			needDropBeforeReturn: true,
			expected:             OperationV128Shl{Shape: ShapeI32x4},
		},
		{
			name: wasm.OpcodeVecI32x4ShrSName, body: vi2v(wasm.OpcodeVecI32x4ShrS),
			needDropBeforeReturn: true,
			expected:             OperationV128Shr{Shape: ShapeI32x4, Signed: true},
		},
		{
			name: wasm.OpcodeVecI32x4ShrUName, body: vi2v(wasm.OpcodeVecI32x4ShrU),
			needDropBeforeReturn: true,
			expected:             OperationV128Shr{Shape: ShapeI32x4, Signed: false},
		},
		{
			name: wasm.OpcodeVecI64x2ShlName, body: vi2v(wasm.OpcodeVecI64x2Shl),
			needDropBeforeReturn: true,
			expected:             OperationV128Shl{Shape: ShapeI64x2},
		},
		{
			name: wasm.OpcodeVecI64x2ShrSName, body: vi2v(wasm.OpcodeVecI64x2ShrS),
			needDropBeforeReturn: true,
			expected:             OperationV128Shr{Shape: ShapeI64x2, Signed: true},
		},
		{
			name: wasm.OpcodeVecI64x2ShrUName, body: vi2v(wasm.OpcodeVecI64x2ShrU),
			needDropBeforeReturn: true,
			expected:             OperationV128Shr{Shape: ShapeI64x2, Signed: false},
		},
		{
			name: wasm.OpcodeVecI8x16EqName, body: vv2v(wasm.OpcodeVecI8x16Eq),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI8x16Eq},
		},
		{
			name: wasm.OpcodeVecI8x16NeName, body: vv2v(wasm.OpcodeVecI8x16Ne),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI8x16Ne},
		},
		{
			name: wasm.OpcodeVecI8x16LtSName, body: vv2v(wasm.OpcodeVecI8x16LtS),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI8x16LtS},
		},
		{
			name: wasm.OpcodeVecI8x16LtUName, body: vv2v(wasm.OpcodeVecI8x16LtU),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI8x16LtU},
		},
		{
			name: wasm.OpcodeVecI8x16GtSName, body: vv2v(wasm.OpcodeVecI8x16GtS),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI8x16GtS},
		},
		{
			name: wasm.OpcodeVecI8x16GtUName, body: vv2v(wasm.OpcodeVecI8x16GtU),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI8x16GtU},
		},
		{
			name: wasm.OpcodeVecI8x16LeSName, body: vv2v(wasm.OpcodeVecI8x16LeS),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI8x16LeS},
		},
		{
			name: wasm.OpcodeVecI8x16LeUName, body: vv2v(wasm.OpcodeVecI8x16LeU),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI8x16LeU},
		},
		{
			name: wasm.OpcodeVecI8x16GeSName, body: vv2v(wasm.OpcodeVecI8x16GeS),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI8x16GeS},
		},
		{
			name: wasm.OpcodeVecI8x16GeUName, body: vv2v(wasm.OpcodeVecI8x16GeU),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI8x16GeU},
		},
		{
			name: wasm.OpcodeVecI16x8EqName, body: vv2v(wasm.OpcodeVecI16x8Eq),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI16x8Eq},
		},
		{
			name: wasm.OpcodeVecI16x8NeName, body: vv2v(wasm.OpcodeVecI16x8Ne),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI16x8Ne},
		},
		{
			name: wasm.OpcodeVecI16x8LtSName, body: vv2v(wasm.OpcodeVecI16x8LtS),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI16x8LtS},
		},
		{
			name: wasm.OpcodeVecI16x8LtUName, body: vv2v(wasm.OpcodeVecI16x8LtU),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI16x8LtU},
		},
		{
			name: wasm.OpcodeVecI16x8GtSName, body: vv2v(wasm.OpcodeVecI16x8GtS),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI16x8GtS},
		},
		{
			name: wasm.OpcodeVecI16x8GtUName, body: vv2v(wasm.OpcodeVecI16x8GtU),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI16x8GtU},
		},
		{
			name: wasm.OpcodeVecI16x8LeSName, body: vv2v(wasm.OpcodeVecI16x8LeS),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI16x8LeS},
		},
		{
			name: wasm.OpcodeVecI16x8LeUName, body: vv2v(wasm.OpcodeVecI16x8LeU),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI16x8LeU},
		},
		{
			name: wasm.OpcodeVecI16x8GeSName, body: vv2v(wasm.OpcodeVecI16x8GeS),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI16x8GeS},
		},
		{
			name: wasm.OpcodeVecI16x8GeUName, body: vv2v(wasm.OpcodeVecI16x8GeU),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI16x8GeU},
		},
		{
			name: wasm.OpcodeVecI32x4EqName, body: vv2v(wasm.OpcodeVecI32x4Eq),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI32x4Eq},
		},
		{
			name: wasm.OpcodeVecI32x4NeName, body: vv2v(wasm.OpcodeVecI32x4Ne),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI32x4Ne},
		},
		{
			name: wasm.OpcodeVecI32x4LtSName, body: vv2v(wasm.OpcodeVecI32x4LtS),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI32x4LtS},
		},
		{
			name: wasm.OpcodeVecI32x4LtUName, body: vv2v(wasm.OpcodeVecI32x4LtU),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI32x4LtU},
		},
		{
			name: wasm.OpcodeVecI32x4GtSName, body: vv2v(wasm.OpcodeVecI32x4GtS),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI32x4GtS},
		},
		{
			name: wasm.OpcodeVecI32x4GtUName, body: vv2v(wasm.OpcodeVecI32x4GtU),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI32x4GtU},
		},
		{
			name: wasm.OpcodeVecI32x4LeSName, body: vv2v(wasm.OpcodeVecI32x4LeS),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI32x4LeS},
		},
		{
			name: wasm.OpcodeVecI32x4LeUName, body: vv2v(wasm.OpcodeVecI32x4LeU),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI32x4LeU},
		},
		{
			name: wasm.OpcodeVecI32x4GeSName, body: vv2v(wasm.OpcodeVecI32x4GeS),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI32x4GeS},
		},
		{
			name: wasm.OpcodeVecI32x4GeUName, body: vv2v(wasm.OpcodeVecI32x4GeU),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI32x4GeU},
		},
		{
			name: wasm.OpcodeVecI64x2EqName, body: vv2v(wasm.OpcodeVecI64x2Eq),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI64x2Eq},
		},
		{
			name: wasm.OpcodeVecI64x2NeName, body: vv2v(wasm.OpcodeVecI64x2Ne),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI64x2Ne},
		},
		{
			name: wasm.OpcodeVecI64x2LtSName, body: vv2v(wasm.OpcodeVecI64x2LtS),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI64x2LtS},
		},
		{
			name: wasm.OpcodeVecI64x2GtSName, body: vv2v(wasm.OpcodeVecI64x2GtS),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI64x2GtS},
		},
		{
			name: wasm.OpcodeVecI64x2LeSName, body: vv2v(wasm.OpcodeVecI64x2LeS),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI64x2LeS},
		},
		{
			name: wasm.OpcodeVecI64x2GeSName, body: vv2v(wasm.OpcodeVecI64x2GeS),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeI64x2GeS},
		},
		{
			name: wasm.OpcodeVecF32x4EqName, body: vv2v(wasm.OpcodeVecF32x4Eq),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeF32x4Eq},
		},
		{
			name: wasm.OpcodeVecF32x4NeName, body: vv2v(wasm.OpcodeVecF32x4Ne),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeF32x4Ne},
		},
		{
			name: wasm.OpcodeVecF32x4LtName, body: vv2v(wasm.OpcodeVecF32x4Lt),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeF32x4Lt},
		},
		{
			name: wasm.OpcodeVecF32x4GtName, body: vv2v(wasm.OpcodeVecF32x4Gt),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeF32x4Gt},
		},
		{
			name: wasm.OpcodeVecF32x4LeName, body: vv2v(wasm.OpcodeVecF32x4Le),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeF32x4Le},
		},
		{
			name: wasm.OpcodeVecF32x4GeName, body: vv2v(wasm.OpcodeVecF32x4Ge),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeF32x4Ge},
		},
		{
			name: wasm.OpcodeVecF64x2EqName, body: vv2v(wasm.OpcodeVecF64x2Eq),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeF64x2Eq},
		},
		{
			name: wasm.OpcodeVecF64x2NeName, body: vv2v(wasm.OpcodeVecF64x2Ne),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeF64x2Ne},
		},
		{
			name: wasm.OpcodeVecF64x2LtName, body: vv2v(wasm.OpcodeVecF64x2Lt),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeF64x2Lt},
		},
		{
			name: wasm.OpcodeVecF64x2GtName, body: vv2v(wasm.OpcodeVecF64x2Gt),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeF64x2Gt},
		},
		{
			name: wasm.OpcodeVecF64x2LeName, body: vv2v(wasm.OpcodeVecF64x2Le),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeF64x2Le},
		},
		{
			name: wasm.OpcodeVecF64x2GeName, body: vv2v(wasm.OpcodeVecF64x2Ge),
			needDropBeforeReturn: true,
			expected:             OperationV128Cmp{Type: V128CmpTypeF64x2Ge},
		},
		{
			name: wasm.OpcodeVecI8x16AllTrueName, body: v2v(wasm.OpcodeVecI8x16AllTrue),
			needDropBeforeReturn: true,
			expected:             OperationV128AllTrue{Shape: ShapeI8x16},
		},
		{
			name: wasm.OpcodeVecI16x8AllTrueName, body: v2v(wasm.OpcodeVecI16x8AllTrue),
			needDropBeforeReturn: true,
			expected:             OperationV128AllTrue{Shape: ShapeI16x8},
		},
		{
			name: wasm.OpcodeVecI32x4AllTrueName, body: v2v(wasm.OpcodeVecI32x4AllTrue),
			needDropBeforeReturn: true,
			expected:             OperationV128AllTrue{Shape: ShapeI32x4},
		},
		{
			name: wasm.OpcodeVecI64x2AllTrueName, body: v2v(wasm.OpcodeVecI64x2AllTrue),
			needDropBeforeReturn: true,
			expected:             OperationV128AllTrue{Shape: ShapeI64x2},
		},
		{
			name: wasm.OpcodeVecI8x16BitMaskName, body: v2v(wasm.OpcodeVecI8x16BitMask),
			needDropBeforeReturn: true, expected: OperationV128BitMask{Shape: ShapeI8x16},
		},
		{
			name: wasm.OpcodeVecI16x8BitMaskName, body: v2v(wasm.OpcodeVecI16x8BitMask),
			needDropBeforeReturn: true, expected: OperationV128BitMask{Shape: ShapeI16x8},
		},
		{
			name: wasm.OpcodeVecI32x4BitMaskName, body: v2v(wasm.OpcodeVecI32x4BitMask),
			needDropBeforeReturn: true, expected: OperationV128BitMask{Shape: ShapeI32x4},
		},
		{
			name: wasm.OpcodeVecI64x2BitMaskName, body: v2v(wasm.OpcodeVecI64x2BitMask),
			needDropBeforeReturn: true, expected: OperationV128BitMask{Shape: ShapeI64x2},
		},
		{
			name: wasm.OpcodeVecV128AnyTrueName, body: v2v(wasm.OpcodeVecV128AnyTrue),
			needDropBeforeReturn: true,
			expected:             OperationV128AnyTrue{},
		},
		{
			name: wasm.OpcodeVecI8x16AddName, body: vv2v(wasm.OpcodeVecI8x16Add),
			needDropBeforeReturn: true,
			expected:             OperationV128Add{Shape: ShapeI8x16},
		},
		{
			name: wasm.OpcodeVecI8x16AddSatSName, body: vv2v(wasm.OpcodeVecI8x16AddSatS),
			needDropBeforeReturn: true,
			expected:             OperationV128AddSat{Shape: ShapeI8x16, Signed: true},
		},
		{
			name: wasm.OpcodeVecI8x16AddSatUName, body: vv2v(wasm.OpcodeVecI8x16AddSatU),
			needDropBeforeReturn: true,
			expected:             OperationV128AddSat{Shape: ShapeI8x16, Signed: false},
		},
		{
			name: wasm.OpcodeVecI8x16SubName, body: vv2v(wasm.OpcodeVecI8x16Sub),
			needDropBeforeReturn: true,
			expected:             OperationV128Sub{Shape: ShapeI8x16},
		},
		{
			name: wasm.OpcodeVecI8x16SubSatSName, body: vv2v(wasm.OpcodeVecI8x16SubSatS),
			needDropBeforeReturn: true,
			expected:             OperationV128SubSat{Shape: ShapeI8x16, Signed: true},
		},
		{
			name: wasm.OpcodeVecI8x16SubSatUName, body: vv2v(wasm.OpcodeVecI8x16SubSatU),
			needDropBeforeReturn: true,
			expected:             OperationV128SubSat{Shape: ShapeI8x16, Signed: false},
		},
		{
			name: wasm.OpcodeVecI16x8AddName, body: vv2v(wasm.OpcodeVecI16x8Add),
			needDropBeforeReturn: true,
			expected:             OperationV128Add{Shape: ShapeI16x8},
		},
		{
			name: wasm.OpcodeVecI16x8AddSatSName, body: vv2v(wasm.OpcodeVecI16x8AddSatS),
			needDropBeforeReturn: true,
			expected:             OperationV128AddSat{Shape: ShapeI16x8, Signed: true},
		},
		{
			name: wasm.OpcodeVecI16x8AddSatUName, body: vv2v(wasm.OpcodeVecI16x8AddSatU),
			needDropBeforeReturn: true,
			expected:             OperationV128AddSat{Shape: ShapeI16x8, Signed: false},
		},
		{
			name: wasm.OpcodeVecI16x8SubName, body: vv2v(wasm.OpcodeVecI16x8Sub),
			needDropBeforeReturn: true,
			expected:             OperationV128Sub{Shape: ShapeI16x8},
		},
		{
			name: wasm.OpcodeVecI16x8SubSatSName, body: vv2v(wasm.OpcodeVecI16x8SubSatS),
			needDropBeforeReturn: true,
			expected:             OperationV128SubSat{Shape: ShapeI16x8, Signed: true},
		},
		{
			name: wasm.OpcodeVecI16x8SubSatUName, body: vv2v(wasm.OpcodeVecI16x8SubSatU),
			needDropBeforeReturn: true,
			expected:             OperationV128SubSat{Shape: ShapeI16x8, Signed: false},
		},
		{
			name: wasm.OpcodeVecI16x8MulName, body: vv2v(wasm.OpcodeVecI16x8Mul),
			needDropBeforeReturn: true,
			expected:             OperationV128Mul{Shape: ShapeI16x8},
		},
		{
			name: wasm.OpcodeVecI32x4AddName, body: vv2v(wasm.OpcodeVecI32x4Add),
			needDropBeforeReturn: true,
			expected:             OperationV128Add{Shape: ShapeI32x4},
		},
		{
			name: wasm.OpcodeVecI32x4SubName, body: vv2v(wasm.OpcodeVecI32x4Sub),
			needDropBeforeReturn: true,
			expected:             OperationV128Sub{Shape: ShapeI32x4},
		},
		{
			name: wasm.OpcodeVecI32x4MulName, body: vv2v(wasm.OpcodeVecI32x4Mul),
			needDropBeforeReturn: true,
			expected:             OperationV128Mul{Shape: ShapeI32x4},
		},
		{
			name: wasm.OpcodeVecI64x2AddName, body: vv2v(wasm.OpcodeVecI64x2Add),
			needDropBeforeReturn: true,
			expected:             OperationV128Add{Shape: ShapeI64x2},
		},
		{
			name: wasm.OpcodeVecI64x2SubName, body: vv2v(wasm.OpcodeVecI64x2Sub),
			needDropBeforeReturn: true,
			expected:             OperationV128Sub{Shape: ShapeI64x2},
		},
		{
			name: wasm.OpcodeVecI64x2MulName, body: vv2v(wasm.OpcodeVecI64x2Mul),
			needDropBeforeReturn: true,
			expected:             OperationV128Mul{Shape: ShapeI64x2},
		},
		{
			name: wasm.OpcodeVecF32x4AddName, body: vv2v(wasm.OpcodeVecF32x4Add),
			needDropBeforeReturn: true,
			expected:             OperationV128Add{Shape: ShapeF32x4},
		},
		{
			name: wasm.OpcodeVecF32x4SubName, body: vv2v(wasm.OpcodeVecF32x4Sub),
			needDropBeforeReturn: true,
			expected:             OperationV128Sub{Shape: ShapeF32x4},
		},
		{
			name: wasm.OpcodeVecF32x4MulName, body: vv2v(wasm.OpcodeVecF32x4Mul),
			needDropBeforeReturn: true,
			expected:             OperationV128Mul{Shape: ShapeF32x4},
		},
		{
			name: wasm.OpcodeVecF32x4DivName, body: vv2v(wasm.OpcodeVecF32x4Div),
			needDropBeforeReturn: true,
			expected:             OperationV128Div{Shape: ShapeF32x4},
		},
		{
			name: wasm.OpcodeVecF64x2AddName, body: vv2v(wasm.OpcodeVecF64x2Add),
			needDropBeforeReturn: true,
			expected:             OperationV128Add{Shape: ShapeF64x2},
		},
		{
			name: wasm.OpcodeVecF64x2SubName, body: vv2v(wasm.OpcodeVecF64x2Sub),
			needDropBeforeReturn: true,
			expected:             OperationV128Sub{Shape: ShapeF64x2},
		},
		{
			name: wasm.OpcodeVecF64x2MulName, body: vv2v(wasm.OpcodeVecF64x2Mul),
			needDropBeforeReturn: true,
			expected:             OperationV128Mul{Shape: ShapeF64x2},
		},
		{
			name: wasm.OpcodeVecF64x2DivName, body: vv2v(wasm.OpcodeVecF64x2Div),
			needDropBeforeReturn: true,
			expected:             OperationV128Div{Shape: ShapeF64x2},
		},
		{
			name: wasm.OpcodeVecI8x16MinSName, body: vv2v(wasm.OpcodeVecI8x16MinS),
			needDropBeforeReturn: true,
			expected:             OperationV128Min{Shape: ShapeI8x16, Signed: true},
		},
		{
			name: wasm.OpcodeVecI8x16MinUName, body: vv2v(wasm.OpcodeVecI8x16MinU),
			needDropBeforeReturn: true,
			expected:             OperationV128Min{Shape: ShapeI8x16},
		},
		{
			name: wasm.OpcodeVecI8x16MaxSName, body: vv2v(wasm.OpcodeVecI8x16MaxS),
			needDropBeforeReturn: true,
			expected:             OperationV128Max{Shape: ShapeI8x16, Signed: true},
		},
		{
			name: wasm.OpcodeVecI8x16MaxUName, body: vv2v(wasm.OpcodeVecI8x16MaxU),
			needDropBeforeReturn: true,
			expected:             OperationV128Max{Shape: ShapeI8x16},
		},
		{
			name: wasm.OpcodeVecI8x16AvgrUName, body: vv2v(wasm.OpcodeVecI8x16AvgrU),
			needDropBeforeReturn: true,
			expected:             OperationV128AvgrU{Shape: ShapeI8x16},
		},
		{
			name: wasm.OpcodeVecI16x8MinSName, body: vv2v(wasm.OpcodeVecI16x8MinS),
			needDropBeforeReturn: true,
			expected:             OperationV128Min{Shape: ShapeI16x8, Signed: true},
		},
		{
			name: wasm.OpcodeVecI16x8MinUName, body: vv2v(wasm.OpcodeVecI16x8MinU),
			needDropBeforeReturn: true,
			expected:             OperationV128Min{Shape: ShapeI16x8},
		},
		{
			name: wasm.OpcodeVecI16x8MaxSName, body: vv2v(wasm.OpcodeVecI16x8MaxS),
			needDropBeforeReturn: true,
			expected:             OperationV128Max{Shape: ShapeI16x8, Signed: true},
		},
		{
			name: wasm.OpcodeVecI16x8MaxUName, body: vv2v(wasm.OpcodeVecI16x8MaxU),
			needDropBeforeReturn: true,
			expected:             OperationV128Max{Shape: ShapeI16x8},
		},
		{
			name: wasm.OpcodeVecI16x8AvgrUName, body: vv2v(wasm.OpcodeVecI16x8AvgrU),
			needDropBeforeReturn: true,
			expected:             OperationV128AvgrU{Shape: ShapeI16x8},
		},
		{
			name: wasm.OpcodeVecI32x4MinSName, body: vv2v(wasm.OpcodeVecI32x4MinS),
			needDropBeforeReturn: true,
			expected:             OperationV128Min{Shape: ShapeI32x4, Signed: true},
		},
		{
			name: wasm.OpcodeVecI32x4MinUName, body: vv2v(wasm.OpcodeVecI32x4MinU),
			needDropBeforeReturn: true,
			expected:             OperationV128Min{Shape: ShapeI32x4},
		},
		{
			name: wasm.OpcodeVecI32x4MaxSName, body: vv2v(wasm.OpcodeVecI32x4MaxS),
			needDropBeforeReturn: true,
			expected:             OperationV128Max{Shape: ShapeI32x4, Signed: true},
		},
		{
			name: wasm.OpcodeVecI32x4MaxUName, body: vv2v(wasm.OpcodeVecI32x4MaxU),
			needDropBeforeReturn: true,
			expected:             OperationV128Max{Shape: ShapeI32x4},
		},
		{
			name: wasm.OpcodeVecF32x4MinName, body: vv2v(wasm.OpcodeVecF32x4Min),
			needDropBeforeReturn: true,
			expected:             OperationV128Min{Shape: ShapeF32x4},
		},
		{
			name: wasm.OpcodeVecF32x4MaxName, body: vv2v(wasm.OpcodeVecF32x4Max),
			needDropBeforeReturn: true,
			expected:             OperationV128Max{Shape: ShapeF32x4},
		},
		{
			name: wasm.OpcodeVecF64x2MinName, body: vv2v(wasm.OpcodeVecF64x2Min),
			needDropBeforeReturn: true,
			expected:             OperationV128Min{Shape: ShapeF64x2},
		},
		{
			name: wasm.OpcodeVecF64x2MaxName, body: vv2v(wasm.OpcodeVecF64x2Max),
			needDropBeforeReturn: true,
			expected:             OperationV128Max{Shape: ShapeF64x2},
		},
		{
			name: wasm.OpcodeVecI8x16AbsName, body: v2v(wasm.OpcodeVecI8x16Abs),
			needDropBeforeReturn: true,
			expected:             OperationV128Abs{Shape: ShapeI8x16},
		},
		{
			name: wasm.OpcodeVecI8x16PopcntName, body: v2v(wasm.OpcodeVecI8x16Popcnt),
			needDropBeforeReturn: true,
			expected:             OperationV128Popcnt{},
		},
		{
			name: wasm.OpcodeVecI16x8AbsName, body: v2v(wasm.OpcodeVecI16x8Abs),
			needDropBeforeReturn: true,
			expected:             OperationV128Abs{Shape: ShapeI16x8},
		},
		{
			name: wasm.OpcodeVecI32x4AbsName, body: v2v(wasm.OpcodeVecI32x4Abs),
			needDropBeforeReturn: true,
			expected:             OperationV128Abs{Shape: ShapeI32x4},
		},
		{
			name: wasm.OpcodeVecI64x2AbsName, body: v2v(wasm.OpcodeVecI64x2Abs),
			needDropBeforeReturn: true,
			expected:             OperationV128Abs{Shape: ShapeI64x2},
		},
		{
			name: wasm.OpcodeVecF32x4AbsName, body: v2v(wasm.OpcodeVecF32x4Abs),
			needDropBeforeReturn: true,
			expected:             OperationV128Abs{Shape: ShapeF32x4},
		},
		{
			name: wasm.OpcodeVecF64x2AbsName, body: v2v(wasm.OpcodeVecF64x2Abs),
			needDropBeforeReturn: true,
			expected:             OperationV128Abs{Shape: ShapeF64x2},
		},
		{
			name: wasm.OpcodeVecF32x4CeilName, body: v2v(wasm.OpcodeVecF32x4Ceil),
			needDropBeforeReturn: true,
			expected:             OperationV128Ceil{Shape: ShapeF32x4},
		},
		{
			name: wasm.OpcodeVecF32x4FloorName, body: v2v(wasm.OpcodeVecF32x4Floor),
			needDropBeforeReturn: true,
			expected:             OperationV128Floor{Shape: ShapeF32x4},
		},
		{
			name: wasm.OpcodeVecF32x4TruncName, body: v2v(wasm.OpcodeVecF32x4Trunc),
			needDropBeforeReturn: true,
			expected:             OperationV128Trunc{Shape: ShapeF32x4},
		},
		{
			name: wasm.OpcodeVecF32x4NearestName, body: v2v(wasm.OpcodeVecF32x4Nearest),
			needDropBeforeReturn: true,
			expected:             OperationV128Nearest{Shape: ShapeF32x4},
		},
		{
			name: wasm.OpcodeVecF64x2CeilName, body: v2v(wasm.OpcodeVecF64x2Ceil),
			needDropBeforeReturn: true,
			expected:             OperationV128Ceil{Shape: ShapeF64x2},
		},
		{
			name: wasm.OpcodeVecF64x2FloorName, body: v2v(wasm.OpcodeVecF64x2Floor),
			needDropBeforeReturn: true,
			expected:             OperationV128Floor{Shape: ShapeF64x2},
		},
		{
			name: wasm.OpcodeVecF64x2TruncName, body: v2v(wasm.OpcodeVecF64x2Trunc),
			needDropBeforeReturn: true,
			expected:             OperationV128Trunc{Shape: ShapeF64x2},
		},
		{
			name: wasm.OpcodeVecF64x2NearestName, body: v2v(wasm.OpcodeVecF64x2Nearest),
			needDropBeforeReturn: true,
			expected:             OperationV128Nearest{Shape: ShapeF64x2},
		},
		{
			name: wasm.OpcodeVecF32x4PminName, body: vv2v(wasm.OpcodeVecF32x4Pmin),
			needDropBeforeReturn: true,
			expected:             OperationV128Pmin{Shape: ShapeF32x4},
		},
		{
			name: wasm.OpcodeVecF32x4PmaxName, body: vv2v(wasm.OpcodeVecF32x4Pmax),
			needDropBeforeReturn: true,
			expected:             OperationV128Pmax{Shape: ShapeF32x4},
		},
		{
			name: wasm.OpcodeVecF64x2PminName, body: vv2v(wasm.OpcodeVecF64x2Pmin),
			needDropBeforeReturn: true,
			expected:             OperationV128Pmin{Shape: ShapeF64x2},
		},
		{
			name: wasm.OpcodeVecF64x2PmaxName, body: vv2v(wasm.OpcodeVecF64x2Pmax),
			needDropBeforeReturn: true,
			expected:             OperationV128Pmax{Shape: ShapeF64x2},
		},
		{
			name: wasm.OpcodeVecI16x8Q15mulrSatSName, body: vv2v(wasm.OpcodeVecI16x8Q15mulrSatS),
			needDropBeforeReturn: true,
			expected:             OperationV128Q15mulrSatS{},
		},
		{
			name: wasm.OpcodeVecI16x8ExtMulLowI8x16SName, body: vv2v(wasm.OpcodeVecI16x8ExtMulLowI8x16S),
			needDropBeforeReturn: true,
			expected: OperationV128ExtMul{
				OriginShape: ShapeI8x16,
				Signed:      true,
				UseLow:      true,
			},
		},
		{
			name: wasm.OpcodeVecI16x8ExtMulHighI8x16SName, body: vv2v(wasm.OpcodeVecI16x8ExtMulHighI8x16S),
			needDropBeforeReturn: true,
			expected: OperationV128ExtMul{
				OriginShape: ShapeI8x16,
				Signed:      true,
				UseLow:      false,
			},
		},
		{
			name: wasm.OpcodeVecI16x8ExtMulLowI8x16UName, body: vv2v(wasm.OpcodeVecI16x8ExtMulLowI8x16U),
			needDropBeforeReturn: true,
			expected: OperationV128ExtMul{
				OriginShape: ShapeI8x16,
				Signed:      false,
				UseLow:      true,
			},
		},
		{
			name: wasm.OpcodeVecI16x8ExtMulHighI8x16UName, body: vv2v(wasm.OpcodeVecI16x8ExtMulHighI8x16U),
			needDropBeforeReturn: true,
			expected: OperationV128ExtMul{
				OriginShape: ShapeI8x16,
				Signed:      false,
				UseLow:      false,
			},
		},
		{
			name: wasm.OpcodeVecI32x4ExtMulLowI16x8SName, body: vv2v(wasm.OpcodeVecI32x4ExtMulLowI16x8S),
			needDropBeforeReturn: true,
			expected: OperationV128ExtMul{
				OriginShape: ShapeI16x8,
				Signed:      true,
				UseLow:      true,
			},
		},
		{
			name: wasm.OpcodeVecI32x4ExtMulHighI16x8SName, body: vv2v(wasm.OpcodeVecI32x4ExtMulHighI16x8S),
			needDropBeforeReturn: true,
			expected: OperationV128ExtMul{
				OriginShape: ShapeI16x8,
				Signed:      true,
				UseLow:      false,
			},
		},
		{
			name: wasm.OpcodeVecI32x4ExtMulLowI16x8UName, body: vv2v(wasm.OpcodeVecI32x4ExtMulLowI16x8U),
			needDropBeforeReturn: true,
			expected: OperationV128ExtMul{
				OriginShape: ShapeI16x8,
				Signed:      false,
				UseLow:      true,
			},
		},
		{
			name: wasm.OpcodeVecI32x4ExtMulHighI16x8UName, body: vv2v(wasm.OpcodeVecI32x4ExtMulHighI16x8U),
			needDropBeforeReturn: true,
			expected: OperationV128ExtMul{
				OriginShape: ShapeI16x8,
				Signed:      false,
				UseLow:      false,
			},
		},
		{
			name: wasm.OpcodeVecI64x2ExtMulLowI32x4SName, body: vv2v(wasm.OpcodeVecI64x2ExtMulLowI32x4S),
			needDropBeforeReturn: true,
			expected: OperationV128ExtMul{
				OriginShape: ShapeI32x4,
				Signed:      true,
				UseLow:      true,
			},
		},
		{
			name: wasm.OpcodeVecI64x2ExtMulHighI32x4SName, body: vv2v(wasm.OpcodeVecI64x2ExtMulHighI32x4S),
			needDropBeforeReturn: true,
			expected: OperationV128ExtMul{
				OriginShape: ShapeI32x4,
				Signed:      true,
				UseLow:      false,
			},
		},
		{
			name: wasm.OpcodeVecI64x2ExtMulLowI32x4UName, body: vv2v(wasm.OpcodeVecI64x2ExtMulLowI32x4U),
			needDropBeforeReturn: true,
			expected: OperationV128ExtMul{
				OriginShape: ShapeI32x4,
				Signed:      false,
				UseLow:      true,
			},
		},
		{
			name: wasm.OpcodeVecI64x2ExtMulHighI32x4UName, body: vv2v(wasm.OpcodeVecI64x2ExtMulHighI32x4U),
			needDropBeforeReturn: true,
			expected: OperationV128ExtMul{
				OriginShape: ShapeI32x4,
				Signed:      false,
				UseLow:      false,
			},
		},
		{
			name: wasm.OpcodeVecI16x8ExtendLowI8x16SName, body: v2v(wasm.OpcodeVecI16x8ExtendLowI8x16S),
			needDropBeforeReturn: true,
			expected: OperationV128Extend{
				OriginShape: ShapeI8x16,
				Signed:      true,
				UseLow:      true,
			},
		},
		{
			name: wasm.OpcodeVecI16x8ExtendHighI8x16SName, body: v2v(wasm.OpcodeVecI16x8ExtendHighI8x16S),
			needDropBeforeReturn: true,
			expected: OperationV128Extend{
				OriginShape: ShapeI8x16,
				Signed:      true,
				UseLow:      false,
			},
		},
		{
			name: wasm.OpcodeVecI16x8ExtendLowI8x16UName, body: v2v(wasm.OpcodeVecI16x8ExtendLowI8x16U),
			needDropBeforeReturn: true,
			expected: OperationV128Extend{
				OriginShape: ShapeI8x16,
				Signed:      false,
				UseLow:      true,
			},
		},
		{
			name: wasm.OpcodeVecI16x8ExtendHighI8x16UName, body: v2v(wasm.OpcodeVecI16x8ExtendHighI8x16U),
			needDropBeforeReturn: true,
			expected: OperationV128Extend{
				OriginShape: ShapeI8x16,
				Signed:      false,
				UseLow:      false,
			},
		},
		{
			name: wasm.OpcodeVecI32x4ExtendLowI16x8SName, body: v2v(wasm.OpcodeVecI32x4ExtendLowI16x8S),
			needDropBeforeReturn: true,
			expected: OperationV128Extend{
				OriginShape: ShapeI16x8,
				Signed:      true,
				UseLow:      true,
			},
		},
		{
			name: wasm.OpcodeVecI32x4ExtendHighI16x8SName, body: v2v(wasm.OpcodeVecI32x4ExtendHighI16x8S),
			needDropBeforeReturn: true,
			expected: OperationV128Extend{
				OriginShape: ShapeI16x8,
				Signed:      true,
				UseLow:      false,
			},
		},
		{
			name: wasm.OpcodeVecI32x4ExtendLowI16x8UName, body: v2v(wasm.OpcodeVecI32x4ExtendLowI16x8U),
			needDropBeforeReturn: true,
			expected: OperationV128Extend{
				OriginShape: ShapeI16x8,
				Signed:      false,
				UseLow:      true,
			},
		},
		{
			name: wasm.OpcodeVecI32x4ExtendHighI16x8UName, body: v2v(wasm.OpcodeVecI32x4ExtendHighI16x8U),
			needDropBeforeReturn: true,
			expected: OperationV128Extend{
				OriginShape: ShapeI16x8,
				Signed:      false,
				UseLow:      false,
			},
		},
		{
			name: wasm.OpcodeVecI64x2ExtendLowI32x4SName, body: v2v(wasm.OpcodeVecI64x2ExtendLowI32x4S),
			needDropBeforeReturn: true,
			expected: OperationV128Extend{
				OriginShape: ShapeI32x4,
				Signed:      true,
				UseLow:      true,
			},
		},
		{
			name: wasm.OpcodeVecI64x2ExtendHighI32x4SName, body: v2v(wasm.OpcodeVecI64x2ExtendHighI32x4S),
			needDropBeforeReturn: true,
			expected: OperationV128Extend{
				OriginShape: ShapeI32x4,
				Signed:      true,
				UseLow:      false,
			},
		},
		{
			name: wasm.OpcodeVecI64x2ExtendLowI32x4UName, body: v2v(wasm.OpcodeVecI64x2ExtendLowI32x4U),
			needDropBeforeReturn: true,
			expected: OperationV128Extend{
				OriginShape: ShapeI32x4,
				Signed:      false,
				UseLow:      true,
			},
		},
		{
			name: wasm.OpcodeVecI64x2ExtendHighI32x4UName, body: v2v(wasm.OpcodeVecI64x2ExtendHighI32x4U),
			needDropBeforeReturn: true,
			expected: OperationV128Extend{
				OriginShape: ShapeI32x4,
				Signed:      false,
				UseLow:      false,
			},
		},

		{
			name: wasm.OpcodeVecI16x8ExtaddPairwiseI8x16SName, body: v2v(wasm.OpcodeVecI16x8ExtaddPairwiseI8x16S),
			needDropBeforeReturn: true,
			expected:             OperationV128ExtAddPairwise{OriginShape: ShapeI8x16, Signed: true},
		},
		{
			name: wasm.OpcodeVecI16x8ExtaddPairwiseI8x16UName, body: v2v(wasm.OpcodeVecI16x8ExtaddPairwiseI8x16U),
			needDropBeforeReturn: true,
			expected:             OperationV128ExtAddPairwise{OriginShape: ShapeI8x16, Signed: false},
		},
		{
			name: wasm.OpcodeVecI32x4ExtaddPairwiseI16x8SName, body: v2v(wasm.OpcodeVecI32x4ExtaddPairwiseI16x8S),
			needDropBeforeReturn: true,
			expected:             OperationV128ExtAddPairwise{OriginShape: ShapeI16x8, Signed: true},
		},
		{
			name: wasm.OpcodeVecI32x4ExtaddPairwiseI16x8UName, body: v2v(wasm.OpcodeVecI32x4ExtaddPairwiseI16x8U),
			needDropBeforeReturn: true,
			expected:             OperationV128ExtAddPairwise{OriginShape: ShapeI16x8, Signed: false},
		},
		{
			name: wasm.OpcodeVecF64x2PromoteLowF32x4ZeroName, body: v2v(wasm.OpcodeVecF64x2PromoteLowF32x4Zero),
			needDropBeforeReturn: true,
			expected:             OperationV128FloatPromote{},
		},
		{
			name: wasm.OpcodeVecF32x4DemoteF64x2ZeroName, body: v2v(wasm.OpcodeVecF32x4DemoteF64x2Zero),
			needDropBeforeReturn: true,
			expected:             OperationV128FloatDemote{},
		},
		{
			name: wasm.OpcodeVecF32x4ConvertI32x4SName, body: v2v(wasm.OpcodeVecF32x4ConvertI32x4S),
			needDropBeforeReturn: true,
			expected:             OperationV128FConvertFromI{DestinationShape: ShapeF32x4, Signed: true},
		},
		{
			name: wasm.OpcodeVecF32x4ConvertI32x4UName, body: v2v(wasm.OpcodeVecF32x4ConvertI32x4U),
			needDropBeforeReturn: true,
			expected:             OperationV128FConvertFromI{DestinationShape: ShapeF32x4, Signed: false},
		},
		{
			name: wasm.OpcodeVecF64x2ConvertLowI32x4SName, body: v2v(wasm.OpcodeVecF64x2ConvertLowI32x4S),
			needDropBeforeReturn: true,
			expected:             OperationV128FConvertFromI{DestinationShape: ShapeF64x2, Signed: true},
		},
		{
			name: wasm.OpcodeVecF64x2ConvertLowI32x4UName, body: v2v(wasm.OpcodeVecF64x2ConvertLowI32x4U),
			needDropBeforeReturn: true,
			expected:             OperationV128FConvertFromI{DestinationShape: ShapeF64x2, Signed: false},
		},
		{
			name: wasm.OpcodeVecI32x4DotI16x8SName, body: vv2v(wasm.OpcodeVecI32x4DotI16x8S),
			needDropBeforeReturn: true,
			expected:             OperationV128Dot{},
		},
		{
			name: wasm.OpcodeVecI8x16NarrowI16x8SName, body: vv2v(wasm.OpcodeVecI8x16NarrowI16x8S),
			needDropBeforeReturn: true,
			expected:             OperationV128Narrow{OriginShape: ShapeI16x8, Signed: true},
		},
		{
			name: wasm.OpcodeVecI8x16NarrowI16x8UName, body: vv2v(wasm.OpcodeVecI8x16NarrowI16x8U),
			needDropBeforeReturn: true,
			expected:             OperationV128Narrow{OriginShape: ShapeI16x8, Signed: false},
		},
		{
			name: wasm.OpcodeVecI16x8NarrowI32x4SName, body: vv2v(wasm.OpcodeVecI16x8NarrowI32x4S),
			needDropBeforeReturn: true,
			expected:             OperationV128Narrow{OriginShape: ShapeI32x4, Signed: true},
		},
		{
			name: wasm.OpcodeVecI16x8NarrowI32x4UName, body: vv2v(wasm.OpcodeVecI16x8NarrowI32x4U),
			needDropBeforeReturn: true,
			expected:             OperationV128Narrow{OriginShape: ShapeI32x4, Signed: false},
		},
		{
			name: wasm.OpcodeVecI32x4TruncSatF32x4SName, body: v2v(wasm.OpcodeVecI32x4TruncSatF32x4S),
			needDropBeforeReturn: true,
			expected:             OperationV128ITruncSatFromF{OriginShape: ShapeF32x4, Signed: true},
		},
		{
			name: wasm.OpcodeVecI32x4TruncSatF32x4UName, body: v2v(wasm.OpcodeVecI32x4TruncSatF32x4U),
			needDropBeforeReturn: true,
			expected:             OperationV128ITruncSatFromF{OriginShape: ShapeF32x4, Signed: false},
		},
		{
			name: wasm.OpcodeVecI32x4TruncSatF64x2SZeroName, body: v2v(wasm.OpcodeVecI32x4TruncSatF64x2SZero),
			needDropBeforeReturn: true,
			expected:             OperationV128ITruncSatFromF{OriginShape: ShapeF64x2, Signed: true},
		},
		{
			name: wasm.OpcodeVecI32x4TruncSatF64x2UZeroName, body: v2v(wasm.OpcodeVecI32x4TruncSatF64x2UZero),
			needDropBeforeReturn: true,
			expected:             OperationV128ITruncSatFromF{OriginShape: ShapeF64x2, Signed: false},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			module := &wasm.Module{
				TypeSection:     []wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0},
				MemorySection:   &wasm.Memory{},
				CodeSection:     []wasm.Code{{Body: tc.body}},
			}
			res, err := CompileFunctions(api.CoreFeaturesV2, 0, module, false)
			require.NoError(t, err)

			var actual Operation
			if tc.needDropBeforeReturn {
				// If the drop operation is inserted, the target op exits at -3
				// as the operations looks like: [... target, drop, br(to return)].
				actual = res[0].Operations[len(res[0].Operations)-3]
			} else {
				// If the drop operation is not inserted, the target op exits at -2
				// as the operations looks like: [... target, br(to return)].
				actual = res[0].Operations[len(res[0].Operations)-2]
			}

			require.Equal(t, tc.expected, actual)
		})
	}
}

// TestCompile_unreachable_Br_BrIf_BrTable ensures that unreachable br/br_if/br_table instructions are correctly ignored.
func TestCompile_unreachable_Br_BrIf_BrTable(t *testing.T) {
	tests := []struct {
		name     string
		mod      *wasm.Module
		expected []Operation
	}{
		{
			name: "br",
			mod: &wasm.Module{
				TypeSection:     []wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0},
				CodeSection: []wasm.Code{{Body: []byte{
					wasm.OpcodeBr, 0, // Return the function -> the followings are unreachable.
					wasm.OpcodeBlock, 0,
					wasm.OpcodeBr, 1,
					wasm.OpcodeEnd, // End the block.
					wasm.OpcodeEnd, // End the function.
				}}},
			},
			expected: []Operation{OperationBr{Target: Label{Kind: LabelKindReturn}}},
		},
		{
			name: "br_if",
			mod: &wasm.Module{
				TypeSection:     []wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0},
				CodeSection: []wasm.Code{{Body: []byte{
					wasm.OpcodeBr, 0, // Return the function -> the followings are unreachable.
					wasm.OpcodeBlock, 0,
					wasm.OpcodeI32Const, 1,
					wasm.OpcodeBrIf, 1,
					wasm.OpcodeEnd, // End the block.
					wasm.OpcodeEnd, // End the function.
				}}},
			},
			expected: []Operation{OperationBr{Target: Label{Kind: LabelKindReturn}}},
		},
		{
			name: "br_table",
			mod: &wasm.Module{
				TypeSection:     []wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0},
				CodeSection: []wasm.Code{{Body: []byte{
					wasm.OpcodeBr, 0, // Return the function -> the followings are unreachable.
					wasm.OpcodeBlock, 0,
					wasm.OpcodeBrTable, 2, 2, 3,
					wasm.OpcodeEnd, // End the block.
					wasm.OpcodeEnd, // End the function.
				}}},
			},
			expected: []Operation{OperationBr{Target: Label{Kind: LabelKindReturn}}},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			res, err := CompileFunctions(api.CoreFeaturesV2, 0, tc.mod, false)
			require.NoError(t, err)
			require.Equal(t, tc.expected, res[0].Operations)
		})
	}
}

// TestCompile_drop_vectors ensures that wasm.OpcodeDrop on vector values is correctly compiled,
// which is not covered by spectest.
func TestCompile_drop_vectors(t *testing.T) {
	tests := []struct {
		name     string
		mod      *wasm.Module
		expected []Operation
	}{
		{
			name: "basic",
			mod: &wasm.Module{
				TypeSection:     []wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0},
				CodeSection: []wasm.Code{{Body: []byte{
					wasm.OpcodeVecPrefix,
					wasm.OpcodeVecV128Const, 1, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0,
					wasm.OpcodeDrop,
					wasm.OpcodeEnd,
				}}},
			},
			expected: []Operation{
				OperationV128Const{Lo: 0x1, Hi: 0x2},
				// InclusiveRange is the range in uint64 representation, so dropping a vector value on top
				// should be translated as drop [0..1] inclusively.
				OperationDrop{Depth: &InclusiveRange{Start: 0, End: 1}},
				OperationBr{Target: Label{Kind: LabelKindReturn}},
			},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			res, err := CompileFunctions(api.CoreFeaturesV2, 0, tc.mod, false)
			require.NoError(t, err)
			require.Equal(t, tc.expected, res[0].Operations)
		})
	}
}

func TestCompile_select_vectors(t *testing.T) {
	tests := []struct {
		name     string
		mod      *wasm.Module
		expected []Operation
	}{
		{
			name: "non typed",
			mod: &wasm.Module{
				TypeSection:     []wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0},
				CodeSection: []wasm.Code{{Body: []byte{
					wasm.OpcodeVecPrefix,
					wasm.OpcodeVecV128Const, 1, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0,
					wasm.OpcodeVecPrefix,
					wasm.OpcodeVecV128Const, 3, 0, 0, 0, 0, 0, 0, 0, 4, 0, 0, 0, 0, 0, 0, 0,
					wasm.OpcodeI32Const, 0,
					wasm.OpcodeSelect,
					wasm.OpcodeDrop,
					wasm.OpcodeEnd,
				}}},
				FunctionDefinitionSection: []wasm.FunctionDefinition{{}},
			},
			expected: []Operation{
				OperationV128Const{Lo: 0x1, Hi: 0x2},
				OperationV128Const{Lo: 0x3, Hi: 0x4},
				OperationConstI32{Value: 0},
				OperationSelect{IsTargetVector: true},
				OperationDrop{Depth: &InclusiveRange{Start: 0, End: 1}},
				OperationBr{Target: Label{Kind: LabelKindReturn}},
			},
		},
		{
			name: "typed",
			mod: &wasm.Module{
				TypeSection:     []wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0},
				CodeSection: []wasm.Code{{Body: []byte{
					wasm.OpcodeVecPrefix,
					wasm.OpcodeVecV128Const, 1, 0, 0, 0, 0, 0, 0, 0, 2, 0, 0, 0, 0, 0, 0, 0,
					wasm.OpcodeVecPrefix,
					wasm.OpcodeVecV128Const, 3, 0, 0, 0, 0, 0, 0, 0, 4, 0, 0, 0, 0, 0, 0, 0,
					wasm.OpcodeI32Const, 0,
					wasm.OpcodeTypedSelect, 0x1, wasm.ValueTypeV128,
					wasm.OpcodeDrop,
					wasm.OpcodeEnd,
				}}},
				FunctionDefinitionSection: []wasm.FunctionDefinition{{}},
			},
			expected: []Operation{
				OperationV128Const{Lo: 0x1, Hi: 0x2},
				OperationV128Const{Lo: 0x3, Hi: 0x4},
				OperationConstI32{Value: 0},
				OperationSelect{IsTargetVector: true},
				OperationDrop{Depth: &InclusiveRange{Start: 0, End: 1}},
				OperationBr{Target: Label{Kind: LabelKindReturn}},
			},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			res, err := CompileFunctions(api.CoreFeaturesV2, 0, tc.mod, false)
			require.NoError(t, err)
			require.Equal(t, tc.expected, res[0].Operations)
		})
	}
}

func TestCompiler_initializeStack(t *testing.T) {
	const v128 = wasm.ValueTypeV128
	tests := []struct {
		name                               string
		sig                                *wasm.FunctionType
		functionLocalTypes                 []wasm.ValueType
		callFrameStackSizeInUint64         int
		expLocalIndexToStackHeightInUint64 map[uint32]int
	}{
		{
			name: "no function local, args>results",
			sig: &wasm.FunctionType{
				Params:            []wasm.ValueType{i32, f32},
				Results:           []wasm.ValueType{i32},
				ParamNumInUint64:  2,
				ResultNumInUint64: 1,
			},
			expLocalIndexToStackHeightInUint64: map[uint32]int{
				0: 0,
				1: 1,
			},
		},
		{
			name: "no function local, args=results",
			sig: &wasm.FunctionType{
				Params:            []wasm.ValueType{i32},
				Results:           []wasm.ValueType{i32},
				ParamNumInUint64:  1,
				ResultNumInUint64: 1,
			},
			expLocalIndexToStackHeightInUint64: map[uint32]int{
				0: 0,
			},
		},
		{
			name: "no function local, args>results, with vector",
			sig: &wasm.FunctionType{
				Params:            []wasm.ValueType{i32, v128, f32},
				Results:           []wasm.ValueType{i32},
				ParamNumInUint64:  4,
				ResultNumInUint64: 1,
			},
			expLocalIndexToStackHeightInUint64: map[uint32]int{
				0: 0,
				1: 1,
				2: 3,
			},
		},
		{
			name: "no function local, args<results",
			sig: &wasm.FunctionType{
				Params:            []wasm.ValueType{},
				Results:           []wasm.ValueType{i32},
				ParamNumInUint64:  0,
				ResultNumInUint64: 1,
			},
			callFrameStackSizeInUint64:         4,
			expLocalIndexToStackHeightInUint64: map[uint32]int{},
		},
		{
			name: "no function local, args<results",
			sig: &wasm.FunctionType{
				Params:            []wasm.ValueType{i32},
				Results:           []wasm.ValueType{i32, f32},
				ParamNumInUint64:  1,
				ResultNumInUint64: 2,
			},
			callFrameStackSizeInUint64:         4,
			expLocalIndexToStackHeightInUint64: map[uint32]int{0: 0},
		},
		{
			name: "no function local, args<results, with vector",
			sig: &wasm.FunctionType{
				Params:            []wasm.ValueType{i32},
				Results:           []wasm.ValueType{i32, v128, f32},
				ParamNumInUint64:  1,
				ResultNumInUint64: 4,
			},
			callFrameStackSizeInUint64:         4,
			expLocalIndexToStackHeightInUint64: map[uint32]int{0: 0},
		},

		// With function locals
		{
			name: "function locals, args>results",
			sig: &wasm.FunctionType{
				Params:            []wasm.ValueType{i32, f32},
				Results:           []wasm.ValueType{i32},
				ParamNumInUint64:  2,
				ResultNumInUint64: 1,
			},
			functionLocalTypes:         []wasm.ValueType{f64},
			callFrameStackSizeInUint64: 4,
			// [i32, f32, callframe.0, callframe.1, callframe.2, callframe.3, f64]
			expLocalIndexToStackHeightInUint64: map[uint32]int{
				0: 0,
				1: 1,
				// Function local comes after call frame.
				2: 6,
			},
		},
		{
			name: "function locals, args>results, with vector",
			sig: &wasm.FunctionType{
				Params:            []wasm.ValueType{i32, v128, f32},
				Results:           []wasm.ValueType{i32},
				ParamNumInUint64:  4,
				ResultNumInUint64: 1,
			},
			functionLocalTypes:         []wasm.ValueType{v128, v128},
			callFrameStackSizeInUint64: 4,
			// [i32, v128.lo, v128.hi, f32, callframe.0, callframe.1, callframe.2, callframe.3, v128.lo, v128.hi, v128.lo, v128.hi]
			expLocalIndexToStackHeightInUint64: map[uint32]int{
				0: 0,
				1: 1,
				2: 3,
				// Function local comes after call frame.
				3: 8,
				4: 10,
			},
		},
		{
			name: "function locals, args<results",
			sig: &wasm.FunctionType{
				Params:            []wasm.ValueType{i32},
				Results:           []wasm.ValueType{i32, i32, i32, i32},
				ParamNumInUint64:  1,
				ResultNumInUint64: 4,
			},
			functionLocalTypes:         []wasm.ValueType{f64},
			callFrameStackSizeInUint64: 4,
			// [i32, _, _, _, callframe.0, callframe.1, callframe.2, callframe.3, f64]
			expLocalIndexToStackHeightInUint64: map[uint32]int{
				0: 0,
				1: 8,
			},
		},
		{
			name: "function locals, args<results with vector",
			sig: &wasm.FunctionType{
				Params:            []wasm.ValueType{v128, f64},
				Results:           []wasm.ValueType{v128, i32, i32, v128},
				ParamNumInUint64:  3,
				ResultNumInUint64: 6,
			},
			functionLocalTypes:         []wasm.ValueType{f64},
			callFrameStackSizeInUint64: 4,
			// [v128.lo, v128.hi, f64, _, _, _, callframe.0, callframe.1, callframe.2, callframe.3, f64]
			expLocalIndexToStackHeightInUint64: map[uint32]int{
				0: 0,
				1: 2,
				2: 10,
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			c := &compiler{
				sig: tc.sig, localTypes: tc.functionLocalTypes,
				callFrameStackSizeInUint64: tc.callFrameStackSizeInUint64,
			}

			c.initializeStack()
			require.Equal(t, tc.expLocalIndexToStackHeightInUint64, c.localIndexToStackHeightInUint64)
		})

	}
}

func Test_ensureTermination(t *testing.T) {
	for _, tc := range []struct {
		ensureTermination bool
		exp               string
	}{
		{
			ensureTermination: true,
			exp: `.entrypoint
	ConstI32 0x0
	Br .L2
.L2
	BuiltinFunctionCheckExitCode
	ConstI32 0x1
	BrIf .L2, .L3
.L3
	ConstI32 0x1
	Br .L4
.L4
	BuiltinFunctionCheckExitCode
	i32.Add
	Drop 0..0
	Br .return
`,
		},
		{
			ensureTermination: false,
			exp: `.entrypoint
	ConstI32 0x0
	Br .L2
.L2
	ConstI32 0x1
	BrIf .L2, .L3
.L3
	ConstI32 0x1
	Br .L4
.L4
	i32.Add
	Drop 0..0
	Br .return
`,
		},
	} {
		t.Run(fmt.Sprintf("%v", tc.ensureTermination), func(t *testing.T) {
			mod := &wasm.Module{
				TypeSection:     []wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0},
				CodeSection: []wasm.Code{{
					Body: []byte{
						wasm.OpcodeI32Const, 0,
						wasm.OpcodeLoop, 0, wasm.OpcodeI32Const, 1, wasm.OpcodeBrIf, 0, wasm.OpcodeEnd,
						wasm.OpcodeI32Const, 1,
						wasm.OpcodeLoop, 0, wasm.OpcodeEnd,
						wasm.OpcodeI32Add,
						wasm.OpcodeDrop,
						wasm.OpcodeEnd,
					},
				}},
			}
			res, err := CompileFunctions(api.CoreFeaturesV2, 0, mod, tc.ensureTermination)
			require.NoError(t, err)
			require.Equal(t, tc.exp, Format(res[0].Operations))
		})
	}
}
