package wazeroir

import (
	"testing"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/internal/wasm"
	"github.com/tetratelabs/wazero/internal/wasm/text"
)

var (
	f64, i32   = wasm.ValueTypeF64, wasm.ValueTypeI32
	i32_i32    = &wasm.FunctionType{Params: []wasm.ValueType{i32}, Results: []wasm.ValueType{i32}}
	i32i32_i32 = &wasm.FunctionType{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}}
	v_v        = &wasm.FunctionType{}
	v_f64f64   = &wasm.FunctionType{Results: []wasm.ValueType{f64, f64}}
)

func TestCompile(t *testing.T) {
	tests := []struct {
		name            string
		module          *wasm.Module
		expected        *CompilationResult
		enabledFeatures wasm.Features
	}{
		{
			name:   "nullary",
			module: requireModuleText(t, `(module (func))`),
			expected: &CompilationResult{
				Operations: []Operation{ // begin with params: []
					&OperationBr{Target: &BranchTarget{}}, // return!
				},
				LabelCallers: map[string]uint32{},
				Functions:    []uint32{0},
				Types:        []*wasm.FunctionType{{}},
				Signature:    &wasm.FunctionType{},
			},
		},
		{
			name: "identity",
			module: requireModuleText(t, `(module
  (func (param $x i32) (result i32) local.get 0)
)`),
			expected: &CompilationResult{
				Operations: []Operation{ // begin with params: [$x]
					&OperationPick{Depth: 0},                                 // [$x, $x]
					&OperationDrop{Depth: &InclusiveRange{Start: 1, End: 1}}, // [$x]
					&OperationBr{Target: &BranchTarget{}},                    // return!
				},
				LabelCallers: map[string]uint32{},
				Types:        []*wasm.FunctionType{{Params: []wasm.ValueType{i32}, Results: []wasm.ValueType{i32}}},
				Functions:    []uint32{0},
				Signature:    &wasm.FunctionType{Params: []wasm.ValueType{wasm.ValueTypeI32}, Results: []wasm.ValueType{wasm.ValueTypeI32}},
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			enabledFeatures := tc.enabledFeatures
			if enabledFeatures == 0 {
				enabledFeatures = wasm.FeaturesFinished
			}

			res, err := CompileFunctions(enabledFeatures, tc.module)
			require.NoError(t, err)
			require.Equal(t, tc.expected, res[0])
		})
	}
}

func TestCompile_Block(t *testing.T) {
	tests := []struct {
		name            string
		module          *wasm.Module
		expected        *CompilationResult
		enabledFeatures wasm.Features
	}{
		{
			name: "type-i32-i32",
			module: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{v_v},
				FunctionSection: []wasm.Index{0},
				CodeSection: []*wasm.Code{{Body: []byte{
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
					&OperationBr{
						Target: &BranchTarget{
							Label: &Label{FrameID: 2, Kind: LabelKindContinuation}, // arbitrary FrameID
						},
					},
					&OperationLabel{
						Label: &Label{FrameID: 2, Kind: LabelKindContinuation}, // arbitrary FrameID
					},
					&OperationBr{Target: &BranchTarget{}}, // return!
				},
				// Note: i32.add comes after br 0 so is unreachable. Compilation succeeds when it feels like it
				// shouldn't because the br instruction is stack-polymorphic. In other words, (br 0) substitutes for the
				// two i32 parameters to add.
				LabelCallers: map[string]uint32{".L2_cont": 1},
				Functions:    []uint32{0},
				Types:        []*wasm.FunctionType{v_v},
				Signature:    v_v,
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

func TestCompile_MultiValue(t *testing.T) {
	i32i32_i32i32 := &wasm.FunctionType{Params: []wasm.ValueType{
		wasm.ValueTypeI32, wasm.ValueTypeI32},
		Results: []wasm.ValueType{wasm.ValueTypeI32, wasm.ValueTypeI32},
	}
	_i32i64 := &wasm.FunctionType{Results: []wasm.ValueType{wasm.ValueTypeI32, wasm.ValueTypeI64}}

	tests := []struct {
		name            string
		module          *wasm.Module
		expected        *CompilationResult
		enabledFeatures wasm.Features
	}{
		{
			name: "swap",
			module: requireModuleText(t, `(module
  (func (param $x i32) (param $y i32) (result i32 i32) local.get 1 local.get 0)
)`),

			expected: &CompilationResult{
				Operations: []Operation{ // begin with params: [$x, $y]
					&OperationPick{Depth: 0},                                 // [$x, $y, $y]
					&OperationPick{Depth: 2},                                 // [$x, $y, $y, $x]
					&OperationDrop{Depth: &InclusiveRange{Start: 2, End: 3}}, // [$y, $x]
					&OperationBr{Target: &BranchTarget{}},                    // return!
				},
				LabelCallers: map[string]uint32{},
				Signature:    i32i32_i32i32,
				Functions:    []wasm.Index{0},
				Types:        []*wasm.FunctionType{i32i32_i32i32},
			},
		},
		{
			name: "br.wast - type-f64-f64-value",
			module: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{v_f64f64},
				FunctionSection: []wasm.Index{0},
				CodeSection: []*wasm.Code{{Body: []byte{
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
					&OperationConstF64{Value: 4}, // [4]
					&OperationConstF64{Value: 5}, // [4, 5]
					&OperationBr{
						Target: &BranchTarget{
							Label: &Label{FrameID: 2, Kind: LabelKindContinuation}, // arbitrary FrameID
						},
					},
					&OperationLabel{
						Label: &Label{FrameID: 2, Kind: LabelKindContinuation}, // arbitrary FrameID
					},
					&OperationBr{Target: &BranchTarget{}}, // return!
				},
				// Note: f64.add comes after br 0 so is unreachable. This is why neither the add, nor its other operand
				// are in the above compilation result.
				LabelCallers: map[string]uint32{".L2_cont": 1}, // arbitrary label
				Signature:    v_f64f64,
				Functions:    []wasm.Index{0},
				Types:        []*wasm.FunctionType{v_f64f64},
			},
		},
		{
			name: "call.wast - $const-i32-i64",
			module: requireModuleText(t, `(module
  (func $const-i32-i64 (result i32 i64) i32.const 306 i64.const 356)
)`),

			expected: &CompilationResult{
				Operations: []Operation{ // begin with params: []
					&OperationConstI32{Value: 306},        // [306]
					&OperationConstI64{Value: 356},        // [306, 356]
					&OperationBr{Target: &BranchTarget{}}, // return!
				},
				LabelCallers: map[string]uint32{},
				Signature:    _i32i64,
				Functions:    []wasm.Index{0},
				Types:        []*wasm.FunctionType{_i32i64},
			},
		},
		{
			name: "if.wast - param",
			module: &wasm.Module{
				TypeSection:     []*wasm.FunctionType{i32_i32}, // (func (param i32) (result i32)
				FunctionSection: []wasm.Index{0},
				CodeSection: []*wasm.Code{{Body: []byte{
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
					&OperationConstI32{Value: 1}, // [$0, 1]
					&OperationPick{Depth: 1},     // [$0, 1, $0]
					&OperationBrIf{ // [$0, 1]
						Then: &BranchTargetDrop{Target: &BranchTarget{Label: &Label{FrameID: 2, Kind: LabelKindHeader}}},
						Else: &BranchTargetDrop{Target: &BranchTarget{Label: &Label{FrameID: 2, Kind: LabelKindElse}}},
					},
					&OperationLabel{Label: &Label{FrameID: 2, Kind: LabelKindHeader}},
					&OperationConstI32{Value: 2},         // [$0, 1, 2]
					&OperationAdd{Type: UnsignedTypeI32}, // [$0, 3]
					&OperationBr{Target: &BranchTarget{Label: &Label{FrameID: 2, Kind: LabelKindContinuation}}},
					&OperationLabel{Label: &Label{FrameID: 2, Kind: LabelKindElse}},
					&OperationConstI32{Value: uint32(api.EncodeI32(-2))}, // [$0, 1, -2]
					&OperationAdd{Type: UnsignedTypeI32},                 // [$0, -1]
					&OperationBr{Target: &BranchTarget{Label: &Label{FrameID: 2, Kind: LabelKindContinuation}}},
					&OperationLabel{Label: &Label{FrameID: 2, Kind: LabelKindContinuation}},
					&OperationDrop{Depth: &InclusiveRange{Start: 1, End: 1}}, // .L2 = [3], .L2_else = [-1]
					&OperationBr{Target: &BranchTarget{}},
				},
				LabelCallers: map[string]uint32{
					".L2":      1,
					".L2_cont": 2,
					".L2_else": 1,
				},
				Signature: i32_i32,
				Functions: []wasm.Index{0},
				Types:     []*wasm.FunctionType{i32_i32},
			},
		},
		{
			name: "if.wast - params",
			module: &wasm.Module{
				TypeSection: []*wasm.FunctionType{
					i32_i32,    // (func (param i32) (result i32)
					i32i32_i32, // (if (param i32 i32) (result i32)
				},
				FunctionSection: []wasm.Index{0},
				CodeSection: []*wasm.Code{{Body: []byte{
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
					&OperationConstI32{Value: 1}, // [$0, 1]
					&OperationConstI32{Value: 2}, // [$0, 1, 2]
					&OperationPick{Depth: 2},     // [$0, 1, 2, $0]
					&OperationBrIf{ // [$0, 1, 2]
						Then: &BranchTargetDrop{Target: &BranchTarget{Label: &Label{FrameID: 2, Kind: LabelKindHeader}}},
						Else: &BranchTargetDrop{Target: &BranchTarget{Label: &Label{FrameID: 2, Kind: LabelKindElse}}},
					},
					&OperationLabel{Label: &Label{FrameID: 2, Kind: LabelKindHeader}},
					&OperationAdd{Type: UnsignedTypeI32}, // [$0, 3]
					&OperationBr{Target: &BranchTarget{Label: &Label{FrameID: 2, Kind: LabelKindContinuation}}},
					&OperationLabel{Label: &Label{FrameID: 2, Kind: LabelKindElse}},
					&OperationSub{Type: UnsignedTypeI32}, // [$0, -1]
					&OperationBr{Target: &BranchTarget{Label: &Label{FrameID: 2, Kind: LabelKindContinuation}}},
					&OperationLabel{Label: &Label{FrameID: 2, Kind: LabelKindContinuation}},
					&OperationDrop{Depth: &InclusiveRange{Start: 1, End: 1}}, // .L2 = [3], .L2_else = [-1]
					&OperationBr{Target: &BranchTarget{}},
				},
				LabelCallers: map[string]uint32{
					".L2":      1,
					".L2_cont": 2,
					".L2_else": 1,
				},
				Signature: i32_i32,
				Functions: []wasm.Index{0},
				Types:     []*wasm.FunctionType{i32_i32, i32i32_i32},
			},
		},
		{
			name: "if.wast - params-break",
			module: &wasm.Module{
				TypeSection: []*wasm.FunctionType{
					i32_i32,    // (func (param i32) (result i32)
					i32i32_i32, // (if (param i32 i32) (result i32)
				},
				FunctionSection: []wasm.Index{0},
				CodeSection: []*wasm.Code{{Body: []byte{
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
					&OperationConstI32{Value: 1}, // [$0, 1]
					&OperationConstI32{Value: 2}, // [$0, 1, 2]
					&OperationPick{Depth: 2},     // [$0, 1, 2, $0]
					&OperationBrIf{ // [$0, 1, 2]
						Then: &BranchTargetDrop{Target: &BranchTarget{Label: &Label{FrameID: 2, Kind: LabelKindHeader}}},
						Else: &BranchTargetDrop{Target: &BranchTarget{Label: &Label{FrameID: 2, Kind: LabelKindElse}}},
					},
					&OperationLabel{Label: &Label{FrameID: 2, Kind: LabelKindHeader}},
					&OperationAdd{Type: UnsignedTypeI32}, // [$0, 3]
					&OperationBr{Target: &BranchTarget{Label: &Label{FrameID: 2, Kind: LabelKindContinuation}}},
					&OperationLabel{Label: &Label{FrameID: 2, Kind: LabelKindElse}},
					&OperationSub{Type: UnsignedTypeI32}, // [$0, -1]
					&OperationBr{Target: &BranchTarget{Label: &Label{FrameID: 2, Kind: LabelKindContinuation}}},
					&OperationLabel{Label: &Label{FrameID: 2, Kind: LabelKindContinuation}},
					&OperationDrop{Depth: &InclusiveRange{Start: 1, End: 1}}, // .L2 = [3], .L2_else = [-1]
					&OperationBr{Target: &BranchTarget{}},
				},
				LabelCallers: map[string]uint32{
					".L2":      1,
					".L2_cont": 2,
					".L2_else": 1,
				},
				Signature: i32_i32,
				Functions: []wasm.Index{0},
				Types:     []*wasm.FunctionType{i32_i32, i32i32_i32},
			},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			enabledFeatures := tc.enabledFeatures
			if enabledFeatures == 0 {
				enabledFeatures = wasm.FeaturesFinished
			}
			res, err := CompileFunctions(enabledFeatures, tc.module)
			require.NoError(t, err)
			require.Equal(t, tc.expected, res[0])
		})
	}
}

func requireCompilationResult(t *testing.T, enabledFeatures wasm.Features, expected *CompilationResult, module *wasm.Module) {
	if enabledFeatures == 0 {
		enabledFeatures = wasm.FeaturesFinished
	}
	res, err := CompileFunctions(enabledFeatures, module)
	require.NoError(t, err)
	require.Equal(t, expected, res[0])
}

func requireModuleText(t *testing.T, source string) *wasm.Module {
	m, err := text.DecodeModule([]byte(source), wasm.FeaturesFinished, wasm.MemoryMaxPages)
	require.NoError(t, err)
	return m
}
