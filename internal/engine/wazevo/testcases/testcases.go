package testcases

import (
	"math"

	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/wasm"
)

const ExportName = "f"

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
	Unreachable        = TestCase{Name: "unreachable", Module: SingleFunctionModule(vv, []byte{wasm.OpcodeUnreachable, wasm.OpcodeEnd}, nil)}
	OnlyReturn         = TestCase{Name: "only_return", Module: SingleFunctionModule(vv, []byte{wasm.OpcodeReturn, wasm.OpcodeEnd}, nil)}
	Params             = TestCase{Name: "params", Module: SingleFunctionModule(i32f32f64_v, []byte{wasm.OpcodeReturn, wasm.OpcodeEnd}, nil)}
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
			ExportSection: []wasm.Export{{Name: ExportName, Index: 0, Type: wasm.ExternTypeFunc}},
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
			Results: []wasm.ValueType{i32, i32, i64, i64},
		}, []byte{
			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeI32Clz,

			wasm.OpcodeLocalGet, 0,
			wasm.OpcodeI32Ctz,

			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeI64Clz,

			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeI64Ctz,

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
			TypeSection:     []wasm.FunctionType{i32_i32},
			FunctionSection: []wasm.Index{0},
			CodeSection: []wasm.Code{{Body: []byte{
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeLocalGet, 0,
				wasm.OpcodeI32Mul,
				wasm.OpcodeEnd,
			}}},
			NameSection: &wasm.NameSection{ModuleName: "env"},
		},
		Module: &wasm.Module{
			ImportFunctionCount: 1,
			TypeSection:         []wasm.FunctionType{i32_i32},
			ImportSection:       []wasm.Import{{Type: wasm.ExternTypeFunc, Module: "env", Name: "i32_i32"}},
			FunctionSection:     []wasm.Index{0},
			ExportSection:       []wasm.Export{{Name: ExportName, Type: wasm.ExternTypeFunc, Index: 1}},
			CodeSection: []wasm.Code{{Body: []byte{
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
			ExportSection:   []wasm.Export{{Name: ExportName, Type: wasm.ExternTypeFunc, Index: 0}},
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
			ExportSection:   []wasm.Export{{Name: ExportName, Type: wasm.ExternTypeFunc, Index: 0}},
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
			ExportSection:   []wasm.Export{{Name: ExportName, Type: wasm.ExternTypeFunc, Index: 0}},
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
			ExportSection:   []wasm.Export{{Name: ExportName, Type: wasm.ExternTypeFunc, Index: 0}},
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
			ExportSection:   []wasm.Export{{Name: ExportName, Type: wasm.ExternTypeFunc, Index: 0}},
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
			ExportSection:   []wasm.Export{{Name: ExportName, Type: wasm.ExternTypeFunc, Index: 1}},
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
			TypeSection:     []wasm.FunctionType{{Results: []wasm.ValueType{i32, i64, f32, f64}}},
			ExportSection:   []wasm.Export{{Name: ExportName, Type: wasm.ExternTypeFunc, Index: 0}},
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
			},
			CodeSection: []wasm.Code{
				{Body: []byte{
					wasm.OpcodeGlobalGet, 0,
					wasm.OpcodeGlobalGet, 1,
					wasm.OpcodeGlobalGet, 2,
					wasm.OpcodeGlobalGet, 3,
					wasm.OpcodeEnd,
				}},
			},
		},
	}

	GlobalsSet = TestCase{
		Name: "globals_get",
		Module: &wasm.Module{
			TypeSection:     []wasm.FunctionType{{Results: []wasm.ValueType{i32, i64, f32, f64}}},
			ExportSection:   []wasm.Export{{Name: ExportName, Type: wasm.ExternTypeFunc, Index: 0}},
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
			ExportSection:   []wasm.Export{{Name: ExportName, Type: wasm.ExternTypeFunc, Index: 0}},
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
			ExportSection:   []wasm.Export{{Name: ExportName, Type: wasm.ExternTypeFunc, Index: 0}},
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
			ExportSection:   []wasm.Export{{Name: ExportName, Type: wasm.ExternTypeFunc, Index: 0}},
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
)

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
		ExportSection: []wasm.Export{{Name: ExportName, Type: wasm.ExternTypeFunc, Index: 0}},
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
	i32 = wasm.ValueTypeI32
	i64 = wasm.ValueTypeI64
	f32 = wasm.ValueTypeF32
	f64 = wasm.ValueTypeF64

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
