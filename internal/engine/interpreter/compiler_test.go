package interpreter

import (
	"fmt"
	"math"
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
)

var (
	wf32, wf64, i32 = wasm.ValueTypeF32, wasm.ValueTypeF64, wasm.ValueTypeI32
	f32_i32         = wasm.FunctionType{
		Params: []wasm.ValueType{wf32}, Results: []wasm.ValueType{i32},
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
	v_f64f64 = wasm.FunctionType{Results: []wasm.ValueType{wf64, wf64}, ResultNumInUint64: 2}
)

func TestCompile(t *testing.T) {
	tests := []struct {
		name            string
		module          *wasm.Module
		expected        *compilationResult
		enabledFeatures api.CoreFeatures
	}{
		{
			name: "nullary",
			module: &wasm.Module{
				TypeSection:     []wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []wasm.Code{{Body: []byte{wasm.OpcodeEnd}}},
			},
			expected: &compilationResult{
				Operations: []unionOperation{ // begin with params: []
					newOperationBr(newLabel(labelKindReturn, 0)), // return!
				},
				LabelCallers: map[label]uint32{},
				Functions:    []uint32{0},
				Types:        []wasm.FunctionType{v_v},
			},
		},
		{
			name: "host wasm nullary",
			module: &wasm.Module{
				TypeSection:     []wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []wasm.Code{{Body: []byte{wasm.OpcodeEnd}}},
			},
			expected: &compilationResult{
				Operations: []unionOperation{ // begin with params: []
					newOperationBr(newLabel(labelKindReturn, 0)), // return!
				},
				LabelCallers: map[label]uint32{},
				Functions:    []uint32{0},
				Types:        []wasm.FunctionType{v_v},
			},
		},
		{
			name: "identity",
			module: &wasm.Module{
				TypeSection:     []wasm.FunctionType{i32_i32},
				FunctionSection: []wasm.Index{0},
				CodeSection:     []wasm.Code{{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeEnd}}},
			},
			expected: &compilationResult{
				Operations: []unionOperation{ // begin with params: [$x]
					newOperationPick(0, false),                         // [$x, $x]
					newOperationDrop(inclusiveRange{Start: 1, End: 1}), // [$x]
					newOperationBr(newLabel(labelKindReturn, 0)),       // return!
				},
				LabelCallers: map[label]uint32{},
				Types: []wasm.FunctionType{
					{
						Params: []wasm.ValueType{i32}, Results: []wasm.ValueType{i32},
						ParamNumInUint64:  1,
						ResultNumInUint64: 1,
					},
				},
				Functions: []uint32{0},
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
			expected: &compilationResult{
				Operations: []unionOperation{ // begin with params: []
					newOperationConstI32(8), // [8]
					newOperationLoad(unsignedTypeI32, memoryArg{Alignment: 2, Offset: 0}), // [x]
					newOperationDrop(inclusiveRange{}),                                    // []
					newOperationBr(newLabel(labelKindReturn, 0)),                          // return!
				},
				LabelCallers: map[label]uint32{},
				Types:        []wasm.FunctionType{v_v},
				Functions:    []uint32{0},
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
			expected: &compilationResult{
				Operations: []unionOperation{ // begin with params: []
					newOperationConstI32(8), // [8]
					newOperationLoad(unsignedTypeI32, memoryArg{Alignment: 2, Offset: 0}), // [x]
					newOperationDrop(inclusiveRange{}),                                    // []
					newOperationBr(newLabel(labelKindReturn, 0)),                          // return!
				},
				LabelCallers: map[label]uint32{},
				Types:        []wasm.FunctionType{v_v},
				Functions:    []uint32{0},
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
			expected: &compilationResult{
				Operations: []unionOperation{ // begin with params: [$delta]
					newOperationPick(0, false),                         // [$delta, $delta]
					newOperationMemoryGrow(),                           // [$delta, $old_size]
					newOperationDrop(inclusiveRange{Start: 1, End: 1}), // [$old_size]
					newOperationBr(newLabel(labelKindReturn, 0)),       // return!
				},
				LabelCallers: map[label]uint32{},
				Types: []wasm.FunctionType{{
					Params: []wasm.ValueType{i32}, Results: []wasm.ValueType{i32},
					ParamNumInUint64:  1,
					ResultNumInUint64: 1,
				}},
				Functions:  []uint32{0},
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
			c, err := newCompiler(enabledFeatures, 0, tc.module, false)
			require.NoError(t, err)

			fn, err := c.Next()
			require.NoError(t, err)
			require.Equal(t, tc.expected, fn)
		})
	}
}

func TestCompile_Block(t *testing.T) {
	tests := []struct {
		name            string
		module          *wasm.Module
		expected        *compilationResult
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
			expected: &compilationResult{
				Operations: []unionOperation{ // begin with params: []
					newOperationBr(newLabel(labelKindContinuation, 2)),    // arbitrary FrameID
					newOperationLabel(newLabel(labelKindContinuation, 2)), // arbitrary FrameID
					newOperationBr(newLabel(labelKindReturn, 0)),
				},
				// Note: i32.add comes after br 0 so is unreachable. Compilation succeeds when it feels like it
				// shouldn't because the br instruction is stack-polymorphic. In other words, (br 0) substitutes for the
				// two i32 parameters to add.
				LabelCallers: map[label]uint32{newLabel(labelKindContinuation, 2): 1},
				Functions:    []uint32{0},
				Types:        []wasm.FunctionType{v_v},
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

	expected := &compilationResult{
		Operations: []unionOperation{ // begin with params: []
			newOperationConstI32(16),                     // [16]
			newOperationConstI32(0),                      // [16, 0]
			newOperationConstI32(7),                      // [16, 0, 7]
			newOperationMemoryInit(1),                    // []
			newOperationDataDrop(1),                      // []
			newOperationBr(newLabel(labelKindReturn, 0)), // return!
		},
		Memory:           memoryTypeStandard,
		UsesMemory:       true,
		HasDataInstances: true,
		LabelCallers:     map[label]uint32{},
		Functions:        []wasm.Index{0},
		Types:            []wasm.FunctionType{v_v},
	}

	c, err := newCompiler(api.CoreFeatureBulkMemoryOperations, 0, module, false)
	require.NoError(t, err)

	actual, err := c.Next()
	require.NoError(t, err)
	require.Equal(t, expected, actual)
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
		expected        *compilationResult
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
			expected: &compilationResult{
				Operations: []unionOperation{ // begin with params: [$x, $y]
					newOperationPick(0, false),                         // [$x, $y, $y]
					newOperationPick(2, false),                         // [$x, $y, $y, $x]
					newOperationDrop(inclusiveRange{Start: 2, End: 3}), // [$y, $x]
					newOperationBr(newLabel(labelKindReturn, 0)),       // return!
				},
				LabelCallers: map[label]uint32{},
				Functions:    []wasm.Index{0},
				Types:        []wasm.FunctionType{i32i32_i32i32},
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
			expected: &compilationResult{
				Operations: []unionOperation{ // begin with params: []
					newOperationConstF64(4),                               // [4]
					newOperationConstF64(5),                               // [4, 5]
					newOperationBr(newLabel(labelKindContinuation, 2)),    // arbitrary FrameID
					newOperationLabel(newLabel(labelKindContinuation, 2)), // arbitrary FrameID
					newOperationBr(newLabel(labelKindReturn, 0)),          // return!
				},
				// Note: f64.add comes after br 0 so is unreachable. This is why neither the add, nor its other operand
				// are in the above compilation result.
				LabelCallers: map[label]uint32{newLabel(labelKindContinuation, 2): 1}, // arbitrary label
				Functions:    []wasm.Index{0},
				Types:        []wasm.FunctionType{v_f64f64},
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
			expected: &compilationResult{
				Operations: []unionOperation{ // begin with params: []
					newOperationConstI32(306),                    // [306]
					newOperationConstI64(356),                    // [306, 356]
					newOperationBr(newLabel(labelKindReturn, 0)), // return!
				},
				LabelCallers: map[label]uint32{},
				Functions:    []wasm.Index{0},
				Types:        []wasm.FunctionType{_i32i64},
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
			expected: &compilationResult{
				Operations: []unionOperation{ // begin with params: [$0]
					newOperationConstI32(1),    // [$0, 1]
					newOperationPick(1, false), // [$0, 1, $0]
					newOperationBrIf( // [$0, 1]
						newLabel(labelKindHeader, 2),
						newLabel(labelKindElse, 2),
						nopinclusiveRange,
					),
					newOperationLabel(newLabel(labelKindHeader, 2)),
					newOperationConstI32(2),          // [$0, 1, 2]
					newOperationAdd(unsignedTypeI32), // [$0, 3]
					newOperationBr(newLabel(labelKindContinuation, 2)),
					newOperationLabel(newLabel(labelKindElse, 2)),
					newOperationConstI32(uint32(api.EncodeI32(-2))), // [$0, 1, -2]
					newOperationAdd(unsignedTypeI32),                // [$0, -1]
					newOperationBr(newLabel(labelKindContinuation, 2)),
					newOperationLabel(newLabel(labelKindContinuation, 2)),
					newOperationDrop(inclusiveRange{Start: 1, End: 1}), // .L2 = [3], .L2_else = [-1]
					newOperationBr(newLabel(labelKindReturn, 0)),
				},
				LabelCallers: map[label]uint32{
					newLabel(labelKindHeader, 2):       1,
					newLabel(labelKindContinuation, 2): 2,
					newLabel(labelKindElse, 2):         1,
				},
				Functions: []wasm.Index{0},
				Types:     []wasm.FunctionType{i32_i32},
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
			expected: &compilationResult{
				Operations: []unionOperation{ // begin with params: [$0]
					newOperationConstI32(1),    // [$0, 1]
					newOperationConstI32(2),    // [$0, 1, 2]
					newOperationPick(2, false), // [$0, 1, 2, $0]
					newOperationBrIf( // [$0, 1, 2]
						newLabel(labelKindHeader, 2),
						newLabel(labelKindElse, 2),
						nopinclusiveRange,
					),
					newOperationLabel(newLabel(labelKindHeader, 2)),
					newOperationAdd(unsignedTypeI32), // [$0, 3]
					newOperationBr(newLabel(labelKindContinuation, 2)),
					newOperationLabel(newLabel(labelKindElse, 2)),
					newOperationSub(unsignedTypeI32), // [$0, -1]
					newOperationBr(newLabel(labelKindContinuation, 2)),
					newOperationLabel(newLabel(labelKindContinuation, 2)),
					newOperationDrop(inclusiveRange{Start: 1, End: 1}), // .L2 = [3], .L2_else = [-1]
					newOperationBr(newLabel(labelKindReturn, 0)),
				},
				LabelCallers: map[label]uint32{
					newLabel(labelKindHeader, 2):       1,
					newLabel(labelKindContinuation, 2): 2,
					newLabel(labelKindElse, 2):         1,
				},
				Functions: []wasm.Index{0},
				Types:     []wasm.FunctionType{i32_i32, i32i32_i32},
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
			expected: &compilationResult{
				Operations: []unionOperation{ // begin with params: [$0]
					newOperationConstI32(1),    // [$0, 1]
					newOperationConstI32(2),    // [$0, 1, 2]
					newOperationPick(2, false), // [$0, 1, 2, $0]
					newOperationBrIf( // [$0, 1, 2]
						newLabel(labelKindHeader, 2),
						newLabel(labelKindElse, 2),
						nopinclusiveRange,
					),
					newOperationLabel(newLabel(labelKindHeader, 2)),
					newOperationAdd(unsignedTypeI32), // [$0, 3]
					newOperationBr(newLabel(labelKindContinuation, 2)),
					newOperationLabel(newLabel(labelKindElse, 2)),
					newOperationSub(unsignedTypeI32), // [$0, -1]
					newOperationBr(newLabel(labelKindContinuation, 2)),
					newOperationLabel(newLabel(labelKindContinuation, 2)),
					newOperationDrop(inclusiveRange{Start: 1, End: 1}), // .L2 = [3], .L2_else = [-1]
					newOperationBr(newLabel(labelKindReturn, 0)),
				},
				LabelCallers: map[label]uint32{
					newLabel(labelKindHeader, 2):       1,
					newLabel(labelKindContinuation, 2): 2,
					newLabel(labelKindElse, 2):         1,
				},
				Functions: []wasm.Index{0},
				Types:     []wasm.FunctionType{i32_i32, i32i32_i32},
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
			c, err := newCompiler(enabledFeatures, 0, tc.module, false)
			require.NoError(t, err)

			actual, err := c.Next()
			require.NoError(t, err)
			require.Equal(t, tc.expected, actual)
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

	expected := &compilationResult{
		Operations: []unionOperation{ // begin with params: [$0]
			newOperationPick(0, false), // [$0, $0]
			newOperationITruncFromF( // [$0, i32.trunc_sat_f32_s($0)]
				f32,
				signedInt32,
				true,
			),
			newOperationDrop(inclusiveRange{Start: 1, End: 1}), // [i32.trunc_sat_f32_s($0)]
			newOperationBr(newLabel(labelKindReturn, 0)),       // return!
		},
		LabelCallers: map[label]uint32{},
		Functions:    []wasm.Index{0},
		Types:        []wasm.FunctionType{f32_i32},
	}
	c, err := newCompiler(api.CoreFeatureNonTrappingFloatToIntConversion, 0, module, false)
	require.NoError(t, err)

	actual, err := c.Next()
	require.NoError(t, err)
	require.Equal(t, expected, actual)
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

	expected := &compilationResult{
		Operations: []unionOperation{ // begin with params: [$0]
			newOperationPick(0, false),                         // [$0, $0]
			newOperationSignExtend32From8(),                    // [$0, i32.extend8_s($0)]
			newOperationDrop(inclusiveRange{Start: 1, End: 1}), // [i32.extend8_s($0)]
			newOperationBr(newLabel(labelKindReturn, 0)),       // return!
		},
		LabelCallers: map[label]uint32{},
		Functions:    []wasm.Index{0},
		Types:        []wasm.FunctionType{i32_i32},
	}
	c, err := newCompiler(api.CoreFeatureSignExtensionOps, 0, module, false)
	require.NoError(t, err)

	actual, err := c.Next()
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func requireCompilationResult(t *testing.T, enabledFeatures api.CoreFeatures, expected *compilationResult, module *wasm.Module) {
	if enabledFeatures == 0 {
		enabledFeatures = api.CoreFeaturesV2
	}
	c, err := newCompiler(enabledFeatures, 0, module, false)
	require.NoError(t, err)

	actual, err := c.Next()
	require.NoError(t, err)

	require.NoError(t, err)
	require.Equal(t, expected, actual)
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

	expected := &compilationResult{
		Operations: []unionOperation{ // begin with params: []
			newOperationConstI32(0),
			newOperationCallIndirect(2, 5),
			newOperationBr(newLabel(labelKindReturn, 0)), // return!
		},
		HasTable:     true,
		LabelCallers: map[label]uint32{},
		Functions:    []wasm.Index{0},
		Types:        []wasm.FunctionType{v_v, v_v, v_v},
	}

	c, err := newCompiler(api.CoreFeatureBulkMemoryOperations, 0, module, false)
	require.NoError(t, err)

	actual, err := c.Next()
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func TestCompile_Refs(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		expected []unionOperation
	}{
		{
			name: "ref.func",
			body: []byte{
				wasm.OpcodeRefFunc, 100,
				wasm.OpcodeDrop,
				wasm.OpcodeEnd,
			},
			expected: []unionOperation{
				newOperationRefFunc(100),
				newOperationDrop(inclusiveRange{Start: 0, End: 0}),
				newOperationBr(newLabel(labelKindReturn, 0)), // return!
			},
		},
		{
			name: "ref.null (externref)",
			body: []byte{
				wasm.OpcodeRefNull, wasm.ValueTypeExternref,
				wasm.OpcodeDrop,
				wasm.OpcodeEnd,
			},
			expected: []unionOperation{
				newOperationConstI64(0),
				newOperationDrop(inclusiveRange{Start: 0, End: 0}),
				newOperationBr(newLabel(labelKindReturn, 0)), // return!
			},
		},
		{
			name: "ref.null (funcref)",
			body: []byte{
				wasm.OpcodeRefNull, wasm.ValueTypeFuncref,
				wasm.OpcodeDrop,
				wasm.OpcodeEnd,
			},
			expected: []unionOperation{
				newOperationConstI64(0),
				newOperationDrop(inclusiveRange{Start: 0, End: 0}),
				newOperationBr(newLabel(labelKindReturn, 0)), // return!
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
			expected: []unionOperation{
				newOperationRefFunc(100),
				newOperationEqz(unsignedInt64),
				newOperationDrop(inclusiveRange{Start: 0, End: 0}),
				newOperationBr(newLabel(labelKindReturn, 0)), // return!
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
			expected: []unionOperation{
				newOperationConstI64(0),
				newOperationEqz(unsignedInt64),
				newOperationDrop(inclusiveRange{Start: 0, End: 0}),
				newOperationBr(newLabel(labelKindReturn, 0)), // return!
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
			c, err := newCompiler(api.CoreFeaturesV2, 0, module, false)
			require.NoError(t, err)

			actual, err := c.Next()
			require.NoError(t, err)
			require.Equal(t, tc.expected, actual.Operations)
		})
	}
}

func TestCompile_TableGetOrSet(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		expected []unionOperation
	}{
		{
			name: "table.get",
			body: []byte{
				wasm.OpcodeI32Const, 10,
				wasm.OpcodeTableGet, 0,
				wasm.OpcodeDrop,
				wasm.OpcodeEnd,
			},
			expected: []unionOperation{
				newOperationConstI32(10),
				newOperationTableGet(0),
				newOperationDrop(inclusiveRange{Start: 0, End: 0}),
				newOperationBr(newLabel(labelKindReturn, 0)), // return!
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
			expected: []unionOperation{
				newOperationConstI32(10),
				newOperationConstI64(0),
				newOperationTableSet(0),
				newOperationBr(newLabel(labelKindReturn, 0)), // return!
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
			expected: []unionOperation{
				newOperationConstI32(10),
				newOperationRefFunc(1),
				newOperationTableSet(0),
				newOperationBr(newLabel(labelKindReturn, 0)), // return!
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
			c, err := newCompiler(api.CoreFeaturesV2, 0, module, false)
			require.NoError(t, err)

			actual, err := c.Next()
			require.NoError(t, err)
			require.Equal(t, tc.expected, actual.Operations)
		})
	}
}

func TestCompile_TableGrowFillSize(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		expected []unionOperation
	}{
		{
			name: "table.grow",
			body: []byte{
				wasm.OpcodeRefNull, wasm.RefTypeFuncref,
				wasm.OpcodeI32Const, 1,
				wasm.OpcodeMiscPrefix, wasm.OpcodeMiscTableGrow, 1,
				wasm.OpcodeEnd,
			},
			expected: []unionOperation{
				newOperationConstI64(0), // Null ref.
				newOperationConstI32(1),
				newOperationTableGrow(1),
				newOperationDrop(inclusiveRange{Start: 0, End: 0}),
				newOperationBr(newLabel(labelKindReturn, 0)), // return!
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
			expected: []unionOperation{
				newOperationConstI32(10),
				newOperationConstI64(0), // Null ref.
				newOperationConstI32(1),
				newOperationTableFill(1),
				newOperationBr(newLabel(labelKindReturn, 0)), // return!
			},
		},
		{
			name: "table.size",
			body: []byte{
				wasm.OpcodeMiscPrefix, wasm.OpcodeMiscTableSize, 1,
				wasm.OpcodeEnd,
			},
			expected: []unionOperation{
				newOperationTableSize(1),
				newOperationDrop(inclusiveRange{Start: 0, End: 0}),
				newOperationBr(newLabel(labelKindReturn, 0)), // return!
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
			c, err := newCompiler(api.CoreFeaturesV2, 0, module, false)
			require.NoError(t, err)

			actual, err := c.Next()
			require.NoError(t, err)
			require.Equal(t, tc.expected, actual.Operations)
			require.True(t, actual.HasTable)
		})
	}
}

func TestCompile_Locals(t *testing.T) {
	tests := []struct {
		name     string
		mod      *wasm.Module
		expected []unionOperation
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
			expected: []unionOperation{
				newOperationPick(1, true), // [param[0].low, param[0].high] -> [param[0].low, param[0].high, param[0].low, param[0].high]
				newOperationDrop(inclusiveRange{Start: 0, End: 3}),
				newOperationBr(newLabel(labelKindReturn, 0)), // return!
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
			expected: []unionOperation{
				newOperationPick(0, false), // [param[0]] -> [param[0], param[0]]
				newOperationDrop(inclusiveRange{Start: 0, End: 1}),
				newOperationBr(newLabel(labelKindReturn, 0)), // return!
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
			expected: []unionOperation{
				newOperationV128Const(0, 0),
				newOperationPick(1, true), // [p[0].low, p[0].high] -> [p[0].low, p[0].high, p[0].low, p[0].high]
				newOperationDrop(inclusiveRange{Start: 0, End: 3}),
				newOperationBr(newLabel(labelKindReturn, 0)), // return!
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
			expected: []unionOperation{
				// [p[0].lo, p[1].hi] -> [p[0].lo, p[1].hi, 0x01, 0x02]
				newOperationV128Const(0x01, 0x02),
				// [p[0].lo, p[1].hi, 0x01, 0x02] -> [0x01, 0x02]
				newOperationSet(3, true),
				newOperationDrop(inclusiveRange{Start: 0, End: 1}),
				newOperationBr(newLabel(labelKindReturn, 0)), // return!
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
			expected: []unionOperation{
				newOperationConstI32(0x1),
				newOperationSet(1, false),
				newOperationDrop(inclusiveRange{Start: 0, End: 0}),
				newOperationBr(newLabel(labelKindReturn, 0)), // return!
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
			expected: []unionOperation{
				newOperationV128Const(0, 0),
				// [p[0].lo, p[1].hi] -> [p[0].lo, p[1].hi, 0x01, 0x02]
				newOperationV128Const(0x01, 0x02),
				// [p[0].lo, p[1].hi, 0x01, 0x02] -> [0x01, 0x02]
				newOperationSet(3, true),
				newOperationDrop(inclusiveRange{Start: 0, End: 1}),
				newOperationBr(newLabel(labelKindReturn, 0)), // return!
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
			expected: []unionOperation{
				// [p[0].lo, p[1].hi] -> [p[0].lo, p[1].hi, 0x01, 0x02]
				newOperationV128Const(0x01, 0x02),
				// [p[0].lo, p[1].hi, 0x01, 0x02] -> [p[0].lo, p[1].hi, 0x01, 0x02, 0x01, 0x02]
				newOperationPick(1, true),
				// [p[0].lo, p[1].hi, 0x01, 0x02, 0x01, 0x02] -> [0x01, 0x02, 0x01, 0x02]
				newOperationSet(5, true),
				newOperationDrop(inclusiveRange{Start: 0, End: 3}),
				newOperationBr(newLabel(labelKindReturn, 0)), // return!
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
			expected: []unionOperation{
				newOperationConstF32(math.Float32frombits(1)),
				newOperationPick(0, false),
				newOperationSet(2, false),
				newOperationDrop(inclusiveRange{Start: 0, End: 1}),
				newOperationBr(newLabel(labelKindReturn, 0)), // return!
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
			expected: []unionOperation{
				newOperationV128Const(0, 0),
				// [p[0].lo, p[1].hi] -> [p[0].lo, p[1].hi, 0x01, 0x02]
				newOperationV128Const(0x01, 0x02),
				// [p[0].lo, p[1].hi, 0x01, 0x02] -> [p[0].lo, p[1].hi, 0x01, 0x02, 0x01, 0x02]
				newOperationPick(1, true),
				// [p[0].lo, p[1].hi, 0x01, 0x02, 0x01, 0x2] -> [0x01, 0x02, 0x01, 0x02]
				newOperationSet(5, true),
				newOperationDrop(inclusiveRange{Start: 0, End: 3}),
				newOperationBr(newLabel(labelKindReturn, 0)), // return!
			},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			c, err := newCompiler(api.CoreFeaturesV2, 0, tc.mod, false)
			require.NoError(t, err)

			actual, err := c.Next()
			require.NoError(t, err)
			msg := fmt.Sprintf("\nhave:\n\t%s\nwant:\n\t%s", format(actual.Operations), format(tc.expected))
			require.Equal(t, tc.expected, actual.Operations, msg)
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
		expected             unionOperation
		needDropBeforeReturn bool
	}{
		{
			name:                 "i8x16 add",
			needDropBeforeReturn: true,
			body:                 vv2v(wasm.OpcodeVecI8x16Add),
			expected:             newOperationV128Add(shapeI8x16),
		},
		{
			name:                 "i8x16 add",
			needDropBeforeReturn: true,
			body:                 vv2v(wasm.OpcodeVecI16x8Add),
			expected:             newOperationV128Add(shapeI16x8),
		},
		{
			name:                 "i32x2 add",
			needDropBeforeReturn: true,
			body:                 vv2v(wasm.OpcodeVecI64x2Add),
			expected:             newOperationV128Add(shapeI64x2),
		},
		{
			name:                 "i64x2 add",
			needDropBeforeReturn: true,
			body:                 vv2v(wasm.OpcodeVecI64x2Add),
			expected:             newOperationV128Add(shapeI64x2),
		},
		{
			name:                 "i8x16 sub",
			needDropBeforeReturn: true,
			body:                 vv2v(wasm.OpcodeVecI8x16Sub),
			expected:             newOperationV128Sub(shapeI8x16),
		},
		{
			name:                 "i16x8 sub",
			needDropBeforeReturn: true,
			body:                 vv2v(wasm.OpcodeVecI16x8Sub),
			expected:             newOperationV128Sub(shapeI16x8),
		},
		{
			name:                 "i32x2 sub",
			needDropBeforeReturn: true,
			body:                 vv2v(wasm.OpcodeVecI64x2Sub),
			expected:             newOperationV128Sub(shapeI64x2),
		},
		{
			name:                 "i64x2 sub",
			needDropBeforeReturn: true,
			body:                 vv2v(wasm.OpcodeVecI64x2Sub),
			expected:             newOperationV128Sub(shapeI64x2),
		},
		{
			name: wasm.OpcodeVecV128LoadName, body: load(wasm.OpcodeVecV128Load, 0, 0),
			needDropBeforeReturn: true,
			expected:             newOperationV128Load(v128LoadType128, memoryArg{Alignment: 0, Offset: 0}),
		},
		{
			name: wasm.OpcodeVecV128LoadName + "/align=4", body: load(wasm.OpcodeVecV128Load, 0, 4),
			needDropBeforeReturn: true,
			expected:             newOperationV128Load(v128LoadType128, memoryArg{Alignment: 4, Offset: 0}),
		},
		{
			name: wasm.OpcodeVecV128Load8x8SName, body: load(wasm.OpcodeVecV128Load8x8s, 1, 0),
			needDropBeforeReturn: true,
			expected:             newOperationV128Load(v128LoadType8x8s, memoryArg{Alignment: 0, Offset: 1}),
		},
		{
			name: wasm.OpcodeVecV128Load8x8SName + "/align=1", body: load(wasm.OpcodeVecV128Load8x8s, 0, 1),
			needDropBeforeReturn: true,
			expected:             newOperationV128Load(v128LoadType8x8s, memoryArg{Alignment: 1, Offset: 0}),
		},
		{
			name: wasm.OpcodeVecV128Load8x8UName, body: load(wasm.OpcodeVecV128Load8x8u, 0, 0),
			needDropBeforeReturn: true,
			expected:             newOperationV128Load(v128LoadType8x8u, memoryArg{Alignment: 0, Offset: 0}),
		},
		{
			name: wasm.OpcodeVecV128Load8x8UName + "/align=1", body: load(wasm.OpcodeVecV128Load8x8u, 0, 1),
			needDropBeforeReturn: true,
			expected:             newOperationV128Load(v128LoadType8x8u, memoryArg{Alignment: 1, Offset: 0}),
		},
		{
			name: wasm.OpcodeVecV128Load16x4SName, body: load(wasm.OpcodeVecV128Load16x4s, 1, 0),
			needDropBeforeReturn: true,
			expected:             newOperationV128Load(v128LoadType16x4s, memoryArg{Alignment: 0, Offset: 1}),
		},
		{
			name: wasm.OpcodeVecV128Load16x4SName + "/align=2", body: load(wasm.OpcodeVecV128Load16x4s, 0, 2),
			needDropBeforeReturn: true,
			expected:             newOperationV128Load(v128LoadType16x4s, memoryArg{Alignment: 2, Offset: 0}),
		},
		{
			name: wasm.OpcodeVecV128Load16x4UName, body: load(wasm.OpcodeVecV128Load16x4u, 0, 0),
			needDropBeforeReturn: true,
			expected:             newOperationV128Load(v128LoadType16x4u, memoryArg{Alignment: 0, Offset: 0}),
		},
		{
			name: wasm.OpcodeVecV128Load16x4UName + "/align=2", body: load(wasm.OpcodeVecV128Load16x4u, 0, 2),
			needDropBeforeReturn: true,
			expected:             newOperationV128Load(v128LoadType16x4u, memoryArg{Alignment: 2, Offset: 0}),
		},
		{
			name: wasm.OpcodeVecV128Load32x2SName, body: load(wasm.OpcodeVecV128Load32x2s, 1, 0),
			needDropBeforeReturn: true,
			expected:             newOperationV128Load(v128LoadType32x2s, memoryArg{Alignment: 0, Offset: 1}),
		},
		{
			name: wasm.OpcodeVecV128Load32x2SName + "/align=3", body: load(wasm.OpcodeVecV128Load32x2s, 0, 3),
			needDropBeforeReturn: true,
			expected:             newOperationV128Load(v128LoadType32x2s, memoryArg{Alignment: 3, Offset: 0}),
		},
		{
			name: wasm.OpcodeVecV128Load32x2UName, body: load(wasm.OpcodeVecV128Load32x2u, 0, 0),
			needDropBeforeReturn: true,
			expected:             newOperationV128Load(v128LoadType32x2u, memoryArg{Alignment: 0, Offset: 0}),
		},
		{
			name: wasm.OpcodeVecV128Load32x2UName + "/align=3", body: load(wasm.OpcodeVecV128Load32x2u, 0, 3),
			needDropBeforeReturn: true,
			expected:             newOperationV128Load(v128LoadType32x2u, memoryArg{Alignment: 3, Offset: 0}),
		},
		{
			name: wasm.OpcodeVecV128Load8SplatName, body: load(wasm.OpcodeVecV128Load8Splat, 2, 0),
			needDropBeforeReturn: true,
			expected:             newOperationV128Load(v128LoadType8Splat, memoryArg{Alignment: 0, Offset: 2}),
		},
		{
			name: wasm.OpcodeVecV128Load16SplatName, body: load(wasm.OpcodeVecV128Load16Splat, 0, 1),
			needDropBeforeReturn: true,
			expected:             newOperationV128Load(v128LoadType16Splat, memoryArg{Alignment: 1, Offset: 0}),
		},
		{
			name: wasm.OpcodeVecV128Load32SplatName, body: load(wasm.OpcodeVecV128Load32Splat, 3, 2),
			needDropBeforeReturn: true,
			expected:             newOperationV128Load(v128LoadType32Splat, memoryArg{Alignment: 2, Offset: 3}),
		},
		{
			name: wasm.OpcodeVecV128Load64SplatName, body: load(wasm.OpcodeVecV128Load64Splat, 0, 3),
			needDropBeforeReturn: true,
			expected:             newOperationV128Load(v128LoadType64Splat, memoryArg{Alignment: 3, Offset: 0}),
		},
		{
			name: wasm.OpcodeVecV128Load32zeroName, body: load(wasm.OpcodeVecV128Load32zero, 0, 2),
			needDropBeforeReturn: true,
			expected:             newOperationV128Load(v128LoadType32zero, memoryArg{Alignment: 2, Offset: 0}),
		},
		{
			name: wasm.OpcodeVecV128Load64zeroName, body: load(wasm.OpcodeVecV128Load64zero, 5, 3),
			needDropBeforeReturn: true,
			expected:             newOperationV128Load(v128LoadType64zero, memoryArg{Alignment: 3, Offset: 5}),
		},
		{
			name: wasm.OpcodeVecV128Load8LaneName, needDropBeforeReturn: true,
			body:     loadLane(wasm.OpcodeVecV128Load8Lane, 5, 0, 10),
			expected: newOperationV128LoadLane(10, 8, memoryArg{Alignment: 0, Offset: 5}),
		},
		{
			name: wasm.OpcodeVecV128Load16LaneName, needDropBeforeReturn: true,
			body:     loadLane(wasm.OpcodeVecV128Load16Lane, 100, 1, 7),
			expected: newOperationV128LoadLane(7, 16, memoryArg{Alignment: 1, Offset: 100}),
		},
		{
			name: wasm.OpcodeVecV128Load32LaneName, needDropBeforeReturn: true,
			body:     loadLane(wasm.OpcodeVecV128Load32Lane, 0, 2, 3),
			expected: newOperationV128LoadLane(3, 32, memoryArg{Alignment: 2, Offset: 0}),
		},
		{
			name: wasm.OpcodeVecV128Load64LaneName, needDropBeforeReturn: true,
			body:     loadLane(wasm.OpcodeVecV128Load64Lane, 0, 3, 1),
			expected: newOperationV128LoadLane(1, 64, memoryArg{Alignment: 3, Offset: 0}),
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
			expected: newOperationV128Store(memoryArg{Alignment: 4, Offset: 10}),
		},
		{
			name:     wasm.OpcodeVecV128Store8LaneName,
			body:     storeLane(wasm.OpcodeVecV128Store8Lane, 0, 0, 0),
			expected: newOperationV128StoreLane(0, 8, memoryArg{Alignment: 0, Offset: 0}),
		},
		{
			name:     wasm.OpcodeVecV128Store8LaneName + "/lane=15",
			body:     storeLane(wasm.OpcodeVecV128Store8Lane, 100, 0, 15),
			expected: newOperationV128StoreLane(15, 8, memoryArg{Alignment: 0, Offset: 100}),
		},
		{
			name:     wasm.OpcodeVecV128Store16LaneName,
			body:     storeLane(wasm.OpcodeVecV128Store16Lane, 0, 0, 0),
			expected: newOperationV128StoreLane(0, 16, memoryArg{Alignment: 0, Offset: 0}),
		},
		{
			name:     wasm.OpcodeVecV128Store16LaneName + "/lane=7/align=1",
			body:     storeLane(wasm.OpcodeVecV128Store16Lane, 100, 1, 7),
			expected: newOperationV128StoreLane(7, 16, memoryArg{Alignment: 1, Offset: 100}),
		},
		{
			name:     wasm.OpcodeVecV128Store32LaneName,
			body:     storeLane(wasm.OpcodeVecV128Store32Lane, 0, 0, 0),
			expected: newOperationV128StoreLane(0, 32, memoryArg{Alignment: 0, Offset: 0}),
		},
		{
			name:     wasm.OpcodeVecV128Store32LaneName + "/lane=3/align=2",
			body:     storeLane(wasm.OpcodeVecV128Store32Lane, 100, 2, 3),
			expected: newOperationV128StoreLane(3, 32, memoryArg{Alignment: 2, Offset: 100}),
		},
		{
			name:     wasm.OpcodeVecV128Store64LaneName,
			body:     storeLane(wasm.OpcodeVecV128Store64Lane, 0, 0, 0),
			expected: newOperationV128StoreLane(0, 64, memoryArg{Alignment: 0, Offset: 0}),
		},
		{
			name:     wasm.OpcodeVecV128Store64LaneName + "/lane=1/align=3",
			body:     storeLane(wasm.OpcodeVecV128Store64Lane, 50, 3, 1),
			expected: newOperationV128StoreLane(1, 64, memoryArg{Alignment: 3, Offset: 50}),
		},
		{
			name:                 wasm.OpcodeVecI8x16ExtractLaneSName,
			body:                 extractLane(wasm.OpcodeVecI8x16ExtractLaneS, 0),
			expected:             newOperationV128ExtractLane(0, true, shapeI8x16),
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI8x16ExtractLaneSName + "/lane=15",
			body:                 extractLane(wasm.OpcodeVecI8x16ExtractLaneS, 15),
			expected:             newOperationV128ExtractLane(15, true, shapeI8x16),
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI8x16ExtractLaneUName,
			body:                 extractLane(wasm.OpcodeVecI8x16ExtractLaneU, 0),
			expected:             newOperationV128ExtractLane(0, false, shapeI8x16),
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI8x16ExtractLaneUName + "/lane=15",
			body:                 extractLane(wasm.OpcodeVecI8x16ExtractLaneU, 15),
			expected:             newOperationV128ExtractLane(15, false, shapeI8x16),
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI16x8ExtractLaneSName,
			body:                 extractLane(wasm.OpcodeVecI16x8ExtractLaneS, 0),
			expected:             newOperationV128ExtractLane(0, true, shapeI16x8),
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI16x8ExtractLaneSName + "/lane=7",
			body:                 extractLane(wasm.OpcodeVecI16x8ExtractLaneS, 7),
			expected:             newOperationV128ExtractLane(7, true, shapeI16x8),
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI16x8ExtractLaneUName,
			body:                 extractLane(wasm.OpcodeVecI16x8ExtractLaneU, 0),
			expected:             newOperationV128ExtractLane(0, false, shapeI16x8),
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI16x8ExtractLaneUName + "/lane=7",
			body:                 extractLane(wasm.OpcodeVecI16x8ExtractLaneU, 7),
			expected:             newOperationV128ExtractLane(7, false, shapeI16x8),
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI32x4ExtractLaneName,
			body:                 extractLane(wasm.OpcodeVecI32x4ExtractLane, 0),
			expected:             newOperationV128ExtractLane(0, false, shapeI32x4),
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI32x4ExtractLaneName + "/lane=3",
			body:                 extractLane(wasm.OpcodeVecI32x4ExtractLane, 3),
			expected:             newOperationV128ExtractLane(3, false, shapeI32x4),
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI64x2ExtractLaneName,
			body:                 extractLane(wasm.OpcodeVecI64x2ExtractLane, 0),
			expected:             newOperationV128ExtractLane(0, false, shapeI64x2),
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI64x2ExtractLaneName + "/lane=1",
			body:                 extractLane(wasm.OpcodeVecI64x2ExtractLane, 1),
			expected:             newOperationV128ExtractLane(1, false, shapeI64x2),
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecF32x4ExtractLaneName,
			body:                 extractLane(wasm.OpcodeVecF32x4ExtractLane, 0),
			expected:             newOperationV128ExtractLane(0, false, shapeF32x4),
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecF32x4ExtractLaneName + "/lane=3",
			body:                 extractLane(wasm.OpcodeVecF32x4ExtractLane, 3),
			expected:             newOperationV128ExtractLane(3, false, shapeF32x4),
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecF64x2ExtractLaneName,
			body:                 extractLane(wasm.OpcodeVecF64x2ExtractLane, 0),
			expected:             newOperationV128ExtractLane(0, false, shapeF64x2),
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecF64x2ExtractLaneName + "/lane=1",
			body:                 extractLane(wasm.OpcodeVecF64x2ExtractLane, 1),
			expected:             newOperationV128ExtractLane(1, false, shapeF64x2),
			needDropBeforeReturn: true,
		},

		{
			name:                 wasm.OpcodeVecI8x16ReplaceLaneName,
			body:                 replaceLane(wasm.OpcodeVecI8x16ReplaceLane, 0),
			expected:             newOperationV128ReplaceLane(0, shapeI8x16),
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI8x16ReplaceLaneName + "/lane=15",
			body:                 replaceLane(wasm.OpcodeVecI8x16ReplaceLane, 15),
			expected:             newOperationV128ReplaceLane(15, shapeI8x16),
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI16x8ReplaceLaneName,
			body:                 replaceLane(wasm.OpcodeVecI16x8ReplaceLane, 0),
			expected:             newOperationV128ReplaceLane(0, shapeI16x8),
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI16x8ReplaceLaneName + "/lane=7",
			body:                 replaceLane(wasm.OpcodeVecI16x8ReplaceLane, 7),
			expected:             newOperationV128ReplaceLane(7, shapeI16x8),
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI32x4ReplaceLaneName,
			body:                 replaceLane(wasm.OpcodeVecI32x4ReplaceLane, 0),
			expected:             newOperationV128ReplaceLane(0, shapeI32x4),
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI32x4ReplaceLaneName + "/lane=3",
			body:                 replaceLane(wasm.OpcodeVecI32x4ReplaceLane, 3),
			expected:             newOperationV128ReplaceLane(3, shapeI32x4),
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI64x2ReplaceLaneName,
			body:                 replaceLane(wasm.OpcodeVecI64x2ReplaceLane, 0),
			expected:             newOperationV128ReplaceLane(0, shapeI64x2),
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecI64x2ReplaceLaneName + "/lane=1",
			body:                 replaceLane(wasm.OpcodeVecI64x2ReplaceLane, 1),
			expected:             newOperationV128ReplaceLane(1, shapeI64x2),
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecF32x4ReplaceLaneName,
			body:                 replaceLane(wasm.OpcodeVecF32x4ReplaceLane, 0),
			expected:             newOperationV128ReplaceLane(0, shapeF32x4),
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecF32x4ReplaceLaneName + "/lane=3",
			body:                 replaceLane(wasm.OpcodeVecF32x4ReplaceLane, 3),
			expected:             newOperationV128ReplaceLane(3, shapeF32x4),
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecF64x2ReplaceLaneName,
			body:                 replaceLane(wasm.OpcodeVecF64x2ReplaceLane, 0),
			expected:             newOperationV128ReplaceLane(0, shapeF64x2),
			needDropBeforeReturn: true,
		},
		{
			name:                 wasm.OpcodeVecF64x2ReplaceLaneName + "/lane=1",
			body:                 replaceLane(wasm.OpcodeVecF64x2ReplaceLane, 1),
			expected:             newOperationV128ReplaceLane(1, shapeF64x2),
			needDropBeforeReturn: true,
		},
		{
			name: wasm.OpcodeVecI8x16SplatName, body: splat(wasm.OpcodeVecI8x16Splat),
			expected:             newOperationV128Splat(shapeI8x16),
			needDropBeforeReturn: true,
		},
		{
			name: wasm.OpcodeVecI16x8SplatName, body: splat(wasm.OpcodeVecI16x8Splat),
			expected:             newOperationV128Splat(shapeI16x8),
			needDropBeforeReturn: true,
		},
		{
			name: wasm.OpcodeVecI32x4SplatName, body: splat(wasm.OpcodeVecI32x4Splat),
			expected:             newOperationV128Splat(shapeI32x4),
			needDropBeforeReturn: true,
		},
		{
			name: wasm.OpcodeVecI64x2SplatName, body: splat(wasm.OpcodeVecI64x2Splat),
			expected:             newOperationV128Splat(shapeI64x2),
			needDropBeforeReturn: true,
		},
		{
			name: wasm.OpcodeVecF32x4SplatName, body: splat(wasm.OpcodeVecF32x4Splat),
			expected:             newOperationV128Splat(shapeF32x4),
			needDropBeforeReturn: true,
		},
		{
			name: wasm.OpcodeVecF64x2SplatName, body: splat(wasm.OpcodeVecF64x2Splat),
			expected:             newOperationV128Splat(shapeF64x2),
			needDropBeforeReturn: true,
		},
		{
			name: wasm.OpcodeVecI8x16SwizzleName, body: vv2v(wasm.OpcodeVecI8x16Swizzle),
			expected:             newOperationV128Swizzle(),
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
			expected: newOperationV128Shuffle([]uint64{
				1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
			}),
			needDropBeforeReturn: true,
		},
		{
			name: wasm.OpcodeVecV128NotName, body: v2v(wasm.OpcodeVecV128Not),
			needDropBeforeReturn: true,
			expected:             newOperationV128Not(),
		},
		{
			name: wasm.OpcodeVecV128AndName, body: vv2v(wasm.OpcodeVecV128And),
			needDropBeforeReturn: true,
			expected:             newOperationV128And(),
		},
		{
			name: wasm.OpcodeVecV128AndNotName, body: vv2v(wasm.OpcodeVecV128AndNot),
			needDropBeforeReturn: true,
			expected:             newOperationV128AndNot(),
		},
		{
			name: wasm.OpcodeVecV128OrName, body: vv2v(wasm.OpcodeVecV128Or),
			needDropBeforeReturn: true,
			expected:             newOperationV128Or(),
		},
		{
			name: wasm.OpcodeVecV128XorName, body: vv2v(wasm.OpcodeVecV128Xor),
			needDropBeforeReturn: true,
			expected:             newOperationV128Xor(),
		},
		{
			name: wasm.OpcodeVecV128BitselectName, body: vvv2v(wasm.OpcodeVecV128Bitselect),
			needDropBeforeReturn: true,
			expected:             newOperationV128Bitselect(),
		},
		{
			name: wasm.OpcodeVecI8x16ShlName, body: vi2v(wasm.OpcodeVecI8x16Shl),
			needDropBeforeReturn: true,
			expected:             newOperationV128Shl(shapeI8x16),
		},
		{
			name: wasm.OpcodeVecI8x16ShrSName, body: vi2v(wasm.OpcodeVecI8x16ShrS),
			needDropBeforeReturn: true,
			expected:             newOperationV128Shr(shapeI8x16, true),
		},
		{
			name: wasm.OpcodeVecI8x16ShrUName, body: vi2v(wasm.OpcodeVecI8x16ShrU),
			needDropBeforeReturn: true,
			expected:             newOperationV128Shr(shapeI8x16, false),
		},
		{
			name: wasm.OpcodeVecI16x8ShlName, body: vi2v(wasm.OpcodeVecI16x8Shl),
			needDropBeforeReturn: true,
			expected:             newOperationV128Shl(shapeI16x8),
		},
		{
			name: wasm.OpcodeVecI16x8ShrSName, body: vi2v(wasm.OpcodeVecI16x8ShrS),
			needDropBeforeReturn: true,
			expected:             newOperationV128Shr(shapeI16x8, true),
		},
		{
			name: wasm.OpcodeVecI16x8ShrUName, body: vi2v(wasm.OpcodeVecI16x8ShrU),
			needDropBeforeReturn: true,
			expected:             newOperationV128Shr(shapeI16x8, false),
		},
		{
			name: wasm.OpcodeVecI32x4ShlName, body: vi2v(wasm.OpcodeVecI32x4Shl),
			needDropBeforeReturn: true,
			expected:             newOperationV128Shl(shapeI32x4),
		},
		{
			name: wasm.OpcodeVecI32x4ShrSName, body: vi2v(wasm.OpcodeVecI32x4ShrS),
			needDropBeforeReturn: true,
			expected:             newOperationV128Shr(shapeI32x4, true),
		},
		{
			name: wasm.OpcodeVecI32x4ShrUName, body: vi2v(wasm.OpcodeVecI32x4ShrU),
			needDropBeforeReturn: true,
			expected:             newOperationV128Shr(shapeI32x4, false),
		},
		{
			name: wasm.OpcodeVecI64x2ShlName, body: vi2v(wasm.OpcodeVecI64x2Shl),
			needDropBeforeReturn: true,
			expected:             newOperationV128Shl(shapeI64x2),
		},
		{
			name: wasm.OpcodeVecI64x2ShrSName, body: vi2v(wasm.OpcodeVecI64x2ShrS),
			needDropBeforeReturn: true,
			expected:             newOperationV128Shr(shapeI64x2, true),
		},
		{
			name: wasm.OpcodeVecI64x2ShrUName, body: vi2v(wasm.OpcodeVecI64x2ShrU),
			needDropBeforeReturn: true,
			expected:             newOperationV128Shr(shapeI64x2, false),
		},
		{
			name: wasm.OpcodeVecI8x16EqName, body: vv2v(wasm.OpcodeVecI8x16Eq),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI8x16Eq),
		},
		{
			name: wasm.OpcodeVecI8x16NeName, body: vv2v(wasm.OpcodeVecI8x16Ne),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI8x16Ne),
		},
		{
			name: wasm.OpcodeVecI8x16LtSName, body: vv2v(wasm.OpcodeVecI8x16LtS),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI8x16LtS),
		},
		{
			name: wasm.OpcodeVecI8x16LtUName, body: vv2v(wasm.OpcodeVecI8x16LtU),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI8x16LtU),
		},
		{
			name: wasm.OpcodeVecI8x16GtSName, body: vv2v(wasm.OpcodeVecI8x16GtS),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI8x16GtS),
		},
		{
			name: wasm.OpcodeVecI8x16GtUName, body: vv2v(wasm.OpcodeVecI8x16GtU),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI8x16GtU),
		},
		{
			name: wasm.OpcodeVecI8x16LeSName, body: vv2v(wasm.OpcodeVecI8x16LeS),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI8x16LeS),
		},
		{
			name: wasm.OpcodeVecI8x16LeUName, body: vv2v(wasm.OpcodeVecI8x16LeU),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI8x16LeU),
		},
		{
			name: wasm.OpcodeVecI8x16GeSName, body: vv2v(wasm.OpcodeVecI8x16GeS),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI8x16GeS),
		},
		{
			name: wasm.OpcodeVecI8x16GeUName, body: vv2v(wasm.OpcodeVecI8x16GeU),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI8x16GeU),
		},
		{
			name: wasm.OpcodeVecI16x8EqName, body: vv2v(wasm.OpcodeVecI16x8Eq),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI16x8Eq),
		},
		{
			name: wasm.OpcodeVecI16x8NeName, body: vv2v(wasm.OpcodeVecI16x8Ne),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI16x8Ne),
		},
		{
			name: wasm.OpcodeVecI16x8LtSName, body: vv2v(wasm.OpcodeVecI16x8LtS),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI16x8LtS),
		},
		{
			name: wasm.OpcodeVecI16x8LtUName, body: vv2v(wasm.OpcodeVecI16x8LtU),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI16x8LtU),
		},
		{
			name: wasm.OpcodeVecI16x8GtSName, body: vv2v(wasm.OpcodeVecI16x8GtS),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI16x8GtS),
		},
		{
			name: wasm.OpcodeVecI16x8GtUName, body: vv2v(wasm.OpcodeVecI16x8GtU),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI16x8GtU),
		},
		{
			name: wasm.OpcodeVecI16x8LeSName, body: vv2v(wasm.OpcodeVecI16x8LeS),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI16x8LeS),
		},
		{
			name: wasm.OpcodeVecI16x8LeUName, body: vv2v(wasm.OpcodeVecI16x8LeU),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI16x8LeU),
		},
		{
			name: wasm.OpcodeVecI16x8GeSName, body: vv2v(wasm.OpcodeVecI16x8GeS),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI16x8GeS),
		},
		{
			name: wasm.OpcodeVecI16x8GeUName, body: vv2v(wasm.OpcodeVecI16x8GeU),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI16x8GeU),
		},
		{
			name: wasm.OpcodeVecI32x4EqName, body: vv2v(wasm.OpcodeVecI32x4Eq),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI32x4Eq),
		},
		{
			name: wasm.OpcodeVecI32x4NeName, body: vv2v(wasm.OpcodeVecI32x4Ne),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI32x4Ne),
		},
		{
			name: wasm.OpcodeVecI32x4LtSName, body: vv2v(wasm.OpcodeVecI32x4LtS),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI32x4LtS),
		},
		{
			name: wasm.OpcodeVecI32x4LtUName, body: vv2v(wasm.OpcodeVecI32x4LtU),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI32x4LtU),
		},
		{
			name: wasm.OpcodeVecI32x4GtSName, body: vv2v(wasm.OpcodeVecI32x4GtS),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI32x4GtS),
		},
		{
			name: wasm.OpcodeVecI32x4GtUName, body: vv2v(wasm.OpcodeVecI32x4GtU),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI32x4GtU),
		},
		{
			name: wasm.OpcodeVecI32x4LeSName, body: vv2v(wasm.OpcodeVecI32x4LeS),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI32x4LeS),
		},
		{
			name: wasm.OpcodeVecI32x4LeUName, body: vv2v(wasm.OpcodeVecI32x4LeU),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI32x4LeU),
		},
		{
			name: wasm.OpcodeVecI32x4GeSName, body: vv2v(wasm.OpcodeVecI32x4GeS),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI32x4GeS),
		},
		{
			name: wasm.OpcodeVecI32x4GeUName, body: vv2v(wasm.OpcodeVecI32x4GeU),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI32x4GeU),
		},
		{
			name: wasm.OpcodeVecI64x2EqName, body: vv2v(wasm.OpcodeVecI64x2Eq),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI64x2Eq),
		},
		{
			name: wasm.OpcodeVecI64x2NeName, body: vv2v(wasm.OpcodeVecI64x2Ne),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI64x2Ne),
		},
		{
			name: wasm.OpcodeVecI64x2LtSName, body: vv2v(wasm.OpcodeVecI64x2LtS),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI64x2LtS),
		},
		{
			name: wasm.OpcodeVecI64x2GtSName, body: vv2v(wasm.OpcodeVecI64x2GtS),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI64x2GtS),
		},
		{
			name: wasm.OpcodeVecI64x2LeSName, body: vv2v(wasm.OpcodeVecI64x2LeS),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI64x2LeS),
		},
		{
			name: wasm.OpcodeVecI64x2GeSName, body: vv2v(wasm.OpcodeVecI64x2GeS),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeI64x2GeS),
		},
		{
			name: wasm.OpcodeVecF32x4EqName, body: vv2v(wasm.OpcodeVecF32x4Eq),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeF32x4Eq),
		},
		{
			name: wasm.OpcodeVecF32x4NeName, body: vv2v(wasm.OpcodeVecF32x4Ne),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeF32x4Ne),
		},
		{
			name: wasm.OpcodeVecF32x4LtName, body: vv2v(wasm.OpcodeVecF32x4Lt),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeF32x4Lt),
		},
		{
			name: wasm.OpcodeVecF32x4GtName, body: vv2v(wasm.OpcodeVecF32x4Gt),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeF32x4Gt),
		},
		{
			name: wasm.OpcodeVecF32x4LeName, body: vv2v(wasm.OpcodeVecF32x4Le),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeF32x4Le),
		},
		{
			name: wasm.OpcodeVecF32x4GeName, body: vv2v(wasm.OpcodeVecF32x4Ge),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeF32x4Ge),
		},
		{
			name: wasm.OpcodeVecF64x2EqName, body: vv2v(wasm.OpcodeVecF64x2Eq),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeF64x2Eq),
		},
		{
			name: wasm.OpcodeVecF64x2NeName, body: vv2v(wasm.OpcodeVecF64x2Ne),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeF64x2Ne),
		},
		{
			name: wasm.OpcodeVecF64x2LtName, body: vv2v(wasm.OpcodeVecF64x2Lt),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeF64x2Lt),
		},
		{
			name: wasm.OpcodeVecF64x2GtName, body: vv2v(wasm.OpcodeVecF64x2Gt),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeF64x2Gt),
		},
		{
			name: wasm.OpcodeVecF64x2LeName, body: vv2v(wasm.OpcodeVecF64x2Le),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeF64x2Le),
		},
		{
			name: wasm.OpcodeVecF64x2GeName, body: vv2v(wasm.OpcodeVecF64x2Ge),
			needDropBeforeReturn: true,
			expected:             newOperationV128Cmp(v128CmpTypeF64x2Ge),
		},
		{
			name: wasm.OpcodeVecI8x16AllTrueName, body: v2v(wasm.OpcodeVecI8x16AllTrue),
			needDropBeforeReturn: true,
			expected:             newOperationV128AllTrue(shapeI8x16),
		},
		{
			name: wasm.OpcodeVecI16x8AllTrueName, body: v2v(wasm.OpcodeVecI16x8AllTrue),
			needDropBeforeReturn: true,
			expected:             newOperationV128AllTrue(shapeI16x8),
		},
		{
			name: wasm.OpcodeVecI32x4AllTrueName, body: v2v(wasm.OpcodeVecI32x4AllTrue),
			needDropBeforeReturn: true,
			expected:             newOperationV128AllTrue(shapeI32x4),
		},
		{
			name: wasm.OpcodeVecI64x2AllTrueName, body: v2v(wasm.OpcodeVecI64x2AllTrue),
			needDropBeforeReturn: true,
			expected:             newOperationV128AllTrue(shapeI64x2),
		},
		{
			name: wasm.OpcodeVecI8x16BitMaskName, body: v2v(wasm.OpcodeVecI8x16BitMask),
			needDropBeforeReturn: true, expected: newOperationV128BitMask(shapeI8x16),
		},
		{
			name: wasm.OpcodeVecI16x8BitMaskName, body: v2v(wasm.OpcodeVecI16x8BitMask),
			needDropBeforeReturn: true, expected: newOperationV128BitMask(shapeI16x8),
		},
		{
			name: wasm.OpcodeVecI32x4BitMaskName, body: v2v(wasm.OpcodeVecI32x4BitMask),
			needDropBeforeReturn: true, expected: newOperationV128BitMask(shapeI32x4),
		},
		{
			name: wasm.OpcodeVecI64x2BitMaskName, body: v2v(wasm.OpcodeVecI64x2BitMask),
			needDropBeforeReturn: true, expected: newOperationV128BitMask(shapeI64x2),
		},
		{
			name: wasm.OpcodeVecV128AnyTrueName, body: v2v(wasm.OpcodeVecV128AnyTrue),
			needDropBeforeReturn: true,
			expected:             newOperationV128AnyTrue(),
		},
		{
			name: wasm.OpcodeVecI8x16AddName, body: vv2v(wasm.OpcodeVecI8x16Add),
			needDropBeforeReturn: true,
			expected:             newOperationV128Add(shapeI8x16),
		},
		{
			name: wasm.OpcodeVecI8x16AddSatSName, body: vv2v(wasm.OpcodeVecI8x16AddSatS),
			needDropBeforeReturn: true,
			expected:             newOperationV128AddSat(shapeI8x16, true),
		},
		{
			name: wasm.OpcodeVecI8x16AddSatUName, body: vv2v(wasm.OpcodeVecI8x16AddSatU),
			needDropBeforeReturn: true,
			expected:             newOperationV128AddSat(shapeI8x16, false),
		},
		{
			name: wasm.OpcodeVecI8x16SubName, body: vv2v(wasm.OpcodeVecI8x16Sub),
			needDropBeforeReturn: true,
			expected:             newOperationV128Sub(shapeI8x16),
		},
		{
			name: wasm.OpcodeVecI8x16SubSatSName, body: vv2v(wasm.OpcodeVecI8x16SubSatS),
			needDropBeforeReturn: true,
			expected:             newOperationV128SubSat(shapeI8x16, true),
		},
		{
			name: wasm.OpcodeVecI8x16SubSatUName, body: vv2v(wasm.OpcodeVecI8x16SubSatU),
			needDropBeforeReturn: true,
			expected:             newOperationV128SubSat(shapeI8x16, false),
		},
		{
			name: wasm.OpcodeVecI16x8AddName, body: vv2v(wasm.OpcodeVecI16x8Add),
			needDropBeforeReturn: true,
			expected:             newOperationV128Add(shapeI16x8),
		},
		{
			name: wasm.OpcodeVecI16x8AddSatSName, body: vv2v(wasm.OpcodeVecI16x8AddSatS),
			needDropBeforeReturn: true,
			expected:             newOperationV128AddSat(shapeI16x8, true),
		},
		{
			name: wasm.OpcodeVecI16x8AddSatUName, body: vv2v(wasm.OpcodeVecI16x8AddSatU),
			needDropBeforeReturn: true,
			expected:             newOperationV128AddSat(shapeI16x8, false),
		},
		{
			name: wasm.OpcodeVecI16x8SubName, body: vv2v(wasm.OpcodeVecI16x8Sub),
			needDropBeforeReturn: true,
			expected:             newOperationV128Sub(shapeI16x8),
		},
		{
			name: wasm.OpcodeVecI16x8SubSatSName, body: vv2v(wasm.OpcodeVecI16x8SubSatS),
			needDropBeforeReturn: true,
			expected:             newOperationV128SubSat(shapeI16x8, true),
		},
		{
			name: wasm.OpcodeVecI16x8SubSatUName, body: vv2v(wasm.OpcodeVecI16x8SubSatU),
			needDropBeforeReturn: true,
			expected:             newOperationV128SubSat(shapeI16x8, false),
		},
		{
			name: wasm.OpcodeVecI16x8MulName, body: vv2v(wasm.OpcodeVecI16x8Mul),
			needDropBeforeReturn: true,
			expected:             newOperationV128Mul(shapeI16x8),
		},
		{
			name: wasm.OpcodeVecI32x4AddName, body: vv2v(wasm.OpcodeVecI32x4Add),
			needDropBeforeReturn: true,
			expected:             newOperationV128Add(shapeI32x4),
		},
		{
			name: wasm.OpcodeVecI32x4SubName, body: vv2v(wasm.OpcodeVecI32x4Sub),
			needDropBeforeReturn: true,
			expected:             newOperationV128Sub(shapeI32x4),
		},
		{
			name: wasm.OpcodeVecI32x4MulName, body: vv2v(wasm.OpcodeVecI32x4Mul),
			needDropBeforeReturn: true,
			expected:             newOperationV128Mul(shapeI32x4),
		},
		{
			name: wasm.OpcodeVecI64x2AddName, body: vv2v(wasm.OpcodeVecI64x2Add),
			needDropBeforeReturn: true,
			expected:             newOperationV128Add(shapeI64x2),
		},
		{
			name: wasm.OpcodeVecI64x2SubName, body: vv2v(wasm.OpcodeVecI64x2Sub),
			needDropBeforeReturn: true,
			expected:             newOperationV128Sub(shapeI64x2),
		},
		{
			name: wasm.OpcodeVecI64x2MulName, body: vv2v(wasm.OpcodeVecI64x2Mul),
			needDropBeforeReturn: true,
			expected:             newOperationV128Mul(shapeI64x2),
		},
		{
			name: wasm.OpcodeVecF32x4AddName, body: vv2v(wasm.OpcodeVecF32x4Add),
			needDropBeforeReturn: true,
			expected:             newOperationV128Add(shapeF32x4),
		},
		{
			name: wasm.OpcodeVecF32x4SubName, body: vv2v(wasm.OpcodeVecF32x4Sub),
			needDropBeforeReturn: true,
			expected:             newOperationV128Sub(shapeF32x4),
		},
		{
			name: wasm.OpcodeVecF32x4MulName, body: vv2v(wasm.OpcodeVecF32x4Mul),
			needDropBeforeReturn: true,
			expected:             newOperationV128Mul(shapeF32x4),
		},
		{
			name: wasm.OpcodeVecF32x4DivName, body: vv2v(wasm.OpcodeVecF32x4Div),
			needDropBeforeReturn: true,
			expected:             newOperationV128Div(shapeF32x4),
		},
		{
			name: wasm.OpcodeVecF64x2AddName, body: vv2v(wasm.OpcodeVecF64x2Add),
			needDropBeforeReturn: true,
			expected:             newOperationV128Add(shapeF64x2),
		},
		{
			name: wasm.OpcodeVecF64x2SubName, body: vv2v(wasm.OpcodeVecF64x2Sub),
			needDropBeforeReturn: true,
			expected:             newOperationV128Sub(shapeF64x2),
		},
		{
			name: wasm.OpcodeVecF64x2MulName, body: vv2v(wasm.OpcodeVecF64x2Mul),
			needDropBeforeReturn: true,
			expected:             newOperationV128Mul(shapeF64x2),
		},
		{
			name: wasm.OpcodeVecF64x2DivName, body: vv2v(wasm.OpcodeVecF64x2Div),
			needDropBeforeReturn: true,
			expected:             newOperationV128Div(shapeF64x2),
		},
		{
			name: wasm.OpcodeVecI8x16MinSName, body: vv2v(wasm.OpcodeVecI8x16MinS),
			needDropBeforeReturn: true,
			expected:             newOperationV128Min(shapeI8x16, true),
		},
		{
			name: wasm.OpcodeVecI8x16MinUName, body: vv2v(wasm.OpcodeVecI8x16MinU),
			needDropBeforeReturn: true,
			expected:             newOperationV128Min(shapeI8x16, false),
		},
		{
			name: wasm.OpcodeVecI8x16MaxSName, body: vv2v(wasm.OpcodeVecI8x16MaxS),
			needDropBeforeReturn: true,
			expected:             newOperationV128Max(shapeI8x16, true),
		},
		{
			name: wasm.OpcodeVecI8x16MaxUName, body: vv2v(wasm.OpcodeVecI8x16MaxU),
			needDropBeforeReturn: true,
			expected:             newOperationV128Max(shapeI8x16, false),
		},
		{
			name: wasm.OpcodeVecI8x16AvgrUName, body: vv2v(wasm.OpcodeVecI8x16AvgrU),
			needDropBeforeReturn: true,
			expected:             newOperationV128AvgrU(shapeI8x16),
		},
		{
			name: wasm.OpcodeVecI16x8MinSName, body: vv2v(wasm.OpcodeVecI16x8MinS),
			needDropBeforeReturn: true,
			expected:             newOperationV128Min(shapeI16x8, true),
		},
		{
			name: wasm.OpcodeVecI16x8MinUName, body: vv2v(wasm.OpcodeVecI16x8MinU),
			needDropBeforeReturn: true,
			expected:             newOperationV128Min(shapeI16x8, false),
		},
		{
			name: wasm.OpcodeVecI16x8MaxSName, body: vv2v(wasm.OpcodeVecI16x8MaxS),
			needDropBeforeReturn: true,
			expected:             newOperationV128Max(shapeI16x8, true),
		},
		{
			name: wasm.OpcodeVecI16x8MaxUName, body: vv2v(wasm.OpcodeVecI16x8MaxU),
			needDropBeforeReturn: true,
			expected:             newOperationV128Max(shapeI16x8, false),
		},
		{
			name: wasm.OpcodeVecI16x8AvgrUName, body: vv2v(wasm.OpcodeVecI16x8AvgrU),
			needDropBeforeReturn: true,
			expected:             newOperationV128AvgrU(shapeI16x8),
		},
		{
			name: wasm.OpcodeVecI32x4MinSName, body: vv2v(wasm.OpcodeVecI32x4MinS),
			needDropBeforeReturn: true,
			expected:             newOperationV128Min(shapeI32x4, true),
		},
		{
			name: wasm.OpcodeVecI32x4MinUName, body: vv2v(wasm.OpcodeVecI32x4MinU),
			needDropBeforeReturn: true,
			expected:             newOperationV128Min(shapeI32x4, false),
		},
		{
			name: wasm.OpcodeVecI32x4MaxSName, body: vv2v(wasm.OpcodeVecI32x4MaxS),
			needDropBeforeReturn: true,
			expected:             newOperationV128Max(shapeI32x4, true),
		},
		{
			name: wasm.OpcodeVecI32x4MaxUName, body: vv2v(wasm.OpcodeVecI32x4MaxU),
			needDropBeforeReturn: true,
			expected:             newOperationV128Max(shapeI32x4, false),
		},
		{
			name: wasm.OpcodeVecF32x4MinName, body: vv2v(wasm.OpcodeVecF32x4Min),
			needDropBeforeReturn: true,
			expected:             newOperationV128Min(shapeF32x4, false),
		},
		{
			name: wasm.OpcodeVecF32x4MaxName, body: vv2v(wasm.OpcodeVecF32x4Max),
			needDropBeforeReturn: true,
			expected:             newOperationV128Max(shapeF32x4, false),
		},
		{
			name: wasm.OpcodeVecF64x2MinName, body: vv2v(wasm.OpcodeVecF64x2Min),
			needDropBeforeReturn: true,
			expected:             newOperationV128Min(shapeF64x2, false),
		},
		{
			name: wasm.OpcodeVecF64x2MaxName, body: vv2v(wasm.OpcodeVecF64x2Max),
			needDropBeforeReturn: true,
			expected:             newOperationV128Max(shapeF64x2, false),
		},
		{
			name: wasm.OpcodeVecI8x16AbsName, body: v2v(wasm.OpcodeVecI8x16Abs),
			needDropBeforeReturn: true,
			expected:             newOperationV128Abs(shapeI8x16),
		},
		{
			name: wasm.OpcodeVecI8x16PopcntName, body: v2v(wasm.OpcodeVecI8x16Popcnt),
			needDropBeforeReturn: true,
			expected:             newOperationV128Popcnt(shapeI8x16),
		},
		{
			name: wasm.OpcodeVecI16x8AbsName, body: v2v(wasm.OpcodeVecI16x8Abs),
			needDropBeforeReturn: true,
			expected:             newOperationV128Abs(shapeI16x8),
		},
		{
			name: wasm.OpcodeVecI32x4AbsName, body: v2v(wasm.OpcodeVecI32x4Abs),
			needDropBeforeReturn: true,
			expected:             newOperationV128Abs(shapeI32x4),
		},
		{
			name: wasm.OpcodeVecI64x2AbsName, body: v2v(wasm.OpcodeVecI64x2Abs),
			needDropBeforeReturn: true,
			expected:             newOperationV128Abs(shapeI64x2),
		},
		{
			name: wasm.OpcodeVecF32x4AbsName, body: v2v(wasm.OpcodeVecF32x4Abs),
			needDropBeforeReturn: true,
			expected:             newOperationV128Abs(shapeF32x4),
		},
		{
			name: wasm.OpcodeVecF64x2AbsName, body: v2v(wasm.OpcodeVecF64x2Abs),
			needDropBeforeReturn: true,
			expected:             newOperationV128Abs(shapeF64x2),
		},
		{
			name: wasm.OpcodeVecF32x4CeilName, body: v2v(wasm.OpcodeVecF32x4Ceil),
			needDropBeforeReturn: true,
			expected:             newOperationV128Ceil(shapeF32x4),
		},
		{
			name: wasm.OpcodeVecF32x4FloorName, body: v2v(wasm.OpcodeVecF32x4Floor),
			needDropBeforeReturn: true,
			expected:             newOperationV128Floor(shapeF32x4),
		},
		{
			name: wasm.OpcodeVecF32x4TruncName, body: v2v(wasm.OpcodeVecF32x4Trunc),
			needDropBeforeReturn: true,
			expected:             newOperationV128Trunc(shapeF32x4),
		},
		{
			name: wasm.OpcodeVecF32x4NearestName, body: v2v(wasm.OpcodeVecF32x4Nearest),
			needDropBeforeReturn: true,
			expected:             newOperationV128Nearest(shapeF32x4),
		},
		{
			name: wasm.OpcodeVecF64x2CeilName, body: v2v(wasm.OpcodeVecF64x2Ceil),
			needDropBeforeReturn: true,
			expected:             newOperationV128Ceil(shapeF64x2),
		},
		{
			name: wasm.OpcodeVecF64x2FloorName, body: v2v(wasm.OpcodeVecF64x2Floor),
			needDropBeforeReturn: true,
			expected:             newOperationV128Floor(shapeF64x2),
		},
		{
			name: wasm.OpcodeVecF64x2TruncName, body: v2v(wasm.OpcodeVecF64x2Trunc),
			needDropBeforeReturn: true,
			expected:             newOperationV128Trunc(shapeF64x2),
		},
		{
			name: wasm.OpcodeVecF64x2NearestName, body: v2v(wasm.OpcodeVecF64x2Nearest),
			needDropBeforeReturn: true,
			expected:             newOperationV128Nearest(shapeF64x2),
		},
		{
			name: wasm.OpcodeVecF32x4PminName, body: vv2v(wasm.OpcodeVecF32x4Pmin),
			needDropBeforeReturn: true,
			expected:             newOperationV128Pmin(shapeF32x4),
		},
		{
			name: wasm.OpcodeVecF32x4PmaxName, body: vv2v(wasm.OpcodeVecF32x4Pmax),
			needDropBeforeReturn: true,
			expected:             newOperationV128Pmax(shapeF32x4),
		},
		{
			name: wasm.OpcodeVecF64x2PminName, body: vv2v(wasm.OpcodeVecF64x2Pmin),
			needDropBeforeReturn: true,
			expected:             newOperationV128Pmin(shapeF64x2),
		},
		{
			name: wasm.OpcodeVecF64x2PmaxName, body: vv2v(wasm.OpcodeVecF64x2Pmax),
			needDropBeforeReturn: true,
			expected:             newOperationV128Pmax(shapeF64x2),
		},
		{
			name: wasm.OpcodeVecI16x8Q15mulrSatSName, body: vv2v(wasm.OpcodeVecI16x8Q15mulrSatS),
			needDropBeforeReturn: true,
			expected:             newOperationV128Q15mulrSatS(),
		},
		{
			name: wasm.OpcodeVecI16x8ExtMulLowI8x16SName, body: vv2v(wasm.OpcodeVecI16x8ExtMulLowI8x16S),
			needDropBeforeReturn: true,
			expected:             newOperationV128ExtMul(shapeI8x16, true, true),
		},
		{
			name: wasm.OpcodeVecI16x8ExtMulHighI8x16SName, body: vv2v(wasm.OpcodeVecI16x8ExtMulHighI8x16S),
			needDropBeforeReturn: true,
			expected:             newOperationV128ExtMul(shapeI8x16, true, false),
		},
		{
			name: wasm.OpcodeVecI16x8ExtMulLowI8x16UName, body: vv2v(wasm.OpcodeVecI16x8ExtMulLowI8x16U),
			needDropBeforeReturn: true,
			expected:             newOperationV128ExtMul(shapeI8x16, false, true),
		},
		{
			name: wasm.OpcodeVecI16x8ExtMulHighI8x16UName, body: vv2v(wasm.OpcodeVecI16x8ExtMulHighI8x16U),
			needDropBeforeReturn: true,
			expected:             newOperationV128ExtMul(shapeI8x16, false, false),
		},
		{
			name: wasm.OpcodeVecI32x4ExtMulLowI16x8SName, body: vv2v(wasm.OpcodeVecI32x4ExtMulLowI16x8S),
			needDropBeforeReturn: true,
			expected:             newOperationV128ExtMul(shapeI16x8, true, true),
		},
		{
			name: wasm.OpcodeVecI32x4ExtMulHighI16x8SName, body: vv2v(wasm.OpcodeVecI32x4ExtMulHighI16x8S),
			needDropBeforeReturn: true,
			expected:             newOperationV128ExtMul(shapeI16x8, true, false),
		},
		{
			name: wasm.OpcodeVecI32x4ExtMulLowI16x8UName, body: vv2v(wasm.OpcodeVecI32x4ExtMulLowI16x8U),
			needDropBeforeReturn: true,
			expected:             newOperationV128ExtMul(shapeI16x8, false, true),
		},
		{
			name: wasm.OpcodeVecI32x4ExtMulHighI16x8UName, body: vv2v(wasm.OpcodeVecI32x4ExtMulHighI16x8U),
			needDropBeforeReturn: true,
			expected:             newOperationV128ExtMul(shapeI16x8, false, false),
		},
		{
			name: wasm.OpcodeVecI64x2ExtMulLowI32x4SName, body: vv2v(wasm.OpcodeVecI64x2ExtMulLowI32x4S),
			needDropBeforeReturn: true,
			expected:             newOperationV128ExtMul(shapeI32x4, true, true),
		},
		{
			name: wasm.OpcodeVecI64x2ExtMulHighI32x4SName, body: vv2v(wasm.OpcodeVecI64x2ExtMulHighI32x4S),
			needDropBeforeReturn: true,
			expected:             newOperationV128ExtMul(shapeI32x4, true, false),
		},
		{
			name: wasm.OpcodeVecI64x2ExtMulLowI32x4UName, body: vv2v(wasm.OpcodeVecI64x2ExtMulLowI32x4U),
			needDropBeforeReturn: true,
			expected:             newOperationV128ExtMul(shapeI32x4, false, true),
		},
		{
			name: wasm.OpcodeVecI64x2ExtMulHighI32x4UName, body: vv2v(wasm.OpcodeVecI64x2ExtMulHighI32x4U),
			needDropBeforeReturn: true,
			expected:             newOperationV128ExtMul(shapeI32x4, false, false),
		},
		{
			name: wasm.OpcodeVecI16x8ExtendLowI8x16SName, body: v2v(wasm.OpcodeVecI16x8ExtendLowI8x16S),
			needDropBeforeReturn: true,
			expected:             newOperationV128Extend(shapeI8x16, true, true),
		},
		{
			name: wasm.OpcodeVecI16x8ExtendHighI8x16SName, body: v2v(wasm.OpcodeVecI16x8ExtendHighI8x16S),
			needDropBeforeReturn: true,
			expected:             newOperationV128Extend(shapeI8x16, true, false),
		},
		{
			name: wasm.OpcodeVecI16x8ExtendLowI8x16UName, body: v2v(wasm.OpcodeVecI16x8ExtendLowI8x16U),
			needDropBeforeReturn: true,
			expected:             newOperationV128Extend(shapeI8x16, false, true),
		},
		{
			name: wasm.OpcodeVecI16x8ExtendHighI8x16UName, body: v2v(wasm.OpcodeVecI16x8ExtendHighI8x16U),
			needDropBeforeReturn: true,
			expected:             newOperationV128Extend(shapeI8x16, false, false),
		},
		{
			name: wasm.OpcodeVecI32x4ExtendLowI16x8SName, body: v2v(wasm.OpcodeVecI32x4ExtendLowI16x8S),
			needDropBeforeReturn: true,
			expected:             newOperationV128Extend(shapeI16x8, true, true),
		},
		{
			name: wasm.OpcodeVecI32x4ExtendHighI16x8SName, body: v2v(wasm.OpcodeVecI32x4ExtendHighI16x8S),
			needDropBeforeReturn: true,
			expected:             newOperationV128Extend(shapeI16x8, true, false),
		},
		{
			name: wasm.OpcodeVecI32x4ExtendLowI16x8UName, body: v2v(wasm.OpcodeVecI32x4ExtendLowI16x8U),
			needDropBeforeReturn: true,
			expected:             newOperationV128Extend(shapeI16x8, false, true),
		},
		{
			name: wasm.OpcodeVecI32x4ExtendHighI16x8UName, body: v2v(wasm.OpcodeVecI32x4ExtendHighI16x8U),
			needDropBeforeReturn: true,
			expected:             newOperationV128Extend(shapeI16x8, false, false),
		},
		{
			name: wasm.OpcodeVecI64x2ExtendLowI32x4SName, body: v2v(wasm.OpcodeVecI64x2ExtendLowI32x4S),
			needDropBeforeReturn: true,
			expected:             newOperationV128Extend(shapeI32x4, true, true),
		},
		{
			name: wasm.OpcodeVecI64x2ExtendHighI32x4SName, body: v2v(wasm.OpcodeVecI64x2ExtendHighI32x4S),
			needDropBeforeReturn: true,
			expected:             newOperationV128Extend(shapeI32x4, true, false),
		},
		{
			name: wasm.OpcodeVecI64x2ExtendLowI32x4UName, body: v2v(wasm.OpcodeVecI64x2ExtendLowI32x4U),
			needDropBeforeReturn: true,
			expected:             newOperationV128Extend(shapeI32x4, false, true),
		},
		{
			name: wasm.OpcodeVecI64x2ExtendHighI32x4UName, body: v2v(wasm.OpcodeVecI64x2ExtendHighI32x4U),
			needDropBeforeReturn: true,
			expected:             newOperationV128Extend(shapeI32x4, false, false),
		},

		{
			name: wasm.OpcodeVecI16x8ExtaddPairwiseI8x16SName, body: v2v(wasm.OpcodeVecI16x8ExtaddPairwiseI8x16S),
			needDropBeforeReturn: true,
			expected:             newOperationV128ExtAddPairwise(shapeI8x16, true),
		},
		{
			name: wasm.OpcodeVecI16x8ExtaddPairwiseI8x16UName, body: v2v(wasm.OpcodeVecI16x8ExtaddPairwiseI8x16U),
			needDropBeforeReturn: true,
			expected:             newOperationV128ExtAddPairwise(shapeI8x16, false),
		},
		{
			name: wasm.OpcodeVecI32x4ExtaddPairwiseI16x8SName, body: v2v(wasm.OpcodeVecI32x4ExtaddPairwiseI16x8S),
			needDropBeforeReturn: true,
			expected:             newOperationV128ExtAddPairwise(shapeI16x8, true),
		},
		{
			name: wasm.OpcodeVecI32x4ExtaddPairwiseI16x8UName, body: v2v(wasm.OpcodeVecI32x4ExtaddPairwiseI16x8U),
			needDropBeforeReturn: true,
			expected:             newOperationV128ExtAddPairwise(shapeI16x8, false),
		},
		{
			name: wasm.OpcodeVecF64x2PromoteLowF32x4ZeroName, body: v2v(wasm.OpcodeVecF64x2PromoteLowF32x4Zero),
			needDropBeforeReturn: true,
			expected:             newOperationV128FloatPromote(),
		},
		{
			name: wasm.OpcodeVecF32x4DemoteF64x2ZeroName, body: v2v(wasm.OpcodeVecF32x4DemoteF64x2Zero),
			needDropBeforeReturn: true,
			expected:             newOperationV128FloatDemote(),
		},
		{
			name: wasm.OpcodeVecF32x4ConvertI32x4SName, body: v2v(wasm.OpcodeVecF32x4ConvertI32x4S),
			needDropBeforeReturn: true,
			expected:             newOperationV128FConvertFromI(shapeF32x4, true),
		},
		{
			name: wasm.OpcodeVecF32x4ConvertI32x4UName, body: v2v(wasm.OpcodeVecF32x4ConvertI32x4U),
			needDropBeforeReturn: true,
			expected:             newOperationV128FConvertFromI(shapeF32x4, false),
		},
		{
			name: wasm.OpcodeVecF64x2ConvertLowI32x4SName, body: v2v(wasm.OpcodeVecF64x2ConvertLowI32x4S),
			needDropBeforeReturn: true,
			expected:             newOperationV128FConvertFromI(shapeF64x2, true),
		},
		{
			name: wasm.OpcodeVecF64x2ConvertLowI32x4UName, body: v2v(wasm.OpcodeVecF64x2ConvertLowI32x4U),
			needDropBeforeReturn: true,
			expected:             newOperationV128FConvertFromI(shapeF64x2, false),
		},
		{
			name: wasm.OpcodeVecI32x4DotI16x8SName, body: vv2v(wasm.OpcodeVecI32x4DotI16x8S),
			needDropBeforeReturn: true,
			expected:             newOperationV128Dot(),
		},
		{
			name: wasm.OpcodeVecI8x16NarrowI16x8SName, body: vv2v(wasm.OpcodeVecI8x16NarrowI16x8S),
			needDropBeforeReturn: true,
			expected:             newOperationV128Narrow(shapeI16x8, true),
		},
		{
			name: wasm.OpcodeVecI8x16NarrowI16x8UName, body: vv2v(wasm.OpcodeVecI8x16NarrowI16x8U),
			needDropBeforeReturn: true,
			expected:             newOperationV128Narrow(shapeI16x8, false),
		},
		{
			name: wasm.OpcodeVecI16x8NarrowI32x4SName, body: vv2v(wasm.OpcodeVecI16x8NarrowI32x4S),
			needDropBeforeReturn: true,
			expected:             newOperationV128Narrow(shapeI32x4, true),
		},
		{
			name: wasm.OpcodeVecI16x8NarrowI32x4UName, body: vv2v(wasm.OpcodeVecI16x8NarrowI32x4U),
			needDropBeforeReturn: true,
			expected:             newOperationV128Narrow(shapeI32x4, false),
		},
		{
			name: wasm.OpcodeVecI32x4TruncSatF32x4SName, body: v2v(wasm.OpcodeVecI32x4TruncSatF32x4S),
			needDropBeforeReturn: true,
			expected:             newOperationV128ITruncSatFromF(shapeF32x4, true),
		},
		{
			name: wasm.OpcodeVecI32x4TruncSatF32x4UName, body: v2v(wasm.OpcodeVecI32x4TruncSatF32x4U),
			needDropBeforeReturn: true,
			expected:             newOperationV128ITruncSatFromF(shapeF32x4, false),
		},
		{
			name: wasm.OpcodeVecI32x4TruncSatF64x2SZeroName, body: v2v(wasm.OpcodeVecI32x4TruncSatF64x2SZero),
			needDropBeforeReturn: true,
			expected:             newOperationV128ITruncSatFromF(shapeF64x2, true),
		},
		{
			name: wasm.OpcodeVecI32x4TruncSatF64x2UZeroName, body: v2v(wasm.OpcodeVecI32x4TruncSatF64x2UZero),
			needDropBeforeReturn: true,
			expected:             newOperationV128ITruncSatFromF(shapeF64x2, false),
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
			c, err := newCompiler(api.CoreFeaturesV2, 0, module, false)
			require.NoError(t, err)

			res, err := c.Next()
			require.NoError(t, err)

			var actual unionOperation
			if tc.needDropBeforeReturn {
				// If the drop operation is inserted, the target op exits at -3
				// as the operations looks like: [... target, drop, br(to return)].
				actual = res.Operations[len(res.Operations)-3]
			} else {
				// If the drop operation is not inserted, the target op exits at -2
				// as the operations looks like: [... target, br(to return)].
				actual = res.Operations[len(res.Operations)-2]
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
		expected []unionOperation
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
			expected: []unionOperation{newOperationBr(newLabel(labelKindReturn, 0))},
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
			expected: []unionOperation{newOperationBr(newLabel(labelKindReturn, 0))},
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
			expected: []unionOperation{newOperationBr(newLabel(labelKindReturn, 0))},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			c, err := newCompiler(api.CoreFeaturesV2, 0, tc.mod, false)
			require.NoError(t, err)

			actual, err := c.Next()
			require.NoError(t, err)
			require.Equal(t, tc.expected, actual.Operations)
		})
	}
}

// TestCompile_drop_vectors ensures that wasm.OpcodeDrop on vector values is correctly compiled,
// which is not covered by spectest.
func TestCompile_drop_vectors(t *testing.T) {
	tests := []struct {
		name     string
		mod      *wasm.Module
		expected []unionOperation
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
			expected: []unionOperation{
				newOperationV128Const(0x1, 0x2),
				// inclusiveRange is the range in uint64 representation, so dropping a vector value on top
				// should be translated as drop [0..1] inclusively.
				newOperationDrop(inclusiveRange{Start: 0, End: 1}),
				newOperationBr(newLabel(labelKindReturn, 0)),
			},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			c, err := newCompiler(api.CoreFeaturesV2, 0, tc.mod, false)
			require.NoError(t, err)

			actual, err := c.Next()
			require.NoError(t, err)
			require.Equal(t, tc.expected, actual.Operations)
		})
	}
}

func TestCompile_select_vectors(t *testing.T) {
	tests := []struct {
		name     string
		mod      *wasm.Module
		expected []unionOperation
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
			expected: []unionOperation{
				newOperationV128Const(0x1, 0x2),
				newOperationV128Const(0x3, 0x4),
				newOperationConstI32(0),
				newOperationSelect(true),
				newOperationDrop(inclusiveRange{Start: 0, End: 1}),
				newOperationBr(newLabel(labelKindReturn, 0)),
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
			expected: []unionOperation{
				newOperationV128Const(0x1, 0x2),
				newOperationV128Const(0x3, 0x4),
				newOperationConstI32(0),
				newOperationSelect(true),
				newOperationDrop(inclusiveRange{Start: 0, End: 1}),
				newOperationBr(newLabel(labelKindReturn, 0)),
			},
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			c, err := newCompiler(api.CoreFeaturesV2, 0, tc.mod, false)
			require.NoError(t, err)

			actual, err := c.Next()
			require.NoError(t, err)
			require.Equal(t, tc.expected, actual.Operations)
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
		expLocalIndexToStackHeightInUint64 []int
	}{
		{
			name: "no function local, args>results",
			sig: &wasm.FunctionType{
				Params:            []wasm.ValueType{i32, wf32},
				Results:           []wasm.ValueType{i32},
				ParamNumInUint64:  2,
				ResultNumInUint64: 1,
			},
			expLocalIndexToStackHeightInUint64: []int{0, 1},
		},
		{
			name: "no function local, args=results",
			sig: &wasm.FunctionType{
				Params:            []wasm.ValueType{i32},
				Results:           []wasm.ValueType{i32},
				ParamNumInUint64:  1,
				ResultNumInUint64: 1,
			},
			expLocalIndexToStackHeightInUint64: []int{0},
		},
		{
			name: "no function local, args>results, with vector",
			sig: &wasm.FunctionType{
				Params:            []wasm.ValueType{i32, v128, wf32},
				Results:           []wasm.ValueType{i32},
				ParamNumInUint64:  4,
				ResultNumInUint64: 1,
			},
			expLocalIndexToStackHeightInUint64: []int{0, 1, 3},
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
			expLocalIndexToStackHeightInUint64: nil,
		},
		{
			name: "no function local, args<results",
			sig: &wasm.FunctionType{
				Params:            []wasm.ValueType{i32},
				Results:           []wasm.ValueType{i32, wf32},
				ParamNumInUint64:  1,
				ResultNumInUint64: 2,
			},
			callFrameStackSizeInUint64:         4,
			expLocalIndexToStackHeightInUint64: []int{0},
		},
		{
			name: "no function local, args<results, with vector",
			sig: &wasm.FunctionType{
				Params:            []wasm.ValueType{i32},
				Results:           []wasm.ValueType{i32, v128, wf32},
				ParamNumInUint64:  1,
				ResultNumInUint64: 4,
			},
			callFrameStackSizeInUint64:         4,
			expLocalIndexToStackHeightInUint64: []int{0},
		},

		// With function locals
		{
			name: "function locals, args>results",
			sig: &wasm.FunctionType{
				Params:            []wasm.ValueType{i32, wf32},
				Results:           []wasm.ValueType{i32},
				ParamNumInUint64:  2,
				ResultNumInUint64: 1,
			},
			functionLocalTypes:         []wasm.ValueType{wf64},
			callFrameStackSizeInUint64: 4,
			// [i32, f32, callframe.0, callframe.1, callframe.2, callframe.3, f64]
			expLocalIndexToStackHeightInUint64: []int{
				0,
				1,
				// Function local comes after call frame.
				6,
			},
		},
		{
			name: "function locals, args>results, with vector",
			sig: &wasm.FunctionType{
				Params:            []wasm.ValueType{i32, v128, wf32},
				Results:           []wasm.ValueType{i32},
				ParamNumInUint64:  4,
				ResultNumInUint64: 1,
			},
			functionLocalTypes:         []wasm.ValueType{v128, v128},
			callFrameStackSizeInUint64: 4,
			// [i32, v128.lo, v128.hi, f32, callframe.0, callframe.1, callframe.2, callframe.3, v128.lo, v128.hi, v128.lo, v128.hi]
			expLocalIndexToStackHeightInUint64: []int{
				0,
				1,
				3,
				// Function local comes after call frame.
				8,
				10,
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
			functionLocalTypes:         []wasm.ValueType{wf64},
			callFrameStackSizeInUint64: 4,
			// [i32, _, _, _, callframe.0, callframe.1, callframe.2, callframe.3, f64]
			expLocalIndexToStackHeightInUint64: []int{0, 8},
		},
		{
			name: "function locals, args<results with vector",
			sig: &wasm.FunctionType{
				Params:            []wasm.ValueType{v128, wf64},
				Results:           []wasm.ValueType{v128, i32, i32, v128},
				ParamNumInUint64:  3,
				ResultNumInUint64: 6,
			},
			functionLocalTypes:         []wasm.ValueType{wf64},
			callFrameStackSizeInUint64: 4,
			// [v128.lo, v128.hi, f64, _, _, _, callframe.0, callframe.1, callframe.2, callframe.3, f64]
			expLocalIndexToStackHeightInUint64: []int{0, 2, 10},
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
			c, err := newCompiler(api.CoreFeaturesV2, 0, mod, tc.ensureTermination)
			require.NoError(t, err)

			actual, err := c.Next()
			require.NoError(t, err)
			require.Equal(t, tc.exp, format(actual.Operations))
		})
	}
}

func TestCompiler_threads(t *testing.T) {
	tests := []struct {
		name               string
		body               []byte
		noDropBeforeReturn bool
		expected           unionOperation
	}{
		{
			name: "i32.atomic.load8_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Load8U, 0x1, 0x8, // alignment=2^1, offset=8
			},
			expected: newOperationAtomicLoad8(unsignedTypeI32, memoryArg{Alignment: 0x1, Offset: 0x8}),
		},
		{
			name: "i32.atomic.load16_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Load16U, 0x2, 0x8, // alignment=2^(2-1), offset=8
			},
			expected: newOperationAtomicLoad16(unsignedTypeI32, memoryArg{Alignment: 0x2, Offset: 0x8}),
		},
		{
			name: "i32.atomic.load",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Load, 0x3, 0x8, // alignment=2^(3-1), offset=8
			},
			expected: newOperationAtomicLoad(unsignedTypeI32, memoryArg{Alignment: 0x3, Offset: 0x8}),
		},
		{
			name: "i64.atomic.load8_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Load8U, 0x1, 0x8, // alignment=2^1, offset=8
			},
			expected: newOperationAtomicLoad8(unsignedTypeI64, memoryArg{Alignment: 0x1, Offset: 0x8}),
		},
		{
			name: "i64.atomic.load16_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Load16U, 0x2, 0x8, // alignment=2^(2-1), offset=8
			},
			expected: newOperationAtomicLoad16(unsignedTypeI64, memoryArg{Alignment: 0x2, Offset: 0x8}),
		},
		{
			name: "i64.atomic.load32_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Load32U, 0x3, 0x8, // alignment=2^(3-1), offset=8
			},
			expected: newOperationAtomicLoad(unsignedTypeI32, memoryArg{Alignment: 0x3, Offset: 0x8}),
		},
		{
			name: "i64.atomic.load",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Load, 0x4, 0x8, // alignment=2^(4-1), offset=8
			},
			expected: newOperationAtomicLoad(unsignedTypeI64, memoryArg{Alignment: 0x4, Offset: 0x8}),
		},
		{
			name: "i32.atomic.store8",
			body: []byte{
				wasm.OpcodeI32Const, 0x1,
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Store8, 0x1, 0x8, // alignment=2^1, offset=8
			},
			noDropBeforeReturn: true,
			expected:           newOperationAtomicStore8(unsignedTypeI32, memoryArg{Alignment: 0x1, Offset: 0x8}),
		},
		{
			name: "i32.atomic.store16",
			body: []byte{
				wasm.OpcodeI32Const, 0x1,
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Store16, 0x2, 0x8, // alignment=2^(2-1), offset=8
			},
			noDropBeforeReturn: true,
			expected:           newOperationAtomicStore16(unsignedTypeI32, memoryArg{Alignment: 0x2, Offset: 0x8}),
		},
		{
			name: "i32.atomic.store",
			body: []byte{
				wasm.OpcodeI32Const, 0x1,
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Store, 0x3, 0x8, // alignment=2^(3-1), offset=8
			},
			noDropBeforeReturn: true,
			expected:           newOperationAtomicStore(unsignedTypeI32, memoryArg{Alignment: 0x3, Offset: 0x8}),
		},
		{
			name: "i64.atomic.store8",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Store8, 0x1, 0x8, // alignment=2^1, offset=8
			},
			noDropBeforeReturn: true,
			expected:           newOperationAtomicStore8(unsignedTypeI64, memoryArg{Alignment: 0x1, Offset: 0x8}),
		},
		{
			name: "i64.atomic.store16",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Store16, 0x2, 0x8, // alignment=2^(2-1), offset=8
			},
			noDropBeforeReturn: true,
			expected:           newOperationAtomicStore16(unsignedTypeI64, memoryArg{Alignment: 0x2, Offset: 0x8}),
		},
		{
			name: "i64.atomic.store32",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Store32, 0x3, 0x8, // alignment=2^(3-1), offset=8
			},
			noDropBeforeReturn: true,
			expected:           newOperationAtomicStore(unsignedTypeI32, memoryArg{Alignment: 0x3, Offset: 0x8}),
		},
		{
			name: "i64.atomic.store",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Store, 0x4, 0x8, // alignment=2^(4-1), offset=8
			},
			noDropBeforeReturn: true,
			expected:           newOperationAtomicStore(unsignedTypeI64, memoryArg{Alignment: 0x4, Offset: 0x8}),
		},
		{
			name: "i32.atomic.rmw8.add_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI32Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw8AddU, 0x1, 0x8, // alignment=2^1, offset=8
			},
			expected: newOperationAtomicRMW8(unsignedTypeI32, memoryArg{Alignment: 0x1, Offset: 0x8}, atomicArithmeticOpAdd),
		},
		{
			name: "i32.atomic.rmw16.add_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI32Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw16AddU, 0x2, 0x8, // alignment=2^(2-1), offset=8
			},
			expected: newOperationAtomicRMW16(unsignedTypeI32, memoryArg{Alignment: 0x2, Offset: 0x8}, atomicArithmeticOpAdd),
		},
		{
			name: "i32.atomic.rmw.add",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI32Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32RmwAdd, 0x3, 0x8, // alignment=2^(3-1), offset=8
			},
			expected: newOperationAtomicRMW(unsignedTypeI32, memoryArg{Alignment: 0x3, Offset: 0x8}, atomicArithmeticOpAdd),
		},
		{
			name: "i64.atomic.rmw8.add_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw8AddU, 0x1, 0x8, // alignment=2^1, offset=8
			},
			expected: newOperationAtomicRMW8(unsignedTypeI64, memoryArg{Alignment: 0x1, Offset: 0x8}, atomicArithmeticOpAdd),
		},
		{
			name: "i64.atomic.rmw16.add_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw16AddU, 0x2, 0x8, // alignment=2^(2-1), offset=8
			},
			expected: newOperationAtomicRMW16(unsignedTypeI64, memoryArg{Alignment: 0x2, Offset: 0x8}, atomicArithmeticOpAdd),
		},
		{
			name: "i64.atomic.rmw32.add_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw32AddU, 0x3, 0x8, // alignment=2^(3-1), offset=8
			},
			expected: newOperationAtomicRMW(unsignedTypeI32, memoryArg{Alignment: 0x3, Offset: 0x8}, atomicArithmeticOpAdd),
		},
		{
			name: "i64.atomic.rmw.add",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64RmwAdd, 0x4, 0x8, // alignment=2^(4-1), offset=8
			},
			expected: newOperationAtomicRMW(unsignedTypeI64, memoryArg{Alignment: 0x4, Offset: 0x8}, atomicArithmeticOpAdd),
		},
		{
			name: "i32.atomic.rmw8.sub_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI32Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw8SubU, 0x1, 0x8, // alignment=2^1, offset=8
			},
			expected: newOperationAtomicRMW8(unsignedTypeI32, memoryArg{Alignment: 0x1, Offset: 0x8}, atomicArithmeticOpSub),
		},
		{
			name: "i32.atomic.rmw16.sub_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI32Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw16SubU, 0x2, 0x8, // alignment=2^(2-1), offset=8
			},
			expected: newOperationAtomicRMW16(unsignedTypeI32, memoryArg{Alignment: 0x2, Offset: 0x8}, atomicArithmeticOpSub),
		},
		{
			name: "i32.atomic.rmw.sub",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI32Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32RmwSub, 0x3, 0x8, // alignment=2^(3-1), offset=8
			},
			expected: newOperationAtomicRMW(unsignedTypeI32, memoryArg{Alignment: 0x3, Offset: 0x8}, atomicArithmeticOpSub),
		},
		{
			name: "i64.atomic.rmw8.sub_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw8SubU, 0x1, 0x8, // alignment=2^1, offset=8
			},
			expected: newOperationAtomicRMW8(unsignedTypeI64, memoryArg{Alignment: 0x1, Offset: 0x8}, atomicArithmeticOpSub),
		},
		{
			name: "i64.atomic.rmw16.sub_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw16SubU, 0x2, 0x8, // alignment=2^(2-1), offset=8
			},
			expected: newOperationAtomicRMW16(unsignedTypeI64, memoryArg{Alignment: 0x2, Offset: 0x8}, atomicArithmeticOpSub),
		},
		{
			name: "i64.atomic.rmw32.sub_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw32SubU, 0x3, 0x8, // alignment=2^(3-1), offset=8
			},
			expected: newOperationAtomicRMW(unsignedTypeI32, memoryArg{Alignment: 0x3, Offset: 0x8}, atomicArithmeticOpSub),
		},
		{
			name: "i64.atomic.rmw.sub",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64RmwSub, 0x4, 0x8, // alignment=2^(4-1), offset=8
			},
			expected: newOperationAtomicRMW(unsignedTypeI64, memoryArg{Alignment: 0x4, Offset: 0x8}, atomicArithmeticOpSub),
		},
		{
			name: "i32.atomic.rmw8.and_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI32Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw8AndU, 0x1, 0x8, // alignment=2^1, offset=8
			},
			expected: newOperationAtomicRMW8(unsignedTypeI32, memoryArg{Alignment: 0x1, Offset: 0x8}, atomicArithmeticOpAnd),
		},
		{
			name: "i32.atomic.rmw16.and_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI32Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw16AndU, 0x2, 0x8, // alignment=2^(2-1), offset=8
			},
			expected: newOperationAtomicRMW16(unsignedTypeI32, memoryArg{Alignment: 0x2, Offset: 0x8}, atomicArithmeticOpAnd),
		},
		{
			name: "i32.atomic.rmw.and",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI32Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32RmwAnd, 0x3, 0x8, // alignment=2^(3-1), offset=8
			},
			expected: newOperationAtomicRMW(unsignedTypeI32, memoryArg{Alignment: 0x3, Offset: 0x8}, atomicArithmeticOpAnd),
		},
		{
			name: "i64.atomic.rmw8.and_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw8AndU, 0x1, 0x8, // alignment=2^1, offset=8
			},
			expected: newOperationAtomicRMW8(unsignedTypeI64, memoryArg{Alignment: 0x1, Offset: 0x8}, atomicArithmeticOpAnd),
		},
		{
			name: "i64.atomic.rmw16.and_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw16AndU, 0x2, 0x8, // alignment=2^(2-1), offset=8
			},
			expected: newOperationAtomicRMW16(unsignedTypeI64, memoryArg{Alignment: 0x2, Offset: 0x8}, atomicArithmeticOpAnd),
		},
		{
			name: "i64.atomic.rmw32.and_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw32AndU, 0x3, 0x8, // alignment=2^(3-1), offset=8
			},
			expected: newOperationAtomicRMW(unsignedTypeI32, memoryArg{Alignment: 0x3, Offset: 0x8}, atomicArithmeticOpAnd),
		},
		{
			name: "i64.atomic.rmw.and",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64RmwAnd, 0x4, 0x8, // alignment=2^(4-1), offset=8
			},
			expected: newOperationAtomicRMW(unsignedTypeI64, memoryArg{Alignment: 0x4, Offset: 0x8}, atomicArithmeticOpAnd),
		},
		{
			name: "i32.atomic.rmw8.or",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI32Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw8OrU, 0x1, 0x8, // alignment=2^1, offset=8
			},
			expected: newOperationAtomicRMW8(unsignedTypeI32, memoryArg{Alignment: 0x1, Offset: 0x8}, atomicArithmeticOpOr),
		},
		{
			name: "i32.atomic.rmw16.or_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI32Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw16OrU, 0x2, 0x8, // alignment=2^(2-1), offset=8
			},
			expected: newOperationAtomicRMW16(unsignedTypeI32, memoryArg{Alignment: 0x2, Offset: 0x8}, atomicArithmeticOpOr),
		},
		{
			name: "i32.atomic.rmw.or",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI32Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32RmwOr, 0x3, 0x8, // alignment=2^(3-1), offset=8
			},
			expected: newOperationAtomicRMW(unsignedTypeI32, memoryArg{Alignment: 0x3, Offset: 0x8}, atomicArithmeticOpOr),
		},
		{
			name: "i64.atomic.rmw8.or_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw8OrU, 0x1, 0x8, // alignment=2^1, offset=8
			},
			expected: newOperationAtomicRMW8(unsignedTypeI64, memoryArg{Alignment: 0x1, Offset: 0x8}, atomicArithmeticOpOr),
		},
		{
			name: "i64.atomic.rmw16.or_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw16OrU, 0x2, 0x8, // alignment=2^(2-1), offset=8
			},
			expected: newOperationAtomicRMW16(unsignedTypeI64, memoryArg{Alignment: 0x2, Offset: 0x8}, atomicArithmeticOpOr),
		},
		{
			name: "i64.atomic.rmw32.or_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw32OrU, 0x3, 0x8, // alignment=2^(3-1), offset=8
			},
			expected: newOperationAtomicRMW(unsignedTypeI32, memoryArg{Alignment: 0x3, Offset: 0x8}, atomicArithmeticOpOr),
		},
		{
			name: "i64.atomic.rmw.or",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64RmwOr, 0x4, 0x8, // alignment=2^(4-1), offset=8
			},
			expected: newOperationAtomicRMW(unsignedTypeI64, memoryArg{Alignment: 0x4, Offset: 0x8}, atomicArithmeticOpOr),
		},
		{
			name: "i32.atomic.rmw8.xor_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI32Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw8XorU, 0x1, 0x8, // alignment=2^1, offset=8
			},
			expected: newOperationAtomicRMW8(unsignedTypeI32, memoryArg{Alignment: 0x1, Offset: 0x8}, atomicArithmeticOpXor),
		},
		{
			name: "i32.atomic.rmw16.xor_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI32Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw16XorU, 0x2, 0x8, // alignment=2^(2-1), offset=8
			},
			expected: newOperationAtomicRMW16(unsignedTypeI32, memoryArg{Alignment: 0x2, Offset: 0x8}, atomicArithmeticOpXor),
		},
		{
			name: "i32.atomic.rmw.xor",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI32Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32RmwXor, 0x3, 0x8, // alignment=2^(3-1), offset=8
			},
			expected: newOperationAtomicRMW(unsignedTypeI32, memoryArg{Alignment: 0x3, Offset: 0x8}, atomicArithmeticOpXor),
		},
		{
			name: "i64.atomic.rmw8.xor_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw8XorU, 0x1, 0x8, // alignment=2^1, offset=8
			},
			expected: newOperationAtomicRMW8(unsignedTypeI64, memoryArg{Alignment: 0x1, Offset: 0x8}, atomicArithmeticOpXor),
		},
		{
			name: "i64.atomic.rmw16.xor_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw16XorU, 0x2, 0x8, // alignment=2^(2-1), offset=8
			},
			expected: newOperationAtomicRMW16(unsignedTypeI64, memoryArg{Alignment: 0x2, Offset: 0x8}, atomicArithmeticOpXor),
		},
		{
			name: "i64.atomic.rmw32.xor_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw32XorU, 0x3, 0x8, // alignment=2^(3-1), offset=8
			},
			expected: newOperationAtomicRMW(unsignedTypeI32, memoryArg{Alignment: 0x3, Offset: 0x8}, atomicArithmeticOpXor),
		},
		{
			name: "i64.atomic.rmw.xor",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64RmwXor, 0x4, 0x8, // alignment=2^(4-1), offset=8
			},
			expected: newOperationAtomicRMW(unsignedTypeI64, memoryArg{Alignment: 0x4, Offset: 0x8}, atomicArithmeticOpXor),
		},
		{
			name: "i32.atomic.rmw8.xchg_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI32Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw8XchgU, 0x1, 0x8, // alignment=2^1, offset=8
			},
			expected: newOperationAtomicRMW8(unsignedTypeI32, memoryArg{Alignment: 0x1, Offset: 0x8}, atomicArithmeticOpNop),
		},
		{
			name: "i32.atomic.rmw16.xchg_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI32Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw16XchgU, 0x2, 0x8, // alignment=2^(2-1), offset=8
			},
			expected: newOperationAtomicRMW16(unsignedTypeI32, memoryArg{Alignment: 0x2, Offset: 0x8}, atomicArithmeticOpNop),
		},
		{
			name: "i32.atomic.rmw.xchg",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI32Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32RmwXchg, 0x3, 0x8, // alignment=2^(3-1), offset=8
			},
			expected: newOperationAtomicRMW(unsignedTypeI32, memoryArg{Alignment: 0x3, Offset: 0x8}, atomicArithmeticOpNop),
		},
		{
			name: "i64.atomic.rmw8.xchg_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw8XchgU, 0x1, 0x8, // alignment=2^1, offset=8
			},
			expected: newOperationAtomicRMW8(unsignedTypeI64, memoryArg{Alignment: 0x1, Offset: 0x8}, atomicArithmeticOpNop),
		},
		{
			name: "i64.atomic.rmw16.xchg_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw16XchgU, 0x2, 0x8, // alignment=2^(2-1), offset=8
			},
			expected: newOperationAtomicRMW16(unsignedTypeI64, memoryArg{Alignment: 0x2, Offset: 0x8}, atomicArithmeticOpNop),
		},
		{
			name: "i64.atomic.rmw32.xchg_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw32XchgU, 0x3, 0x8, // alignment=2^(3-1), offset=8
			},
			expected: newOperationAtomicRMW(unsignedTypeI32, memoryArg{Alignment: 0x3, Offset: 0x8}, atomicArithmeticOpNop),
		},
		{
			name: "i64.atomic.rmw.xchg",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64RmwXchg, 0x4, 0x8, // alignment=2^(4-1), offset=8
			},
			expected: newOperationAtomicRMW(unsignedTypeI64, memoryArg{Alignment: 0x4, Offset: 0x8}, atomicArithmeticOpNop),
		},
		{
			name: "i32.atomic.rmw8.cmpxchg_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI32Const, 0x1,
				wasm.OpcodeI32Const, 0x2,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw8CmpxchgU, 0x1, 0x8, // alignment=2^1, offset=8
			},
			expected: newOperationAtomicRMW8Cmpxchg(unsignedTypeI32, memoryArg{Alignment: 0x1, Offset: 0x8}),
		},
		{
			name: "i32.atomic.rmw16.cmpxchg_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI32Const, 0x1,
				wasm.OpcodeI32Const, 0x2,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw16CmpxchgU, 0x2, 0x8, // alignment=2^(2-1), offset=8
			},
			expected: newOperationAtomicRMW16Cmpxchg(unsignedTypeI32, memoryArg{Alignment: 0x2, Offset: 0x8}),
		},
		{
			name: "i32.atomic.rmw.cmpxchg",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI32Const, 0x1,
				wasm.OpcodeI32Const, 0x2,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32RmwCmpxchg, 0x3, 0x8, // alignment=2^(3-1), offset=8
			},
			expected: newOperationAtomicRMWCmpxchg(unsignedTypeI32, memoryArg{Alignment: 0x3, Offset: 0x8}),
		},
		{
			name: "i64.atomic.rmw8.xchg_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeI64Const, 0x2,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw8CmpxchgU, 0x1, 0x8, // alignment=2^1, offset=8
			},
			expected: newOperationAtomicRMW8Cmpxchg(unsignedTypeI64, memoryArg{Alignment: 0x1, Offset: 0x8}),
		},
		{
			name: "i64.atomic.rmw16.cmpxchg_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeI64Const, 0x2,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw16CmpxchgU, 0x2, 0x8, // alignment=2^(2-1), offset=8
			},
			expected: newOperationAtomicRMW16Cmpxchg(unsignedTypeI64, memoryArg{Alignment: 0x2, Offset: 0x8}),
		},
		{
			name: "i64.atomic.rmw32.cmpxchg_u",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw32CmpxchgU, 0x3, 0x8, // alignment=2^(3-1), offset=8
			},
			expected: newOperationAtomicRMWCmpxchg(unsignedTypeI32, memoryArg{Alignment: 0x3, Offset: 0x8}),
		},
		{
			name: "i64.atomic.rmw.cmpxchg",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeI64Const, 0x2,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64RmwCmpxchg, 0x4, 0x8, // alignment=2^(4-1), offset=8
			},
			expected: newOperationAtomicRMWCmpxchg(unsignedTypeI64, memoryArg{Alignment: 0x4, Offset: 0x8}),
		},
		{
			name: "memory.atomic.wait32",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI32Const, 0x1,
				wasm.OpcodeI64Const, 0x2,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicMemoryWait32, 0x3, 0x8, // alignment=2^(3-1), offset=8
			},
			expected: newOperationAtomicMemoryWait(unsignedTypeI32, memoryArg{Alignment: 0x3, Offset: 0x8}),
		},
		{
			name: "memory.atomic.wait64",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI64Const, 0x1,
				wasm.OpcodeI64Const, 0x2,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicMemoryWait64, 0x4, 0x8, // alignment=2^(4-1), offset=8
			},
			expected: newOperationAtomicMemoryWait(unsignedTypeI64, memoryArg{Alignment: 0x4, Offset: 0x8}),
		},
		{
			name: "memory.atomic.notify",
			body: []byte{
				wasm.OpcodeI32Const, 0x0,
				wasm.OpcodeI32Const, 0x1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicMemoryNotify, 0x3, 0x8, // alignment=2^(3-1), offset=8
			},
			expected: newOperationAtomicMemoryNotify(memoryArg{Alignment: 0x3, Offset: 0x8}),
		},
		{
			name: "memory.atomic.fence",
			body: []byte{
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicFence, 0x0, // consistency=0
			},
			noDropBeforeReturn: true,
			expected:           newOperationAtomicFence(),
		},
	}

	for _, tc := range tests {
		tt := tc
		t.Run(tt.name, func(t *testing.T) {
			body := append([]byte{}, tt.body...)
			if !tt.noDropBeforeReturn {
				body = append(body, wasm.OpcodeDrop)
			}
			body = append(body, wasm.OpcodeEnd)
			module := &wasm.Module{
				TypeSection:     []wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0},
				MemorySection:   &wasm.Memory{},
				CodeSection:     []wasm.Code{{Body: body}},
			}
			c, err := newCompiler(api.CoreFeaturesV2, 0, module, false)
			require.NoError(t, err)

			res, err := c.Next()
			require.NoError(t, err)
			if tt.noDropBeforeReturn {
				require.Equal(t, tc.expected, res.Operations[len(res.Operations)-2])
			} else {
				require.Equal(t, tc.expected, res.Operations[len(res.Operations)-3])
			}
		})
	}
}
