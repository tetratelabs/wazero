package testcases

import (
	"math"

	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/wasm"
)

const ExportedFunctionName = "f"

var (
	Empty     = TestCase{Name: "empty", Module: SingleFunctionModule(vv, []byte{wasm.OpcodeEnd}, nil)}
	Constants = TestCase{Name: "consts", Module: SingleFunctionModule(wasm.FunctionType{
		Results: []wasm.ValueType{i32, i64, f32, f64},
	}, []byte{
		wasm.OpcodeI32Const, 1,
		wasm.OpcodeI64Const, 2,
		wasm.OpcodeF32Const,
		byte(math.Float32bits(32.0)),
		byte(math.Float32bits(32.0) >> 8),
		byte(math.Float32bits(32.0) >> 16),
		byte(math.Float32bits(32.0) >> 24),
		wasm.OpcodeF64Const,
		byte(math.Float64bits(64.0)),
		byte(math.Float64bits(64.0) >> 8),
		byte(math.Float64bits(64.0) >> 16),
		byte(math.Float64bits(64.0) >> 24),
		byte(math.Float64bits(64.0) >> 32),
		byte(math.Float64bits(64.0) >> 40),
		byte(math.Float64bits(64.0) >> 48),
		byte(math.Float64bits(64.0) >> 56),
		wasm.OpcodeEnd,
	}, nil)}
	Unreachable  = TestCase{Name: "unreachable", Module: SingleFunctionModule(vv, []byte{wasm.OpcodeUnreachable, wasm.OpcodeEnd}, nil)}
	OnlyReturn   = TestCase{Name: "only_return", Module: SingleFunctionModule(vv, []byte{wasm.OpcodeReturn, wasm.OpcodeEnd}, nil)}
	Params       = TestCase{Name: "params", Module: SingleFunctionModule(i32f32f64_v, []byte{wasm.OpcodeReturn, wasm.OpcodeEnd}, nil)}
	AddSubReturn = TestCase{
		Name: "add_sub_params_return_const",
		Module: SingleFunctionModule(wasm.FunctionType{Results: []wasm.ValueType{i32, i32, i64, i64}}, []byte{
			// Small i32 constants should be inlined on arm64, amd64.
			wasm.OpcodeI32Const, 4,
			wasm.OpcodeI32Const, 5,
			wasm.OpcodeI32Add,
			wasm.OpcodeI32Const, 6,
			wasm.OpcodeI32Sub,

			// Large i32 constants should be inlined on amd4, load from register on arm64.
			wasm.OpcodeI32Const, 3,
			wasm.OpcodeI32Const, 0xff, 0xff, 0xff, 0xff, 0,
			wasm.OpcodeI32Add,
			wasm.OpcodeI32Const, 0xff, 0xff, 0xff, 0xff, 0,
			wasm.OpcodeI32Sub,

			// Small i64 constants should be inlined on arm64, amd64.
			wasm.OpcodeI64Const, 4,
			wasm.OpcodeI64Const, 5,
			wasm.OpcodeI64Add,
			wasm.OpcodeI64Const, 6,
			wasm.OpcodeI64Sub,

			// Large i64 constants are loaded from register on arm64, amd64.
			wasm.OpcodeI64Const, 3,
			wasm.OpcodeI64Const, 0xff, 0xff, 0xff, 0xff, 0xff, 0,
			wasm.OpcodeI64Add,
			wasm.OpcodeI64Const, 0xff, 0xff, 0xff, 0xff, 0xff, 0,
			wasm.OpcodeI64Sub,

			wasm.OpcodeEnd,
		}, nil),
	}
	AddSubParamsReturn = TestCase{
		Name: "add_sub_params_return",
		Module: SingleFunctionModule(i32i32_i32, []byte{
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeI32Add,
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeI32Sub,
			wasm.OpcodeEnd,
		}, nil),
	}
	ArithmReturn = TestCase{
		Name: "arithm return",
		Module: SingleFunctionModule(
			wasm.FunctionType{
				Params: []wasm.ValueType{i32, i32, i32, i64, i64, i64},
				Results: []wasm.ValueType{
					i32, i32, i32, i32,
					i32, i32, i32,
					i32, i32,

					i64, i64, i64, i64,
					i64, i64, i64,
					i64, i64,
				},
			},
			[]byte{
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeI32Mul,
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeI32And,
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeI32Or,
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeI32Xor,

				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeI32Shl,
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeLocalGet, 2,
				wasm.OpcodeI32ShrS,
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeI32ShrU,

				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeI32Rotr,
				// Exercise the case where the amount is an immediate.
				wasm.OpcodeI32Const, 10,
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeI32Rotl,

				// i64

				wasm.OpcodeLocalGet, 3,
				wasm.OpcodeLocalGet, 4,
				wasm.OpcodeI64Mul,
				wasm.OpcodeLocalGet, 3,
				wasm.OpcodeLocalGet, 4,
				wasm.OpcodeI64And,
				wasm.OpcodeLocalGet, 3,
				wasm.OpcodeLocalGet, 4,
				wasm.OpcodeI64Or,
				wasm.OpcodeLocalGet, 3,
				wasm.OpcodeLocalGet, 4,
				wasm.OpcodeI64Xor,

				wasm.OpcodeLocalGet, 3,
				wasm.OpcodeLocalGet, 4,
				wasm.OpcodeI64Shl,
				wasm.OpcodeLocalGet, 5,
				wasm.OpcodeLocalGet, 4,
				wasm.OpcodeI64ShrS,
				wasm.OpcodeLocalGet, 3,
				wasm.OpcodeLocalGet, 4,
				wasm.OpcodeI64ShrU,

				wasm.OpcodeLocalGet, 3,
				wasm.OpcodeLocalGet, 4,
				wasm.OpcodeI64Rotr,
				// Exercise the case where the amount is an immediate.
				wasm.OpcodeI64Const, 10,
				wasm.OpcodeLocalGet, 3,
				wasm.OpcodeI64Rotl,

				wasm.OpcodeEnd,
			}, nil),
	}
	DivUReturn32 = TestCase{
		Name: "div return unsigned 32",
		Module: SingleFunctionModule(
			wasm.FunctionType{
				Params:  []wasm.ValueType{i32, i32, i32, i32},
				Results: []wasm.ValueType{i32, i32},
			},
			[]byte{
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeI32DivU,

				wasm.OpcodeLocalGet, 2,
				wasm.OpcodeLocalGet, 3,
				wasm.OpcodeI32RemU,

				wasm.OpcodeEnd,
			}, nil),
	}
	DivUReturn64 = TestCase{
		Name: "div return unsigned 64",
		Module: SingleFunctionModule(
			wasm.FunctionType{
				Params:  []wasm.ValueType{i64, i64, i64, i64},
				Results: []wasm.ValueType{i64, i64},
			},
			[]byte{
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeI64DivU,

				wasm.OpcodeLocalGet, 2,
				wasm.OpcodeLocalGet, 3,
				wasm.OpcodeI64RemU,

				wasm.OpcodeEnd,
			}, nil),
	}
	DivSReturn32 = TestCase{
		Name: "div return signed 32",
		Module: SingleFunctionModule(
			wasm.FunctionType{
				Params:  []wasm.ValueType{i32, i32, i32, i32},
				Results: []wasm.ValueType{i32, i32},
			},
			[]byte{
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeI32DivS,

				wasm.OpcodeLocalGet, 2,
				wasm.OpcodeLocalGet, 3,
				wasm.OpcodeI32RemS,

				wasm.OpcodeEnd,
			}, nil),
	}
	DivSReturn32_weird = TestCase{
		Name: "div return signed 32",
		Module: SingleFunctionModule(
			wasm.FunctionType{
				Params:  []wasm.ValueType{i32, i32, i32, i32},
				Results: []wasm.ValueType{i32, i32},
			},
			[]byte{
				wasm.OpcodeLocalGet, 2,
				wasm.OpcodeLocalGet, 3,
				wasm.OpcodeI32RemS,

				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeI32DivS,

				wasm.OpcodeEnd,
			}, nil),
	}

	DivSReturn64 = TestCase{
		Name: "div return signed 64",
		Module: SingleFunctionModule(
			wasm.FunctionType{
				Params:  []wasm.ValueType{i64, i64, i64, i64},
				Results: []wasm.ValueType{i64, i64},
			},
			[]byte{
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeI64DivS,

				wasm.OpcodeLocalGet, 2,
				wasm.OpcodeLocalGet, 3,
				wasm.OpcodeI64RemS,

				wasm.OpcodeEnd,
			}, nil),
	}
	Locals       = TestCase{Name: "locals", Module: SingleFunctionModule(vv, []byte{wasm.OpcodeEnd}, []wasm.ValueType{i32, i64, f32, f64})}
	LocalsParams = TestCase{
		Name: "locals_params",
		Module: SingleFunctionModule(
			i64f32f64_i64f32f64,
			[]byte{
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeI64Add,
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeI64Sub,

				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeF32Add,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeF32Sub,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeF32Mul,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeF32Div,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeF32Max,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeF32Min,

				wasm.OpcodeLocalGet, 2,
				wasm.OpcodeLocalGet, 2,
				wasm.OpcodeF64Add,
				wasm.OpcodeLocalGet, 2,
				wasm.OpcodeF64Sub,
				wasm.OpcodeLocalGet, 2,
				wasm.OpcodeF64Mul,
				wasm.OpcodeLocalGet, 2,
				wasm.OpcodeF64Div,
				wasm.OpcodeLocalGet, 2,
				wasm.OpcodeF64Max,
				wasm.OpcodeLocalGet, 2,
				wasm.OpcodeF64Min,

				wasm.OpcodeEnd,
			}, []wasm.ValueType{i32, i64, f32, f64},
		),
	}
	LocalParamReturn = TestCase{
		Name: "local_param_return",
		Module: SingleFunctionModule(i32_i32i32, []byte{
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeEnd,
		}, []wasm.ValueType{i32}),
	}
	LocalParamTeeReturn = TestCase{
		Name: "local_param_tee_return",
		Module: SingleFunctionModule(i32_i32i32, []byte{
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalTee, 1,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeEnd,
		}, []wasm.ValueType{i32}),
	}
	SwapParamAndReturn = TestCase{
		Name: "swap_param_and_return",
		Module: SingleFunctionModule(i32i32_i32i32, []byte{
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeEnd,
		}, nil),
	}
	Selects = TestCase{
		Name: "swap_param_and_return",
		Module: SingleFunctionModule(i32i32i64i64f32f32f64f64_i32i64, []byte{
			// i32 select.
			wasm.OpcodeLocalGet, 0, // x
			wasm.OpcodeLocalGet, 1, // y
			// cond
			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeI64Eq,
			wasm.OpcodeSelect,

			// i64 select.
			wasm.OpcodeLocalGet, 2, // x
			wasm.OpcodeLocalGet, 3, // y
			wasm.OpcodeLocalGet, 1, // cond
			wasm.OpcodeTypedSelect, 1, wasm.ValueTypeI64,

			// f32 select.
			wasm.OpcodeLocalGet, 4, // x
			wasm.OpcodeLocalGet, 5, // y
			// cond
			wasm.OpcodeLocalGet, 6,
			wasm.OpcodeLocalGet, 7,
			wasm.OpcodeF64Gt,
			wasm.OpcodeTypedSelect, 1, wasm.ValueTypeF32,

			// f64 select.
			wasm.OpcodeLocalGet, 6, // x
			wasm.OpcodeLocalGet, 7, // y
			// cond
			wasm.OpcodeLocalGet, 4,
			wasm.OpcodeLocalGet, 5,
			wasm.OpcodeF32Ne,
			wasm.OpcodeTypedSelect, 1, wasm.ValueTypeF64,

			wasm.OpcodeEnd,
		}, nil),
	}
	SwapParamsAndReturn = TestCase{
		Name: "swap_params_and_return",
		Module: SingleFunctionModule(i32i32_i32i32, []byte{
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalSet, 1,
			wasm.OpcodeLocalSet, 0,
			wasm.OpcodeBlock, blockSignature_vv,
			wasm.OpcodeEnd,
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeEnd,
		}, nil),
	}
	BlockBr = TestCase{
		Name: "block_br",
		Module: SingleFunctionModule(vv, []byte{
			wasm.OpcodeBlock, 0,
			wasm.OpcodeBr, 0,
			wasm.OpcodeEnd,
			wasm.OpcodeEnd,
		}, []wasm.ValueType{i32, i64, f32, f64}),
	}
	BlockBrIf = TestCase{
		Name: "block_br_if",
		Module: SingleFunctionModule(vv, []byte{
			wasm.OpcodeBlock, 0,
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeBrIf, 0,
			wasm.OpcodeUnreachable,
			wasm.OpcodeEnd,
			wasm.OpcodeEnd,
		}, []wasm.ValueType{i32}),
	}
	LoopBr = TestCase{
		Name: "loop_br",
		Module: SingleFunctionModule(vv, []byte{
			wasm.OpcodeLoop, 0,
			wasm.OpcodeBr, 0,
			wasm.OpcodeEnd,
			wasm.OpcodeEnd,
		}, []wasm.ValueType{}),
	}
	LoopBrWithParamResults = TestCase{
		Name: "loop_with_param_results",
		Module: SingleFunctionModule(i32i32_i32, []byte{
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeLoop, 0,
			wasm.OpcodeI32Const, 1,
			wasm.OpcodeBrIf, 0,
			wasm.OpcodeDrop,
			wasm.OpcodeEnd,
			wasm.OpcodeEnd,
		}, []wasm.ValueType{}),
	}
	LoopBrIf = TestCase{
		Name: "loop_br_if",
		Module: SingleFunctionModule(vv, []byte{
			wasm.OpcodeLoop, 0,
			wasm.OpcodeI32Const, 1,
			wasm.OpcodeBrIf, 0,
			wasm.OpcodeReturn,
			wasm.OpcodeEnd,
			wasm.OpcodeEnd,
		}, []wasm.ValueType{}),
	}
	BlockBlockBr = TestCase{
		Name: "block_block_br",
		Module: SingleFunctionModule(vv, []byte{
			wasm.OpcodeBlock, 0,
			wasm.OpcodeBlock, 0,
			wasm.OpcodeBr, 1,
			wasm.OpcodeEnd,
			wasm.OpcodeEnd,
			wasm.OpcodeEnd,
		}, []wasm.ValueType{i32, i64, f32, f64}),
	}
	IfWithoutElse = TestCase{
		Name: "if_without_else",
		Module: SingleFunctionModule(vv, []byte{
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeIf, 0,
			wasm.OpcodeEnd,
			wasm.OpcodeEnd,
		}, []wasm.ValueType{i32}),
	}
	IfElse = TestCase{
		Name: "if_else",
		Module: SingleFunctionModule(vv, []byte{
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeIf, 0,
			wasm.OpcodeElse,
			wasm.OpcodeBr, 1,
			wasm.OpcodeEnd,
			wasm.OpcodeEnd,
		}, []wasm.ValueType{i32}),
	}
	SinglePredecessorLocalRefs = TestCase{
		Name: "single_predecessor_local_refs",
		Module: &wasm.Module{
			TypeSection:     []wasm.FunctionType{vv, v_i32},
			FunctionSection: []wasm.Index{1},
			CodeSection: []wasm.Code{{
				LocalTypes: []wasm.ValueType{i32, i32, i32},
				Body: []byte{
					wasm.OpcodeLocalGet, 0,
					wasm.OpcodeIf, 0,
					// This is defined in the first block which is the sole predecessor of If.
					wasm.OpcodeLocalGet, 2,
					wasm.OpcodeReturn,
					wasm.OpcodeElse,
					wasm.OpcodeEnd,
					// This is defined in the first block which is the sole predecessor of this block.
					// Note that If block will never reach here because it's returning early.
					wasm.OpcodeLocalGet, 0,
					wasm.OpcodeEnd,
				},
			}},
		},
	}
	MultiPredecessorLocalRef = TestCase{
		Name: "multi_predecessor_local_ref",
		Module: SingleFunctionModule(i32i32_i32, []byte{
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeIf, blockSignature_vv,
			// Set the first param to the local.
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalSet, 2,
			wasm.OpcodeElse,
			// Set the second param to the local.
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeLocalSet, 2,
			wasm.OpcodeEnd,

			// Return the local as a result which has multiple definitions in predecessors (Then and Else).
			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeEnd,
		}, []wasm.ValueType{i32}),
	}
	ReferenceValueFromUnsealedBlock = TestCase{
		Name: "reference_value_from_unsealed_block",
		Module: SingleFunctionModule(i32_i32, []byte{
			wasm.OpcodeLoop, blockSignature_vv,
			// Loop will not be sealed until we reach the end,
			// so this will result in referencing the unsealed definition search.
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeReturn,
			wasm.OpcodeEnd,
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeEnd,
		}, []wasm.ValueType{i32}),
	}
	ReferenceValueFromUnsealedBlock2 = TestCase{
		Name: "reference_value_from_unsealed_block2",
		Module: SingleFunctionModule(i32_i32, []byte{
			wasm.OpcodeLoop, blockSignature_vv,
			wasm.OpcodeBlock, blockSignature_vv,

			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeBrIf, 1,
			wasm.OpcodeEnd,

			wasm.OpcodeEnd,
			wasm.OpcodeI32Const, 0,
			wasm.OpcodeEnd,
		}, []wasm.ValueType{}),
	}
	ReferenceValueFromUnsealedBlock3 = TestCase{
		Name: "reference_value_from_unsealed_block3",
		Module: SingleFunctionModule(i32_v, []byte{
			wasm.OpcodeLoop, blockSignature_vv,
			wasm.OpcodeBlock, blockSignature_vv,
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeBrIf, 2,
			wasm.OpcodeEnd,
			wasm.OpcodeI32Const, 1,
			wasm.OpcodeLocalSet, 0,
			wasm.OpcodeBr, 0,
			wasm.OpcodeEnd,
			wasm.OpcodeEnd,
		}, []wasm.ValueType{}),
	}
	CallSimple = TestCase{
		Name: "call_simple",
		Module: &wasm.Module{
			TypeSection:     []wasm.FunctionType{v_i32, v_i32},
			FunctionSection: []wasm.Index{0, 1},
			CodeSection: []wasm.Code{
				{Body: []byte{
					// Call v_i32.
					wasm.OpcodeCall, 1,
					wasm.OpcodeEnd,
				}},
				// v_i32: return 40.
				{Body: []byte{wasm.OpcodeI32Const, 40, wasm.OpcodeEnd}},
			},
			ExportSection: []wasm.Export{{Name: ExportedFunctionName, Index: 0, Type: wasm.ExternTypeFunc}},
		},
	}
	Call = TestCase{
		Name: "call",
		Module: &wasm.Module{
			TypeSection:     []wasm.FunctionType{v_i32i32, v_i32, i32i32_i32, i32_i32i32},
			FunctionSection: []wasm.Index{0, 1, 2, 3},
			CodeSection: []wasm.Code{
				{Body: []byte{
					// Call v_i32.
					wasm.OpcodeCall, 1,
					// Call i32i32_i32.
					wasm.OpcodeI32Const, 5,
					wasm.OpcodeCall, 2,
					// Call i32_i32i32.
					wasm.OpcodeCall, 3,
					wasm.OpcodeEnd,
				}},
				// v_i32: return 100.
				{Body: []byte{wasm.OpcodeI32Const, 40, wasm.OpcodeEnd}},
				// i32i32_i32: adds.
				{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeI32Add, wasm.OpcodeEnd}},
				// i32_i32i32: duplicates.
				{Body: []byte{wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 0, wasm.OpcodeEnd}},
			},
			ExportSection: []wasm.Export{{Name: ExportedFunctionName, Index: 0, Type: wasm.ExternTypeFunc}},
		},
	}
	ManyMiddleValues = TestCase{
		Name: "many_middle_values",
		Module: SingleFunctionModule(wasm.FunctionType{
			Params:  []wasm.ValueType{i32, f32},
			Results: []wasm.ValueType{i32, f32},
		}, []byte{
			wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Const, 1, wasm.OpcodeI32Mul,
			wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Const, 2, wasm.OpcodeI32Mul,
			wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Const, 3, wasm.OpcodeI32Mul,
			wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Const, 4, wasm.OpcodeI32Mul,
			wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Const, 5, wasm.OpcodeI32Mul,
			wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Const, 6, wasm.OpcodeI32Mul,
			wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Const, 7, wasm.OpcodeI32Mul,
			wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Const, 8, wasm.OpcodeI32Mul,
			wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Const, 9, wasm.OpcodeI32Mul,
			wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Const, 10, wasm.OpcodeI32Mul,
			wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Const, 11, wasm.OpcodeI32Mul,
			wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Const, 12, wasm.OpcodeI32Mul,
			wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Const, 13, wasm.OpcodeI32Mul,
			wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Const, 14, wasm.OpcodeI32Mul,
			wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Const, 15, wasm.OpcodeI32Mul,
			wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Const, 16, wasm.OpcodeI32Mul,
			wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Const, 17, wasm.OpcodeI32Mul,
			wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Const, 18, wasm.OpcodeI32Mul,
			wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Const, 19, wasm.OpcodeI32Mul,
			wasm.OpcodeLocalGet, 0, wasm.OpcodeI32Const, 20, wasm.OpcodeI32Mul,

			wasm.OpcodeI32Add, wasm.OpcodeI32Add, wasm.OpcodeI32Add, wasm.OpcodeI32Add, wasm.OpcodeI32Add,
			wasm.OpcodeI32Add, wasm.OpcodeI32Add, wasm.OpcodeI32Add, wasm.OpcodeI32Add, wasm.OpcodeI32Add,
			wasm.OpcodeI32Add, wasm.OpcodeI32Add, wasm.OpcodeI32Add, wasm.OpcodeI32Add, wasm.OpcodeI32Add,
			wasm.OpcodeI32Add, wasm.OpcodeI32Add, wasm.OpcodeI32Add, wasm.OpcodeI32Add,

			wasm.OpcodeLocalGet, 1, wasm.OpcodeF32Const, 0, 0, 0x80, 0x3f, wasm.OpcodeF32Mul,
			wasm.OpcodeLocalGet, 1, wasm.OpcodeF32Const, 0, 0, 0, 0x40, wasm.OpcodeF32Mul,
			wasm.OpcodeLocalGet, 1, wasm.OpcodeF32Const, 0, 0, 0x40, 0x40, wasm.OpcodeF32Mul,
			wasm.OpcodeLocalGet, 1, wasm.OpcodeF32Const, 0, 0, 0x80, 0x40, wasm.OpcodeF32Mul,
			wasm.OpcodeLocalGet, 1, wasm.OpcodeF32Const, 0, 0, 0xa0, 0x40, wasm.OpcodeF32Mul,
			wasm.OpcodeLocalGet, 1, wasm.OpcodeF32Const, 0, 0, 0xc0, 0x40, wasm.OpcodeF32Mul,
			wasm.OpcodeLocalGet, 1, wasm.OpcodeF32Const, 0, 0, 0xe0, 0x40, wasm.OpcodeF32Mul,
			wasm.OpcodeLocalGet, 1, wasm.OpcodeF32Const, 0, 0, 0, 0x41, wasm.OpcodeF32Mul,
			wasm.OpcodeLocalGet, 1, wasm.OpcodeF32Const, 0, 0, 0x10, 0x41, wasm.OpcodeF32Mul,
			wasm.OpcodeLocalGet, 1, wasm.OpcodeF32Const, 0, 0, 0x20, 0x41, wasm.OpcodeF32Mul,
			wasm.OpcodeLocalGet, 1, wasm.OpcodeF32Const, 0, 0, 0x30, 0x41, wasm.OpcodeF32Mul,
			wasm.OpcodeLocalGet, 1, wasm.OpcodeF32Const, 0, 0, 0x40, 0x41, wasm.OpcodeF32Mul,
			wasm.OpcodeLocalGet, 1, wasm.OpcodeF32Const, 0, 0, 0x50, 0x41, wasm.OpcodeF32Mul,
			wasm.OpcodeLocalGet, 1, wasm.OpcodeF32Const, 0, 0, 0x60, 0x41, wasm.OpcodeF32Mul,
			wasm.OpcodeLocalGet, 1, wasm.OpcodeF32Const, 0, 0, 0x70, 0x41, wasm.OpcodeF32Mul,
			wasm.OpcodeLocalGet, 1, wasm.OpcodeF32Const, 0, 0, 0x80, 0x41, wasm.OpcodeF32Mul,
			wasm.OpcodeLocalGet, 1, wasm.OpcodeF32Const, 0, 0, 0x88, 0x41, wasm.OpcodeF32Mul,
			wasm.OpcodeLocalGet, 1, wasm.OpcodeF32Const, 0, 0, 0x90, 0x41, wasm.OpcodeF32Mul,
			wasm.OpcodeLocalGet, 1, wasm.OpcodeF32Const, 0, 0, 0x98, 0x41, wasm.OpcodeF32Mul,
			wasm.OpcodeLocalGet, 1, wasm.OpcodeF32Const, 0, 0, 0xa0, 0x41, wasm.OpcodeF32Mul,

			wasm.OpcodeF32Add, wasm.OpcodeF32Add, wasm.OpcodeF32Add, wasm.OpcodeF32Add, wasm.OpcodeF32Add,
			wasm.OpcodeF32Add, wasm.OpcodeF32Add, wasm.OpcodeF32Add, wasm.OpcodeF32Add, wasm.OpcodeF32Add,
			wasm.OpcodeF32Add, wasm.OpcodeF32Add, wasm.OpcodeF32Add, wasm.OpcodeF32Add, wasm.OpcodeF32Add,
			wasm.OpcodeF32Add, wasm.OpcodeF32Add, wasm.OpcodeF32Add, wasm.OpcodeF32Add,

			wasm.OpcodeEnd,
		}, nil),
	}
	CallManyParams = TestCase{
		Name: "call_many_params",
		Module: &wasm.Module{
			TypeSection: []wasm.FunctionType{
				{Params: []wasm.ValueType{i32, i64, f32, f64}},
				{
					Params: []wasm.ValueType{
						i32, i64, f32, f64, i32, i64, f32, f64,
						i32, i64, f32, f64, i32, i64, f32, f64,
						i32, i64, f32, f64, i32, i64, f32, f64,
						i32, i64, f32, f64, i32, i64, f32, f64,
						i32, i64, f32, f64, i32, i64, f32, f64,
					},
				},
			},
			FunctionSection: []wasm.Index{0, 1},
			CodeSection: []wasm.Code{
				{
					Body: []byte{
						wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 2, wasm.OpcodeLocalGet, 3,
						wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 2, wasm.OpcodeLocalGet, 3,
						wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 2, wasm.OpcodeLocalGet, 3,
						wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 2, wasm.OpcodeLocalGet, 3,
						wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 2, wasm.OpcodeLocalGet, 3,
						wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 2, wasm.OpcodeLocalGet, 3,
						wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 2, wasm.OpcodeLocalGet, 3,
						wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 2, wasm.OpcodeLocalGet, 3,
						wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 2, wasm.OpcodeLocalGet, 3,
						wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 2, wasm.OpcodeLocalGet, 3,
						wasm.OpcodeCall, 1,
						wasm.OpcodeEnd,
					},
				},
				{Body: []byte{wasm.OpcodeEnd}},
			},
		},
	}
	CallManyReturns = TestCase{
		Name: "call_many_returns",
		Module: &wasm.Module{
			TypeSection: []wasm.FunctionType{
				{
					Params: []wasm.ValueType{i32, i64, f32, f64},
					Results: []wasm.ValueType{
						i32, i64, f32, f64, i32, i64, f32, f64,
						i32, i64, f32, f64, i32, i64, f32, f64,
						i32, i64, f32, f64, i32, i64, f32, f64,
						i32, i64, f32, f64, i32, i64, f32, f64,
						i32, i64, f32, f64, i32, i64, f32, f64,
					},
				},
			},
			FunctionSection: []wasm.Index{0, 0},
			CodeSection: []wasm.Code{
				{
					Body: []byte{
						wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 2, wasm.OpcodeLocalGet, 3,
						wasm.OpcodeCall, 1,
						wasm.OpcodeEnd,
					},
				},
				{Body: []byte{
					wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 2, wasm.OpcodeLocalGet, 3,
					wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 2, wasm.OpcodeLocalGet, 3,
					wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 2, wasm.OpcodeLocalGet, 3,
					wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 2, wasm.OpcodeLocalGet, 3,
					wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 2, wasm.OpcodeLocalGet, 3,
					wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 2, wasm.OpcodeLocalGet, 3,
					wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 2, wasm.OpcodeLocalGet, 3,
					wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 2, wasm.OpcodeLocalGet, 3,
					wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 2, wasm.OpcodeLocalGet, 3,
					wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 2, wasm.OpcodeLocalGet, 3,
					wasm.OpcodeEnd,
				}},
			},
		},
	}
	ManyParamsSmallResults = TestCase{
		Name: "many_params_small_results",
		Module: SingleFunctionModule(wasm.FunctionType{
			Params: []wasm.ValueType{
				i32, i64, f32, f64, i32, i64, f32, f64,
				i32, i64, f32, f64, i32, i64, f32, f64,
				i32, i64, f32, f64, i32, i64, f32, f64,
				i32, i64, f32, f64, i32, i64, f32, f64,
				i32, i64, f32, f64, i32, i64, f32, f64,
			},
			Results: []wasm.ValueType{
				i32, i64, f32, f64,
			},
		}, []byte{
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 9,
			wasm.OpcodeLocalGet, 18,
			wasm.OpcodeLocalGet, 27,
			wasm.OpcodeEnd,
		}, nil),
	}
	SmallParamsManyResults = TestCase{
		Name: "small_params_many_results",
		Module: SingleFunctionModule(wasm.FunctionType{
			Params: []wasm.ValueType{i32, i64, f32, f64},
			Results: []wasm.ValueType{
				i32, i64, f32, f64, i32, i64, f32, f64,
				i32, i64, f32, f64, i32, i64, f32, f64,
				i32, i64, f32, f64, i32, i64, f32, f64,
				i32, i64, f32, f64, i32, i64, f32, f64,
				i32, i64, f32, f64, i32, i64, f32, f64,
			},
		}, []byte{
			wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 2, wasm.OpcodeLocalGet, 3, wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 2, wasm.OpcodeLocalGet, 3,
			wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 2, wasm.OpcodeLocalGet, 3, wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 2, wasm.OpcodeLocalGet, 3,
			wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 2, wasm.OpcodeLocalGet, 3, wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 2, wasm.OpcodeLocalGet, 3,
			wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 2, wasm.OpcodeLocalGet, 3, wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 2, wasm.OpcodeLocalGet, 3,
			wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 2, wasm.OpcodeLocalGet, 3, wasm.OpcodeLocalGet, 0, wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 2, wasm.OpcodeLocalGet, 3,
			wasm.OpcodeEnd,
		}, nil),
	}
	ManyParamsManyResults = TestCase{
		Name: "many_params_many_results",
		Module: SingleFunctionModule(wasm.FunctionType{
			Params: []wasm.ValueType{
				i32, i64, f32, f64, i32, i64, f32, f64,
				i32, i64, f32, f64, i32, i64, f32, f64,
				i32, i64, f32, f64, i32, i64, f32, f64,
				i32, i64, f32, f64, i32, i64, f32, f64,
				i32, i64, f32, f64, i32, i64, f32, f64,
			},
			Results: []wasm.ValueType{
				f64, f32, i64, i32, f64, f32, i64, i32,
				f64, f32, i64, i32, f64, f32, i64, i32,
				f64, f32, i64, i32, f64, f32, i64, i32,
				f64, f32, i64, i32, f64, f32, i64, i32,
				f64, f32, i64, i32, f64, f32, i64, i32,
			},
		}, []byte{
			wasm.OpcodeLocalGet, 39, wasm.OpcodeLocalGet, 38, wasm.OpcodeLocalGet, 37, wasm.OpcodeLocalGet, 36,
			wasm.OpcodeLocalGet, 35, wasm.OpcodeLocalGet, 34, wasm.OpcodeLocalGet, 33, wasm.OpcodeLocalGet, 32,
			wasm.OpcodeLocalGet, 31, wasm.OpcodeLocalGet, 30, wasm.OpcodeLocalGet, 29, wasm.OpcodeLocalGet, 28,
			wasm.OpcodeLocalGet, 27, wasm.OpcodeLocalGet, 26, wasm.OpcodeLocalGet, 25, wasm.OpcodeLocalGet, 24,
			wasm.OpcodeLocalGet, 23, wasm.OpcodeLocalGet, 22, wasm.OpcodeLocalGet, 21, wasm.OpcodeLocalGet, 20,
			wasm.OpcodeLocalGet, 19, wasm.OpcodeLocalGet, 18, wasm.OpcodeLocalGet, 17, wasm.OpcodeLocalGet, 16,
			wasm.OpcodeLocalGet, 15, wasm.OpcodeLocalGet, 14, wasm.OpcodeLocalGet, 13, wasm.OpcodeLocalGet, 12,
			wasm.OpcodeLocalGet, 11, wasm.OpcodeLocalGet, 10, wasm.OpcodeLocalGet, 9, wasm.OpcodeLocalGet, 8,
			wasm.OpcodeLocalGet, 7, wasm.OpcodeLocalGet, 6, wasm.OpcodeLocalGet, 5, wasm.OpcodeLocalGet, 4,
			wasm.OpcodeLocalGet, 3, wasm.OpcodeLocalGet, 2, wasm.OpcodeLocalGet, 1, wasm.OpcodeLocalGet, 0,
			wasm.OpcodeEnd,
		}, nil),
	}
	IntegerComparisons = TestCase{
		Name: "integer_comparisons",
		Module: SingleFunctionModule(wasm.FunctionType{
			Params:  []wasm.ValueType{i32, i32, i64, i64},
			Results: []wasm.ValueType{i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32},
		}, []byte{
			// eq.
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeI32Eq,
			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeI64Eq,
			// neq.
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeI32Ne,
			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeI64Ne,
			// LtS.
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeI32LtS,
			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeI64LtS,
			// LtU.
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeI32LtU,
			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeI64LtU,
			// GtS.
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeI32GtS,
			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeI64GtS,
			// GtU.
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeI32GtU,
			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeI64GtU,
			// LeS.
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeI32LeS,
			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeI64LeS,
			// LeU.
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeI32LeU,
			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeI64LeU,
			// GeS.
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeI32GeS,
			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeI64GeS,
			// GeU.
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeI32GeU,
			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeI64GeU,
			wasm.OpcodeEnd,
		}, []wasm.ValueType{}),
	}
	IntegerBitwise = TestCase{
		Name: "integer_bitwise",
		Module: SingleFunctionModule(wasm.FunctionType{
			Params:  []wasm.ValueType{i32, i32, i64, i64},
			Results: []wasm.ValueType{i32, i32, i32, i32, i64, i64, i64, i64, i64, i64},
		}, []byte{
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeI32And,

			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeI32Or,

			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeI32Xor,

			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeI32Rotr,

			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeI64And,

			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeI64Or,

			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeI64Xor,

			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeI64Const, 8,
			wasm.OpcodeI64Shl,
			wasm.OpcodeI64Xor,

			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeI64Rotl,

			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeI64Rotr,

			wasm.OpcodeEnd,
		}, []wasm.ValueType{}),
	}
	IntegerShift = TestCase{
		Name: "integer_shift",
		Module: SingleFunctionModule(wasm.FunctionType{
			Params:  []wasm.ValueType{i32, i32, i64, i64},
			Results: []wasm.ValueType{i32, i32, i64, i64, i32, i32, i64, i64, i32, i32, i64, i64},
		}, []byte{
			// logical left.
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeI32Shl,
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeI32Const, 31,
			wasm.OpcodeI32Shl,
			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeI64Shl,
			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeI64Const, 32,
			wasm.OpcodeI64Shl,
			// logical right.
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeI32ShrU,
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeI32Const, 31,
			wasm.OpcodeI32ShrU,
			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeI64ShrU,
			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeI64Const, 32,
			wasm.OpcodeI64ShrU,
			// arithmetic right.
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeI32ShrS,
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeI32Const, 31,
			wasm.OpcodeI32ShrS,
			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeI64ShrS,
			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeI64Const, 32,
			wasm.OpcodeI64ShrS,
			wasm.OpcodeEnd,
		}, []wasm.ValueType{}),
	}
	IntegerExtensions = TestCase{
		Name: "integer_extensions",
		Module: SingleFunctionModule(wasm.FunctionType{
			Params:  []wasm.ValueType{i32, i64},
			Results: []wasm.ValueType{i64, i64, i64, i64, i64, i32, i32},
		}, []byte{
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeI64ExtendI32S,

			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeI64ExtendI32U,

			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeI64Extend8S,

			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeI64Extend16S,

			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeI64Extend32S,

			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeI32Extend8S,

			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeI32Extend16S,

			wasm.OpcodeEnd,
		}, []wasm.ValueType{}),
	}
	IntegerBitCounts = TestCase{
		Name: "integer_bit_counts",
		Module: SingleFunctionModule(wasm.FunctionType{
			Params:  []wasm.ValueType{i32, i64},
			Results: []wasm.ValueType{i32, i32, i32, i64, i64, i64},
		}, []byte{
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeI32Clz,

			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeI32Ctz,

			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeI32Popcnt,

			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeI64Clz,

			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeI64Ctz,

			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeI64Popcnt,

			wasm.OpcodeEnd,
		}, []wasm.ValueType{}),
	}
	FloatComparisons = TestCase{
		Name: "float_comparisons",
		Module: SingleFunctionModule(wasm.FunctionType{
			Params:  []wasm.ValueType{f32, f32, f64, f64},
			Results: []wasm.ValueType{i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32, i32},
		}, []byte{
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeF32Eq,
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeF32Ne,
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeF32Lt,
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeF32Gt,
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeF32Le,
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeF32Ge,

			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeF64Eq,
			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeF64Ne,
			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeF64Lt,
			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeF64Gt,
			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeF64Le,
			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeF64Ge,

			wasm.OpcodeEnd,
		}, []wasm.ValueType{}),
	}
	FloatArithm = TestCase{
		Name: "float_arithm",
		Module: SingleFunctionModule(wasm.FunctionType{
			Params: []wasm.ValueType{f64, f64, f64, f32, f32, f32},
			Results: []wasm.ValueType{
				f64, f64, f64, f64, f64, f64, f64, f64, f64, f64, f64, f64,
				f32, f32, f32, f32, f32, f32, f32, f32, f32, f32, f32, f32,
			},
		}, []byte{
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeF64Neg,

			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeF64Neg,
			wasm.OpcodeF64Abs,

			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeF64Sqrt,

			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeF64Add,

			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeF64Sub,

			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeF64Mul,

			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeF64Div,

			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeF64Nearest,

			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeF64Floor,

			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeF64Ceil,

			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeF64Trunc,

			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeF64Neg,
			wasm.OpcodeF64Copysign,

			// 32-bit floats.
			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeF32Neg,

			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeF32Neg,
			wasm.OpcodeF32Abs,

			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeF32Sqrt,

			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeLocalGet, 4,
			wasm.OpcodeF32Add,

			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeLocalGet, 4,
			wasm.OpcodeF32Sub,

			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeLocalGet, 4,
			wasm.OpcodeF32Mul,

			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeLocalGet, 4,
			wasm.OpcodeF32Div,

			wasm.OpcodeLocalGet, 5,
			wasm.OpcodeF32Nearest,

			wasm.OpcodeLocalGet, 5,
			wasm.OpcodeF32Floor,

			wasm.OpcodeLocalGet, 5,
			wasm.OpcodeF32Ceil,

			wasm.OpcodeLocalGet, 5,
			wasm.OpcodeF32Trunc,

			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeLocalGet, 4,
			wasm.OpcodeF32Neg,
			wasm.OpcodeF32Copysign,

			wasm.OpcodeEnd,
		}, []wasm.ValueType{}),
	}
	FloatLe = TestCase{
		Name: "float_le",
		Module: SingleFunctionModule(wasm.FunctionType{
			Params:  []wasm.ValueType{f64},
			Results: []wasm.ValueType{i64, i64},
		}, []byte{
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeF64Const, 0, 0, 0, 0, 0, 0, 240, 63, // 1.0
			wasm.OpcodeF64Le,
			wasm.OpcodeIf, blockSignature_vv,
			wasm.OpcodeI64Const, 1,
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeF64Const, 0, 0, 0, 0, 0, 0, 240, 63, // 1.0
			wasm.OpcodeF64Le,
			wasm.OpcodeI64ExtendI32U,
			wasm.OpcodeReturn,
			wasm.OpcodeElse,
			wasm.OpcodeI64Const, 0,
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeF64Const, 0, 0, 0, 0, 0, 0, 240, 63, // 1.0
			wasm.OpcodeF64Le,
			wasm.OpcodeI64ExtendI32U,
			wasm.OpcodeReturn,
			wasm.OpcodeEnd,
			wasm.OpcodeUnreachable,
			wasm.OpcodeEnd,
		}, []wasm.ValueType{}),
	}
	MinMaxFloat = TestCase{
		Name: "min_max_float",
		Module: SingleFunctionModule(wasm.FunctionType{
			Params: []wasm.ValueType{f64, f64, f32, f32},
			Results: []wasm.ValueType{
				f64, f64,
				f32, f32,
			},
		}, []byte{
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeF64Min,

			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeF64Max,

			// 32-bit floats.
			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeF32Min,

			wasm.OpcodeLocalGet, 2,
			wasm.OpcodeLocalGet, 3,
			wasm.OpcodeF32Max,

			wasm.OpcodeEnd,
		}, []wasm.ValueType{}),
	}
	FloatConversions = TestCase{
		Name: "float_conversions",
		Module: SingleFunctionModule(wasm.FunctionType{
			Params:  []wasm.ValueType{f64, f32},
			Results: []wasm.ValueType{i64, i64, i32, i32, i64, i64, i32, i32, f32, f64},
		}, []byte{
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeI64TruncF64S,

			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeI64TruncF32S,

			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeI32TruncF64S,

			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeI32TruncF32S,

			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeI64TruncF64U,

			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeI64TruncF32U,

			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeI32TruncF64U,

			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeI32TruncF32U,

			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeF32DemoteF64,

			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeF64PromoteF32,

			wasm.OpcodeEnd,
		}, []wasm.ValueType{}),
	}
	NonTrappingFloatConversions = TestCase{
		Name: "float_conversions",
		Module: SingleFunctionModule(wasm.FunctionType{
			Params:  []wasm.ValueType{f64, f32},
			Results: []wasm.ValueType{i64, i64, i32, i32, i64, i64, i32, i32},
		}, []byte{
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeMiscPrefix, wasm.OpcodeMiscI64TruncSatF64S,

			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeMiscPrefix, wasm.OpcodeMiscI64TruncSatF32S,

			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeMiscPrefix, wasm.OpcodeMiscI32TruncSatF64S,

			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeMiscPrefix, wasm.OpcodeMiscI32TruncSatF32S,

			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeMiscPrefix, wasm.OpcodeMiscI64TruncSatF64U,

			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeMiscPrefix, wasm.OpcodeMiscI64TruncSatF32U,

			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeMiscPrefix, wasm.OpcodeMiscI32TruncSatF64U,

			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeMiscPrefix, wasm.OpcodeMiscI32TruncSatF32U,

			wasm.OpcodeEnd,
		}, []wasm.ValueType{}),
	}
	FibonacciRecursive = TestCase{
		Name: "recursive_fibonacci",
		Module: SingleFunctionModule(i32_i32, []byte{
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeI32Const, 2,
			wasm.OpcodeI32LtS,
			wasm.OpcodeIf, blockSignature_vv,
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeReturn,
			wasm.OpcodeEnd,
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeI32Const, 1,
			wasm.OpcodeI32Sub,
			wasm.OpcodeCall, 0,
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeI32Const, 2,
			wasm.OpcodeI32Sub,
			wasm.OpcodeCall, 0,
			wasm.OpcodeI32Add,
			wasm.OpcodeEnd,
		}, nil),
	}
	ImportedFunctionCall = TestCase{
		Name: "imported_function_call",
		Imported: &wasm.Module{
			ExportSection:   []wasm.Export{{Name: "i32_i32", Type: wasm.ExternTypeFunc}},
			TypeSection:     []wasm.FunctionType{i32i32_i32},
			FunctionSection: []wasm.Index{0},
			CodeSection: []wasm.Code{{Body: []byte{
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeI32Mul,
				wasm.OpcodeEnd,
			}}},
			NameSection: &wasm.NameSection{ModuleName: "env"},
		},
		Module: &wasm.Module{
			ImportFunctionCount: 1,
			TypeSection:         []wasm.FunctionType{i32_i32, i32i32_i32},
			ImportSection:       []wasm.Import{{Type: wasm.ExternTypeFunc, Module: "env", Name: "i32_i32", DescFunc: 1}},
			FunctionSection:     []wasm.Index{0},
			ExportSection: []wasm.Export{
				{Name: ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 1},
				{Name: "imported_exported", Type: wasm.ExternTypeFunc, Index: 0 /* imported */},
			},
			CodeSection: []wasm.Code{{Body: []byte{
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeCall, 0,
				wasm.OpcodeEnd,
			}}},
		},
	}

	MemoryStoreBasic = TestCase{
		Name: "memory_load_basic",
		Module: &wasm.Module{
			TypeSection:     []wasm.FunctionType{{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}}},
			ExportSection:   []wasm.Export{{Name: ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 0}},
			MemorySection:   &wasm.Memory{Min: 1},
			FunctionSection: []wasm.Index{0},
			CodeSection: []wasm.Code{{Body: []byte{
				wasm.OpcodeLocalGet, 0, // offset
				wasm.OpcodeLocalGet, 1, // value
				wasm.OpcodeI32Store, 0x2, 0x0, // alignment=2 (natural alignment) staticOffset=0
				// Read back.
				wasm.OpcodeLocalGet, 0, // offset
				wasm.OpcodeI32Load, 0x2, 0x0, // alignment=2 (natural alignment) staticOffset=0
				wasm.OpcodeEnd,
			}}},
		},
	}

	MemoryStores = TestCase{
		Name: "memory_load_basic",
		Module: &wasm.Module{
			TypeSection:     []wasm.FunctionType{{Params: []wasm.ValueType{i32, i64, f32, f64}}},
			ExportSection:   []wasm.Export{{Name: ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 0}},
			MemorySection:   &wasm.Memory{Min: 1},
			FunctionSection: []wasm.Index{0},
			CodeSection: []wasm.Code{{Body: []byte{
				wasm.OpcodeI32Const, 0, // offset
				wasm.OpcodeLocalGet, 0, // value
				wasm.OpcodeI32Store, 0x2, 0x0,

				wasm.OpcodeI32Const, 8, // offset
				wasm.OpcodeLocalGet, 1, // value
				wasm.OpcodeI64Store, 0x3, 0x0,

				wasm.OpcodeI32Const, 16, // offset
				wasm.OpcodeLocalGet, 2, // value
				wasm.OpcodeF32Store, 0x2, 0x0,

				wasm.OpcodeI32Const, 24, // offset
				wasm.OpcodeLocalGet, 3, // value
				wasm.OpcodeF64Store, 0x3, 0x0,

				wasm.OpcodeI32Const, 32,
				wasm.OpcodeLocalGet, 0, // value
				wasm.OpcodeI32Store8, 0x0, 0,

				wasm.OpcodeI32Const, 40,
				wasm.OpcodeLocalGet, 0, // value
				wasm.OpcodeI32Store16, 0x1, 0,

				wasm.OpcodeI32Const, 48,
				wasm.OpcodeLocalGet, 1, // value
				wasm.OpcodeI64Store8, 0x0, 0,

				wasm.OpcodeI32Const, 56,
				wasm.OpcodeLocalGet, 1, // value
				wasm.OpcodeI64Store16, 0x1, 0,

				wasm.OpcodeI32Const, 0xc0, 0, // 64 in leb128.
				wasm.OpcodeLocalGet, 1, // value
				wasm.OpcodeI64Store32, 0x2, 0,

				wasm.OpcodeEnd,
			}}},
		},
	}

	MemoryLoadBasic = TestCase{
		Name: "memory_load_basic",
		Module: &wasm.Module{
			TypeSection: []wasm.FunctionType{{
				Params:  []wasm.ValueType{i32},
				Results: []wasm.ValueType{i32},
			}},
			ExportSection:   []wasm.Export{{Name: ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 0}},
			MemorySection:   &wasm.Memory{Min: 1},
			FunctionSection: []wasm.Index{0},
			CodeSection: []wasm.Code{{Body: []byte{
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeI32Load, 0x2, 0x0, // alignment=2 (natural alignment) staticOffset=0
				wasm.OpcodeEnd,
			}}},
			DataSection: []wasm.DataSegment{{OffsetExpression: constExprI32(0), Init: maskedBuf(int(wasm.MemoryPageSize))}},
		},
	}

	MemorySizeGrow = TestCase{
		Name: "memory_size_grow",
		Module: &wasm.Module{
			TypeSection:     []wasm.FunctionType{{Results: []wasm.ValueType{i32, i32, i32}}},
			ExportSection:   []wasm.Export{{Name: ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 0}},
			MemorySection:   &wasm.Memory{Min: 1, Max: 2, IsMaxEncoded: true},
			FunctionSection: []wasm.Index{0},
			CodeSection: []wasm.Code{{Body: []byte{
				wasm.OpcodeI32Const, 1,
				wasm.OpcodeMemoryGrow, 0, // return 1.
				wasm.OpcodeMemorySize, 0, // return 2.
				wasm.OpcodeI32Const, 1,
				wasm.OpcodeMemoryGrow, 0, // return -1 since already maximum size.
				wasm.OpcodeEnd,
			}}},
		},
	}

	MemoryLoadBasic2 = TestCase{
		Name: "memory_load_basic2",
		Module: &wasm.Module{
			TypeSection:     []wasm.FunctionType{i32_i32, {}},
			ExportSection:   []wasm.Export{{Name: ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 0}},
			MemorySection:   &wasm.Memory{Min: 1},
			FunctionSection: []wasm.Index{0, 1},
			CodeSection: []wasm.Code{
				{Body: []byte{
					wasm.OpcodeLocalGet, 0,
					wasm.OpcodeI32Const, 0,
					wasm.OpcodeI32Eq,
					wasm.OpcodeIf, blockSignature_vv,
					wasm.OpcodeCall, 0x1, // After this the memory buf/size pointer reloads.
					wasm.OpcodeElse, // But in Else block, we do nothing, so not reloaded.
					wasm.OpcodeEnd,

					// Therefore, this block should reload the memory buf/size pointer here.
					wasm.OpcodeLocalGet, 0,
					wasm.OpcodeI32Load, 0x2, 0x0, // alignment=2 (natural alignment) staticOffset=0

					wasm.OpcodeEnd,
				}},
				{Body: []byte{wasm.OpcodeEnd}},
			},
			DataSection: []wasm.DataSegment{{OffsetExpression: constExprI32(0), Init: maskedBuf(int(wasm.MemoryPageSize))}},
		},
	}

	ImportedMemoryGrow = TestCase{
		Name: "imported_memory_grow",
		Imported: &wasm.Module{
			ExportSection: []wasm.Export{
				{Name: "mem", Type: wasm.ExternTypeMemory, Index: 0},
				{Name: "size", Type: wasm.ExternTypeFunc, Index: 0},
			},
			MemorySection:   &wasm.Memory{Min: 1},
			TypeSection:     []wasm.FunctionType{v_i32},
			FunctionSection: []wasm.Index{0},
			CodeSection:     []wasm.Code{{Body: []byte{wasm.OpcodeMemorySize, 0, wasm.OpcodeEnd}}},
			DataSection:     []wasm.DataSegment{{OffsetExpression: constExprI32(0), Init: maskedBuf(int(wasm.MemoryPageSize))}},
			NameSection:     &wasm.NameSection{ModuleName: "env"},
		},
		Module: &wasm.Module{
			ImportMemoryCount:   1,
			ImportFunctionCount: 1,
			ImportSection: []wasm.Import{
				{Module: "env", Name: "mem", Type: wasm.ExternTypeMemory, DescMem: &wasm.Memory{Min: 1}},
				{Module: "env", Name: "size", Type: wasm.ExternTypeFunc, DescFunc: 0},
			},
			TypeSection:     []wasm.FunctionType{v_i32, {Results: []wasm.ValueType{i32, i32, i32, i32}}},
			ExportSection:   []wasm.Export{{Name: ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 1}},
			FunctionSection: []wasm.Index{1},
			CodeSection: []wasm.Code{
				{Body: []byte{
					wasm.OpcodeCall, 0, // Call imported size function. --> 1
					wasm.OpcodeMemorySize, 0, // --> 1.
					wasm.OpcodeI32Const, 10,
					wasm.OpcodeMemoryGrow, 0,
					wasm.OpcodeDrop,
					wasm.OpcodeCall, 0, // Call imported size function. --> 11.
					wasm.OpcodeMemorySize, 0, // --> 11.
					wasm.OpcodeEnd,
				}},
			},
		},
	}

	GlobalsGet = TestCase{
		Name: "globals_get",
		Module: &wasm.Module{
			TypeSection:     []wasm.FunctionType{{Results: []wasm.ValueType{i32, i64, f32, f64, v128}}},
			ExportSection:   []wasm.Export{{Name: ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 0}},
			FunctionSection: []wasm.Index{0},
			GlobalSection: []wasm.Global{
				{
					Type: wasm.GlobalType{ValType: wasm.ValueTypeI32, Mutable: false},
					Init: constExprI32(math.MinInt32),
				},
				{
					Type: wasm.GlobalType{ValType: wasm.ValueTypeI64, Mutable: false},
					Init: constExprI64(math.MinInt64),
				},
				{
					Type: wasm.GlobalType{ValType: wasm.ValueTypeF32, Mutable: false},
					Init: constExprF32(math.MaxFloat32),
				},
				{
					Type: wasm.GlobalType{ValType: wasm.ValueTypeF64, Mutable: false},
					Init: constExprF64(math.MaxFloat64),
				},
				{
					Type: wasm.GlobalType{ValType: wasm.ValueTypeV128, Mutable: false},
					Init: constExprV128(1234, 5678),
				},
			},
			CodeSection: []wasm.Code{
				{Body: []byte{
					wasm.OpcodeGlobalGet, 0,
					wasm.OpcodeGlobalGet, 1,
					wasm.OpcodeGlobalGet, 2,
					wasm.OpcodeGlobalGet, 3,
					wasm.OpcodeGlobalGet, 4,
					wasm.OpcodeEnd,
				}},
			},
		},
	}

	GlobalsSet = TestCase{
		Name: "globals_set",
		Module: &wasm.Module{
			TypeSection:     []wasm.FunctionType{{Results: []wasm.ValueType{i32, i64, f32, f64, v128}}},
			ExportSection:   []wasm.Export{{Name: ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 0}},
			FunctionSection: []wasm.Index{0},
			GlobalSection: []wasm.Global{
				{
					Type: wasm.GlobalType{ValType: wasm.ValueTypeI32, Mutable: true},
					Init: constExprI32(0),
				},
				{
					Type: wasm.GlobalType{ValType: wasm.ValueTypeI64, Mutable: true},
					Init: constExprI64(0),
				},
				{
					Type: wasm.GlobalType{ValType: wasm.ValueTypeF32, Mutable: true},
					Init: constExprF32(0),
				},
				{
					Type: wasm.GlobalType{ValType: wasm.ValueTypeF64, Mutable: true},
					Init: constExprF64(0),
				},
				{
					Type: wasm.GlobalType{ValType: wasm.ValueTypeV128, Mutable: true},
					Init: constExprV128(0, 0),
				},
			},
			CodeSection: []wasm.Code{
				{Body: []byte{
					wasm.OpcodeI32Const, 1,
					wasm.OpcodeGlobalSet, 0,
					wasm.OpcodeGlobalGet, 0,
					wasm.OpcodeI64Const, 2,
					wasm.OpcodeGlobalSet, 1,
					wasm.OpcodeGlobalGet, 1,
					wasm.OpcodeF32Const, 0, 0, 64, 64, // 3.0
					wasm.OpcodeGlobalSet, 2,
					wasm.OpcodeGlobalGet, 2,
					wasm.OpcodeF64Const, 0, 0, 0, 0, 0, 0, 16, 64, // 4.0
					wasm.OpcodeGlobalSet, 3,
					wasm.OpcodeGlobalGet, 3,
					wasm.OpcodeVecPrefix, wasm.OpcodeVecV128Const,
					10, 0, 0, 0, 0, 0, 0, 0, 20, 0, 0, 0, 0, 0, 0, 0,
					wasm.OpcodeGlobalSet, 4,
					wasm.OpcodeGlobalGet, 4,
					wasm.OpcodeEnd,
				}},
			},
		},
	}

	GlobalsMutable = TestCase{
		Module: &wasm.Module{
			TypeSection: []wasm.FunctionType{
				{Results: []wasm.ValueType{i32, i64, f32, f64, i32, i64, f32, f64}},
				{},
			},
			ExportSection:   []wasm.Export{{Name: ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 0}},
			FunctionSection: []wasm.Index{0, 1},
			GlobalSection: []wasm.Global{
				{
					Type: wasm.GlobalType{ValType: wasm.ValueTypeI32, Mutable: true},
					Init: constExprI32(100),
				},
				{
					Type: wasm.GlobalType{ValType: wasm.ValueTypeI64, Mutable: true},
					Init: constExprI64(200),
				},
				{
					Type: wasm.GlobalType{ValType: wasm.ValueTypeF32, Mutable: true},
					Init: constExprF32(300.0),
				},
				{
					Type: wasm.GlobalType{ValType: wasm.ValueTypeF64, Mutable: true},
					Init: constExprF64(400.0),
				},
			},
			CodeSection: []wasm.Code{
				{Body: []byte{
					wasm.OpcodeGlobalGet, 0,
					wasm.OpcodeGlobalGet, 1,
					wasm.OpcodeGlobalGet, 2,
					wasm.OpcodeGlobalGet, 3,
					wasm.OpcodeCall, 1,
					wasm.OpcodeGlobalGet, 0,
					wasm.OpcodeGlobalGet, 1,
					wasm.OpcodeGlobalGet, 2,
					wasm.OpcodeGlobalGet, 3,
					wasm.OpcodeEnd,
				}},
				{Body: []byte{
					wasm.OpcodeI32Const, 1,
					wasm.OpcodeGlobalSet, 0,
					wasm.OpcodeI64Const, 2,
					wasm.OpcodeGlobalSet, 1,
					wasm.OpcodeF32Const, 0, 0, 64, 64, // 3.0
					wasm.OpcodeGlobalSet, 2,
					wasm.OpcodeF64Const, 0, 0, 0, 0, 0, 0, 16, 64, // 4.0
					wasm.OpcodeGlobalSet, 3,
					wasm.OpcodeReturn,
					wasm.OpcodeEnd,
				}},
			},
		},
	}

	MemoryLoads = TestCase{
		Name: "memory_loads",
		Module: &wasm.Module{
			TypeSection: []wasm.FunctionType{{
				Params: []wasm.ValueType{i32},
				Results: []wasm.ValueType{
					i32, i64, f32, f64, i32, i64, f32, f64,
					i32, i32, i32, i32, i32, i32, i32, i32,
					i64, i64, i64, i64, i64, i64, i64, i64, i64, i64, i64, i64,
				},
			}},
			ExportSection:   []wasm.Export{{Name: ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 0}},
			MemorySection:   &wasm.Memory{Min: 1},
			FunctionSection: []wasm.Index{0},
			CodeSection: []wasm.Code{{Body: []byte{
				// Basic loads (without extensions).
				wasm.OpcodeLocalGet, 0, // 0
				wasm.OpcodeI32Load, 0x2, 0x0, // alignment=2 (natural alignment) staticOffset=0
				wasm.OpcodeLocalGet, 0, // 1
				wasm.OpcodeI64Load, 0x3, 0x0, // alignment=3 (natural alignment) staticOffset=0
				wasm.OpcodeLocalGet, 0, // 2
				wasm.OpcodeF32Load, 0x2, 0x0, // alignment=2 (natural alignment) staticOffset=0
				wasm.OpcodeLocalGet, 0, // 3
				wasm.OpcodeF64Load, 0x3, 0x0, // alignment=3 (natural alignment) staticOffset=0
				wasm.OpcodeLocalGet, 0, // 4
				wasm.OpcodeI32Load, 0x2, 0xf, // alignment=2 (natural alignment) staticOffset=16
				wasm.OpcodeLocalGet, 0, // 5
				wasm.OpcodeI64Load, 0x3, 0xf, // alignment=3 (natural alignment) staticOffset=16
				wasm.OpcodeLocalGet, 0, // 6
				wasm.OpcodeF32Load, 0x2, 0xf, // alignment=2 (natural alignment) staticOffset=16
				wasm.OpcodeLocalGet, 0, // 7
				wasm.OpcodeF64Load, 0x3, 0xf, // alignment=3 (natural alignment) staticOffset=16

				// Extension integer loads.
				wasm.OpcodeLocalGet, 0, // 8
				wasm.OpcodeI32Load8S, 0x0, 0x0, // alignment=0 (natural alignment) staticOffset=0
				wasm.OpcodeLocalGet, 0, // 9
				wasm.OpcodeI32Load8S, 0x0, 0xf, // alignment=0 (natural alignment) staticOffset=16

				wasm.OpcodeLocalGet, 0, // 10
				wasm.OpcodeI32Load8U, 0x0, 0x0, // alignment=0 (natural alignment) staticOffset=0
				wasm.OpcodeLocalGet, 0, // 11
				wasm.OpcodeI32Load8U, 0x0, 0xf, // alignment=0 (natural alignment) staticOffset=16
				wasm.OpcodeLocalGet, 0, // 12
				wasm.OpcodeI32Load16S, 0x1, 0x0, // alignment=1 (natural alignment) staticOffset=0
				wasm.OpcodeLocalGet, 0, // 13
				wasm.OpcodeI32Load16S, 0x1, 0xf, // alignment=1 (natural alignment) staticOffset=16
				wasm.OpcodeLocalGet, 0, // 14
				wasm.OpcodeI32Load16U, 0x1, 0x0, // alignment=1 (natural alignment) staticOffset=0
				wasm.OpcodeLocalGet, 0, // 15
				wasm.OpcodeI32Load16U, 0x1, 0xf, // alignment=1 (natural alignment) staticOffset=16
				wasm.OpcodeLocalGet, 0, // 16
				wasm.OpcodeI64Load8S, 0x0, 0x0, // alignment=0 (natural alignment) staticOffset=0
				wasm.OpcodeLocalGet, 0, // 17
				wasm.OpcodeI64Load8S, 0x0, 0xf, // alignment=0 (natural alignment) staticOffset=16
				wasm.OpcodeLocalGet, 0, // 18
				wasm.OpcodeI64Load8U, 0x0, 0x0, // alignment=0 (natural alignment) staticOffset=0
				wasm.OpcodeLocalGet, 0, // 19

				wasm.OpcodeI64Load8U, 0x0, 0xf, // alignment=0 (natural alignment) staticOffset=16
				wasm.OpcodeLocalGet, 0, // 20
				wasm.OpcodeI64Load16S, 0x1, 0x0, // alignment=1 (natural alignment) staticOffset=0
				wasm.OpcodeLocalGet, 0, // 21
				wasm.OpcodeI64Load16S, 0x1, 0xf, // alignment=1 (natural alignment) staticOffset=16
				wasm.OpcodeLocalGet, 0, // 22
				wasm.OpcodeI64Load16U, 0x1, 0x0, // alignment=1 (natural alignment) staticOffset=0
				wasm.OpcodeLocalGet, 0, // 23
				wasm.OpcodeI64Load16U, 0x1, 0xf, // alignment=1 (natural alignment) staticOffset=16
				wasm.OpcodeLocalGet, 0, // 24
				wasm.OpcodeI64Load32S, 0x2, 0x0, // alignment=2 (natural alignment) staticOffset=0
				wasm.OpcodeLocalGet, 0, // 25
				wasm.OpcodeI64Load32S, 0x2, 0xf, // alignment=2 (natural alignment) staticOffset=16
				wasm.OpcodeLocalGet, 0, // 26
				wasm.OpcodeI64Load32U, 0x2, 0x0, // alignment=2 (natural alignment) staticOffset=0
				wasm.OpcodeLocalGet, 0, // 27
				wasm.OpcodeI64Load32U, 0x2, 0xf, // alignment=2 (natural alignment) staticOffset=16

				wasm.OpcodeEnd,
			}}},
			DataSection: []wasm.DataSegment{{OffsetExpression: constExprI32(0), Init: maskedBuf(int(wasm.MemoryPageSize))}},
		},
	}

	CallIndirect = TestCase{
		Module: &wasm.Module{
			TypeSection:     []wasm.FunctionType{i32_i32, {}, v_i32, v_i32i32},
			ExportSection:   []wasm.Export{{Name: ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 0}},
			FunctionSection: []wasm.Index{0, 1, 2, 3},
			TableSection:    []wasm.Table{{Type: wasm.RefTypeFuncref, Min: 1000}},
			ElementSection: []wasm.ElementSegment{
				{
					OffsetExpr: constExprI32(0), TableIndex: 0, Type: wasm.RefTypeFuncref, Mode: wasm.ElementModeActive,
					// Set the function 1, 2, 3 at the beginning of the table.
					Init: []wasm.Index{1, 2, 3},
				},
			},
			CodeSection: []wasm.Code{
				{Body: []byte{
					wasm.OpcodeLocalGet, 0,
					wasm.OpcodeCallIndirect, 2, 0, // Expecting type 2 (v_i32), in tables[0]
					wasm.OpcodeEnd,
				}},
				{Body: []byte{wasm.OpcodeEnd}},
				{Body: []byte{wasm.OpcodeI32Const, 10, wasm.OpcodeEnd}},
				{Body: []byte{wasm.OpcodeI32Const, 1, wasm.OpcodeI32Const, 1, wasm.OpcodeEnd}},
			},
		},
	}

	BrTable = TestCase{
		Module: &wasm.Module{
			TypeSection:     []wasm.FunctionType{i32_i32, {}},
			ExportSection:   []wasm.Export{{Name: ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 0}},
			FunctionSection: []wasm.Index{0},
			CodeSection: []wasm.Code{
				{Body: []byte{
					wasm.OpcodeBlock, 1, // Signature v_v,
					wasm.OpcodeBlock, 1, // Signature v_v,
					wasm.OpcodeBlock, 1, // Signature v_v,
					wasm.OpcodeBlock, 1, // Signature v_v,
					wasm.OpcodeBlock, 1, // Signature v_v,
					wasm.OpcodeBlock, 1, // Signature v_v,
					wasm.OpcodeLocalGet, 0,
					wasm.OpcodeBrTable,
					6,                // size of label vector
					0, 1, 2, 3, 4, 5, // labels.
					0, // default label
					wasm.OpcodeEnd, wasm.OpcodeI32Const, 11, wasm.OpcodeReturn,
					wasm.OpcodeEnd, wasm.OpcodeI32Const, 12, wasm.OpcodeReturn,
					wasm.OpcodeEnd, wasm.OpcodeI32Const, 13, wasm.OpcodeReturn,
					wasm.OpcodeEnd, wasm.OpcodeI32Const, 14, wasm.OpcodeReturn,
					wasm.OpcodeEnd, wasm.OpcodeI32Const, 15, wasm.OpcodeReturn,
					wasm.OpcodeEnd, wasm.OpcodeI32Const, 16, wasm.OpcodeReturn,
					wasm.OpcodeEnd,
				}},
			},
		},
	}

	BrTableWithArg = TestCase{
		Name: "br_table_with_arg",
		Module: &wasm.Module{
			TypeSection:     []wasm.FunctionType{i32i32_i32, v_i32},
			ExportSection:   []wasm.Export{{Name: ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 0}},
			FunctionSection: []wasm.Index{0},
			CodeSection: []wasm.Code{
				{Body: []byte{
					wasm.OpcodeBlock, 1, // Signature v_i32,
					wasm.OpcodeBlock, 1, // Signature v_i32,
					wasm.OpcodeBlock, 1, // Signature v_i32,
					wasm.OpcodeBlock, 1, // Signature v_i32,
					wasm.OpcodeBlock, 1, // Signature v_i32,
					wasm.OpcodeBlock, 1, // Signature v_i32,
					wasm.OpcodeLocalGet, 1, // Argument to each destination.
					wasm.OpcodeLocalGet, 0,
					wasm.OpcodeBrTable,
					6,                // size of label vector
					0, 1, 2, 3, 4, 5, // labels.
					0, // default label
					wasm.OpcodeEnd, wasm.OpcodeI32Const, 11, wasm.OpcodeI32Add, wasm.OpcodeReturn,
					wasm.OpcodeEnd, wasm.OpcodeI32Const, 12, wasm.OpcodeI32Add, wasm.OpcodeReturn,
					wasm.OpcodeEnd, wasm.OpcodeI32Const, 13, wasm.OpcodeI32Add, wasm.OpcodeReturn,
					wasm.OpcodeEnd, wasm.OpcodeI32Const, 14, wasm.OpcodeI32Add, wasm.OpcodeReturn,
					wasm.OpcodeEnd, wasm.OpcodeI32Const, 15, wasm.OpcodeI32Add, wasm.OpcodeReturn,
					wasm.OpcodeEnd, wasm.OpcodeI32Const, 16, wasm.OpcodeI32Add, wasm.OpcodeReturn,
					wasm.OpcodeUnreachable,
					wasm.OpcodeEnd,
				}},
			},
		},
	}

	IfThenEndNestingUnreachableIfThenElseEnd = TestCase{
		// This has been detected by fuzzing. This should belong to internal/integration_test/fuzzcases eventually,
		// but for now, wazevo should have its own cases under engine/wazevo.
		Name: "if_then_end_nesting_unreachable_if_then_else_end",
		// (module
		//  (type (;0;) (func (param f64 f64 f64)))
		//  (func (;0;) (type 0) (param f64 f64 f64)
		//    block (result i64) ;; label = @1
		//      memory.size
		//      if ;; label = @2
		//        memory.size
		//        br 0 (;@2;)
		//        if ;; label = @3
		//        else
		//        end
		//        drop
		//      end
		//      i64.const 0
		//    end
		//    drop
		//  )
		//  (memory (;0;) 4554)
		//)
		Module: &wasm.Module{
			TypeSection: []wasm.FunctionType{
				{Params: []wasm.ValueType{f64, f64, f64}},
				{Results: []wasm.ValueType{i64}},
			},

			ExportSection:   []wasm.Export{{Name: ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 0}},
			MemorySection:   &wasm.Memory{Min: 4554},
			FunctionSection: []wasm.Index{0},
			CodeSection: []wasm.Code{{Body: []byte{
				wasm.OpcodeBlock, 1, // Signature v_i64,
				wasm.OpcodeMemorySize, 0,
				wasm.OpcodeIf, blockSignature_vv,
				wasm.OpcodeMemorySize, 0,
				wasm.OpcodeBr, 0x0, // label=0
				wasm.OpcodeIf, blockSignature_vv,
				wasm.OpcodeElse,
				wasm.OpcodeEnd,
				wasm.OpcodeDrop,
				wasm.OpcodeEnd,
				wasm.OpcodeI64Const, 0,
				wasm.OpcodeEnd,
				wasm.OpcodeDrop,
				wasm.OpcodeEnd,
			}}},
		},
	}

	VecBitSelect = TestCase{
		Name: "vector_bit_select",
		Module: &wasm.Module{
			TypeSection:     []wasm.FunctionType{{Params: []wasm.ValueType{v128, v128, v128}, Results: []wasm.ValueType{v128, v128}}},
			ExportSection:   []wasm.Export{{Name: ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 0}},
			FunctionSection: []wasm.Index{0},
			CodeSection: []wasm.Code{{Body: []byte{
				// In arm64, the "C" of BSL instruction is overwritten by the result,
				// so this case can be used to ensure that it is still alive if 'c' is used somewhere else,
				// which in this case is as a return value.
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeLocalGet, 2,
				wasm.OpcodeVecPrefix, wasm.OpcodeVecV128Bitselect,
				wasm.OpcodeLocalGet, 2, // Returns the 'c' as-is.
				wasm.OpcodeEnd,
			}}},
		},
	}

	VecShuffle = TestCase{
		Name:   "shuffle",
		Module: VecShuffleWithLane(0, 1, 2, 3, 4, 5, 6, 7, 24, 25, 26, 27, 28, 29, 30, 31),
	}

	MemoryWait32 = TestCase{
		Name: "memory_wait32",
		Module: &wasm.Module{
			TypeSection:     []wasm.FunctionType{{Params: []wasm.ValueType{i32, i32, i64}, Results: []wasm.ValueType{i32}}},
			ExportSection:   []wasm.Export{{Name: ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 0}},
			MemorySection:   &wasm.Memory{Min: 1, Max: 1, IsMaxEncoded: true, IsShared: true},
			FunctionSection: []wasm.Index{0},
			CodeSection: []wasm.Code{{Body: []byte{
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeLocalGet, 2,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicMemoryWait32, 0x1, 8,
				wasm.OpcodeEnd,
			}}},
		},
	}

	MemoryWait64 = TestCase{
		Name: "memory_wait64",
		Module: &wasm.Module{
			TypeSection:     []wasm.FunctionType{{Params: []wasm.ValueType{i32, i64, i64}, Results: []wasm.ValueType{i32}}},
			ExportSection:   []wasm.Export{{Name: ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 0}},
			MemorySection:   &wasm.Memory{Min: 1, Max: 1, IsMaxEncoded: true, IsShared: true},
			FunctionSection: []wasm.Index{0},
			CodeSection: []wasm.Code{{Body: []byte{
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeLocalGet, 2,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicMemoryWait64, 0x2, 8,
				wasm.OpcodeEnd,
			}}},
		},
	}

	MemoryNotify = TestCase{
		Name: "memory_notify",
		Module: &wasm.Module{
			TypeSection:     []wasm.FunctionType{{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}}},
			ExportSection:   []wasm.Export{{Name: ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 0}},
			MemorySection:   &wasm.Memory{Min: 1, Max: 1, IsMaxEncoded: true, IsShared: true},
			FunctionSection: []wasm.Index{0},
			CodeSection: []wasm.Code{{Body: []byte{
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicMemoryNotify, 0x1, 8,
				wasm.OpcodeEnd,
			}}},
		},
	}

	AtomicRmwAdd = TestCase{
		Name: "atomic_rmw_add",
		Module: &wasm.Module{
			TypeSection: []wasm.FunctionType{{
				Params:  []wasm.ValueType{i32, i32, i32, i64, i64, i64, i64},
				Results: []wasm.ValueType{i32, i32, i32, i64, i64, i64, i64},
			}},
			ExportSection:   []wasm.Export{{Name: ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 0}},
			MemorySection:   &wasm.Memory{Min: 1, Max: 1, IsMaxEncoded: true, IsShared: true},
			FunctionSection: []wasm.Index{0},
			CodeSection: []wasm.Code{{Body: []byte{
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw8AddU, 0x0, 0,
				wasm.OpcodeI32Const, 8,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw16AddU, 0x1, 0,
				wasm.OpcodeI32Const, 16,
				wasm.OpcodeLocalGet, 2,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32RmwAdd, 0x2, 0,
				wasm.OpcodeI32Const, 24,
				wasm.OpcodeLocalGet, 3,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw8AddU, 0x0, 0,
				wasm.OpcodeI32Const, 32,
				wasm.OpcodeLocalGet, 4,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw16AddU, 0x1, 0,
				wasm.OpcodeI32Const, 40,
				wasm.OpcodeLocalGet, 5,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw32AddU, 0x2, 0,
				wasm.OpcodeI32Const, 48,
				wasm.OpcodeLocalGet, 6,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64RmwAdd, 0x3, 0,
				wasm.OpcodeEnd,
			}}},
		},
	}

	AtomicRmwSub = TestCase{
		Name: "atomic_rmw_sub",
		Module: &wasm.Module{
			TypeSection: []wasm.FunctionType{{
				Params:  []wasm.ValueType{i32, i32, i32, i64, i64, i64, i64},
				Results: []wasm.ValueType{i32, i32, i32, i64, i64, i64, i64},
			}},
			ExportSection:   []wasm.Export{{Name: ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 0}},
			MemorySection:   &wasm.Memory{Min: 1, Max: 1, IsMaxEncoded: true, IsShared: true},
			FunctionSection: []wasm.Index{0},
			CodeSection: []wasm.Code{{Body: []byte{
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw8SubU, 0x0, 0,
				wasm.OpcodeI32Const, 8,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw16SubU, 0x1, 0,
				wasm.OpcodeI32Const, 16,
				wasm.OpcodeLocalGet, 2,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32RmwSub, 0x2, 0,
				wasm.OpcodeI32Const, 24,
				wasm.OpcodeLocalGet, 3,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw8SubU, 0x0, 0,
				wasm.OpcodeI32Const, 32,
				wasm.OpcodeLocalGet, 4,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw16SubU, 0x1, 0,
				wasm.OpcodeI32Const, 40,
				wasm.OpcodeLocalGet, 5,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw32SubU, 0x2, 0,
				wasm.OpcodeI32Const, 48,
				wasm.OpcodeLocalGet, 6,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64RmwSub, 0x3, 0,
				wasm.OpcodeEnd,
			}}},
		},
	}

	AtomicRmwAnd = TestCase{
		Name: "atomic_rmw_and",
		Module: &wasm.Module{
			TypeSection: []wasm.FunctionType{{
				Params:  []wasm.ValueType{i32, i32, i32, i64, i64, i64, i64},
				Results: []wasm.ValueType{i32, i32, i32, i64, i64, i64, i64},
			}},
			ExportSection:   []wasm.Export{{Name: ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 0}},
			MemorySection:   &wasm.Memory{Min: 1, Max: 1, IsMaxEncoded: true, IsShared: true},
			FunctionSection: []wasm.Index{0},
			CodeSection: []wasm.Code{{Body: []byte{
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw8AndU, 0x0, 0,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw16AndU, 0x1, 8,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 2,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32RmwAnd, 0x2, 16,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 3,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw8AndU, 0x0, 24,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 4,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw16AndU, 0x1, 32,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 5,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw32AndU, 0x2, 40,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 6,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64RmwAnd, 0x3, 48,
				wasm.OpcodeEnd,
			}}},
		},
	}

	AtomicRmwOr = TestCase{
		Name: "atomic_rmw_or",
		Module: &wasm.Module{
			TypeSection: []wasm.FunctionType{{
				Params:  []wasm.ValueType{i32, i32, i32, i64, i64, i64, i64},
				Results: []wasm.ValueType{i32, i32, i32, i64, i64, i64, i64},
			}},
			ExportSection:   []wasm.Export{{Name: ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 0}},
			MemorySection:   &wasm.Memory{Min: 1, Max: 1, IsMaxEncoded: true, IsShared: true},
			FunctionSection: []wasm.Index{0},
			CodeSection: []wasm.Code{{Body: []byte{
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw8OrU, 0x0, 0,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw16OrU, 0x1, 8,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 2,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32RmwOr, 0x2, 16,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 3,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw8OrU, 0x0, 24,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 4,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw16OrU, 0x1, 32,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 5,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw32OrU, 0x2, 40,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 6,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64RmwOr, 0x3, 48,
				wasm.OpcodeEnd,
			}}},
		},
	}

	AtomicRmwXor = TestCase{
		Name: "atomic_rmw_xor",
		Module: &wasm.Module{
			TypeSection: []wasm.FunctionType{{
				Params:  []wasm.ValueType{i32, i32, i32, i64, i64, i64, i64},
				Results: []wasm.ValueType{i32, i32, i32, i64, i64, i64, i64},
			}},
			ExportSection:   []wasm.Export{{Name: ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 0}},
			MemorySection:   &wasm.Memory{Min: 1, Max: 1, IsMaxEncoded: true, IsShared: true},
			FunctionSection: []wasm.Index{0},
			CodeSection: []wasm.Code{{Body: []byte{
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw8XorU, 0x0, 0,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw16XorU, 0x1, 8,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 2,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32RmwXor, 0x2, 16,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 3,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw8XorU, 0x0, 24,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 4,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw16XorU, 0x1, 32,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 5,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw32XorU, 0x2, 40,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 6,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64RmwXor, 0x3, 48,
				wasm.OpcodeEnd,
			}}},
		},
	}

	AtomicRmwXchg = TestCase{
		Name: "atomic_rmw_xchg",
		Module: &wasm.Module{
			TypeSection: []wasm.FunctionType{{
				Params:  []wasm.ValueType{i32, i32, i32, i64, i64, i64, i64},
				Results: []wasm.ValueType{i32, i32, i32, i64, i64, i64, i64},
			}},
			ExportSection:   []wasm.Export{{Name: ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 0}},
			MemorySection:   &wasm.Memory{Min: 1, Max: 1, IsMaxEncoded: true, IsShared: true},
			FunctionSection: []wasm.Index{0},
			CodeSection: []wasm.Code{{Body: []byte{
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw8XchgU, 0x0, 0,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw16XchgU, 0x1, 8,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 2,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32RmwXchg, 0x2, 16,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 3,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw8XchgU, 0x0, 24,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 4,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw16XchgU, 0x1, 32,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 5,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw32XchgU, 0x2, 40,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 6,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64RmwXchg, 0x3, 48,
				wasm.OpcodeEnd,
			}}},
		},
	}

	AtomicStoreLoad = TestCase{
		Name: "atomic_store_load",
		Module: &wasm.Module{
			TypeSection: []wasm.FunctionType{{
				Params:  []wasm.ValueType{i32, i32, i32, i64, i64, i64, i64},
				Results: []wasm.ValueType{i32, i32, i32, i64, i64, i64, i64},
			}},
			ExportSection:   []wasm.Export{{Name: ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 0}},
			MemorySection:   &wasm.Memory{Min: 1, Max: 1, IsMaxEncoded: true, IsShared: true},
			FunctionSection: []wasm.Index{0},
			CodeSection: []wasm.Code{{Body: []byte{
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Store8, 0x0, 0,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Load8U, 0x0, 0,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Store16, 0x1, 8,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Load16U, 0x1, 8,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 2,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Store, 0x2, 16,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Load, 0x2, 16,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 3,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Store8, 0x0, 24,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Load8U, 0x0, 24,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 4,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Store16, 0x1, 32,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Load16U, 0x1, 32,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 5,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Store32, 0x2, 40,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Load32U, 0x2, 40,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 6,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Store, 0x3, 48,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Load, 0x3, 48,
				wasm.OpcodeEnd,
			}}},
		},
	}

	AtomicCas = TestCase{
		Name: "atomic_cas",
		Module: &wasm.Module{
			TypeSection: []wasm.FunctionType{{
				Params:  []wasm.ValueType{i32, i32, i32, i32, i32, i32, i64, i64, i64, i64, i64, i64, i64, i64},
				Results: []wasm.ValueType{i32, i32, i32, i64, i64, i64, i64},
			}},
			ExportSection:   []wasm.Export{{Name: ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 0}},
			MemorySection:   &wasm.Memory{Min: 1, Max: 1, IsMaxEncoded: true, IsShared: true},
			FunctionSection: []wasm.Index{0},
			CodeSection: []wasm.Code{{Body: []byte{
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw8CmpxchgU, 0x0, 0,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 2,
				wasm.OpcodeLocalGet, 3,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw16CmpxchgU, 0x1, 8,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 4,
				wasm.OpcodeLocalGet, 5,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32RmwCmpxchg, 0x2, 16,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 6,
				wasm.OpcodeLocalGet, 7,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw8CmpxchgU, 0x0, 24,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 8,
				wasm.OpcodeLocalGet, 9,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw16CmpxchgU, 0x1, 32,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 10,
				wasm.OpcodeLocalGet, 11,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw32CmpxchgU, 0x2, 40,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 12,
				wasm.OpcodeLocalGet, 13,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64RmwCmpxchg, 0x3, 48,
				wasm.OpcodeEnd,
			}}},
		},
	}

	AtomicCasConst0 = TestCase{
		Name: "atomic_cas_const0",
		Module: &wasm.Module{
			TypeSection: []wasm.FunctionType{{
				Params:  []wasm.ValueType{i32, i32, i32, i64, i64, i64, i64},
				Results: []wasm.ValueType{i32, i32, i32, i64, i64, i64, i64},
			}},
			ExportSection:   []wasm.Export{{Name: ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 0}},
			MemorySection:   &wasm.Memory{Min: 1, Max: 1, IsMaxEncoded: true, IsShared: true},
			FunctionSection: []wasm.Index{0},
			CodeSection: []wasm.Code{{Body: []byte{
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw8CmpxchgU, 0x0, 0,
				wasm.OpcodeI32Const, 8,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32Rmw16CmpxchgU, 0x1, 0,
				wasm.OpcodeI32Const, 16,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeLocalGet, 2,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI32RmwCmpxchg, 0x2, 0,
				wasm.OpcodeI32Const, 24,
				wasm.OpcodeI64Const, 0,
				wasm.OpcodeLocalGet, 3,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw8CmpxchgU, 0x0, 0,
				wasm.OpcodeI32Const, 32,
				wasm.OpcodeI64Const, 0,
				wasm.OpcodeLocalGet, 4,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw16CmpxchgU, 0x1, 0,
				wasm.OpcodeI32Const, 40,
				wasm.OpcodeI64Const, 0,
				wasm.OpcodeLocalGet, 5,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64Rmw32CmpxchgU, 0x2, 0,
				wasm.OpcodeI32Const, 48,
				wasm.OpcodeI64Const, 0,
				wasm.OpcodeLocalGet, 6,
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicI64RmwCmpxchg, 0x3, 0,
				wasm.OpcodeEnd,
			}}},
		},
	}

	AtomicFence = TestCase{
		Name: "atomic_fence",
		Module: &wasm.Module{
			TypeSection: []wasm.FunctionType{{
				Params:  []wasm.ValueType{},
				Results: []wasm.ValueType{},
			}},
			ExportSection:   []wasm.Export{{Name: ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 0}},
			MemorySection:   &wasm.Memory{Min: 1, Max: 1, IsMaxEncoded: true, IsShared: true},
			FunctionSection: []wasm.Index{0},
			CodeSection: []wasm.Code{{Body: []byte{
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicFence, 0,
				wasm.OpcodeEnd,
			}}},
		},
	}

	AtomicFenceNoMemory = TestCase{
		Name: "atomic_fence",
		Module: &wasm.Module{
			TypeSection: []wasm.FunctionType{{
				Params:  []wasm.ValueType{},
				Results: []wasm.ValueType{},
			}},
			ExportSection:   []wasm.Export{{Name: ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 0}},
			FunctionSection: []wasm.Index{0},
			CodeSection: []wasm.Code{{Body: []byte{
				wasm.OpcodeAtomicPrefix, wasm.OpcodeAtomicFence, 0,
				wasm.OpcodeEnd,
			}}},
		},
	}

	IcmpAndZero = TestCase{
		Name: "icmp_and_zero",
		Module: &wasm.Module{
			TypeSection:     []wasm.FunctionType{{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}}},
			ExportSection:   []wasm.Export{{Name: ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 0}},
			FunctionSection: []wasm.Index{0},
			CodeSection: []wasm.Code{{Body: []byte{
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeI32And,
				wasm.OpcodeI32Eqz,
				wasm.OpcodeIf, blockSignature_vv,
				wasm.OpcodeI32Const, 1,
				wasm.OpcodeReturn,
				wasm.OpcodeElse,
				wasm.OpcodeI32Const, 0,
				wasm.OpcodeReturn,
				wasm.OpcodeEnd,
				wasm.OpcodeUnreachable,
				wasm.OpcodeEnd,
			}}},
		},
	}
)

// VecShuffleWithLane returns a VecShuffle test with a custom 16-bytes immediate (lane indexes).
func VecShuffleWithLane(lane ...byte) *wasm.Module {
	return &wasm.Module{
		TypeSection:     []wasm.FunctionType{{Params: []wasm.ValueType{v128, v128}, Results: []wasm.ValueType{v128}}},
		ExportSection:   []wasm.Export{{Name: ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 0}},
		FunctionSection: []wasm.Index{0},
		CodeSection: []wasm.Code{{
			Body: append(append([]byte{
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeLocalGet, 1,
				wasm.OpcodeVecPrefix, wasm.OpcodeVecV128i8x16Shuffle,
			}, lane...),
				wasm.OpcodeEnd),
		}},
	}
}

type TestCase struct {
	Name             string
	Imported, Module *wasm.Module
}

func SingleFunctionModule(typ wasm.FunctionType, body []byte, localTypes []wasm.ValueType) *wasm.Module {
	return &wasm.Module{
		TypeSection:     []wasm.FunctionType{typ},
		FunctionSection: []wasm.Index{0},
		CodeSection: []wasm.Code{{
			LocalTypes: localTypes,
			Body:       body,
		}},
		ExportSection: []wasm.Export{{Name: ExportedFunctionName, Type: wasm.ExternTypeFunc, Index: 0}},
	}
}

var (
	vv                              = wasm.FunctionType{}
	v_i32                           = wasm.FunctionType{Results: []wasm.ValueType{i32}}
	v_i32i32                        = wasm.FunctionType{Results: []wasm.ValueType{i32, i32}}
	i32_v                           = wasm.FunctionType{Params: []wasm.ValueType{i32}}
	i32_i32                         = wasm.FunctionType{Params: []wasm.ValueType{i32}, Results: []wasm.ValueType{i32}}
	i32i32_i32                      = wasm.FunctionType{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}}
	i32i32_i32i32                   = wasm.FunctionType{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32, i32}}
	i32i32i64i64f32f32f64f64_i32i64 = wasm.FunctionType{Params: []wasm.ValueType{i32, i32, i64, i64, f32, f32, f64, f64}, Results: []wasm.ValueType{i32, i64, f32, f64}}
	i32_i32i32                      = wasm.FunctionType{Params: []wasm.ValueType{i32}, Results: []wasm.ValueType{i32, i32}}
	i32f32f64_v                     = wasm.FunctionType{Params: []wasm.ValueType{i32, f32, f64}, Results: nil}
	i64f32f64_i64f32f64             = wasm.FunctionType{Params: []wasm.ValueType{i64, f32, f64}, Results: []wasm.ValueType{i64, f32, f64}}
)

const (
	i32  = wasm.ValueTypeI32
	i64  = wasm.ValueTypeI64
	f32  = wasm.ValueTypeF32
	f64  = wasm.ValueTypeF64
	v128 = wasm.ValueTypeV128

	blockSignature_vv = 0x40 // 0x40 is the v_v signature in 33-bit signed. See wasm.DecodeBlockType.
)

func maskedBuf(size int) []byte {
	ret := make([]byte, size)
	for i := range ret {
		ret[i] = byte(i)
	}
	return ret
}

func constExprI32(i int32) wasm.ConstantExpression {
	return wasm.ConstantExpression{
		Opcode: wasm.OpcodeI32Const,
		Data:   leb128.EncodeInt32(i),
	}
}

func constExprI64(i int64) wasm.ConstantExpression {
	return wasm.ConstantExpression{
		Opcode: wasm.OpcodeI64Const,
		Data:   leb128.EncodeInt64(i),
	}
}

func constExprF32(i float32) wasm.ConstantExpression {
	b := math.Float32bits(i)
	return wasm.ConstantExpression{
		Opcode: wasm.OpcodeF32Const,
		Data:   []byte{byte(b), byte(b >> 8), byte(b >> 16), byte(b >> 24)},
	}
}

func constExprF64(i float64) wasm.ConstantExpression {
	b := math.Float64bits(i)
	return wasm.ConstantExpression{
		Opcode: wasm.OpcodeF64Const,
		Data: []byte{
			byte(b), byte(b >> 8), byte(b >> 16), byte(b >> 24),
			byte(b >> 32), byte(b >> 40), byte(b >> 48), byte(b >> 56),
		},
	}
}

func constExprV128(lo, hi uint64) wasm.ConstantExpression {
	return wasm.ConstantExpression{
		Opcode: wasm.OpcodeVecV128Const,
		Data: []byte{
			byte(lo), byte(lo >> 8), byte(lo >> 16), byte(lo >> 24),
			byte(lo >> 32), byte(lo >> 40), byte(lo >> 48), byte(lo >> 56),
			byte(hi), byte(hi >> 8), byte(hi >> 16), byte(hi >> 24),
			byte(hi >> 32), byte(hi >> 40), byte(hi >> 48), byte(hi >> 56),
		},
	}
}
