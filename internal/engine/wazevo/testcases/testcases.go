package testcases

import (
	"math"

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
	SwapParamAndReturn = TestCase{
		Name: "swap_param_and_return",
		Module: SingleFunctionModule(i32i32_i32i32, []byte{
			wasm.OpcodeLocalGet, 1,
			wasm.OpcodeLocalGet, 0,
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
)

type TestCase struct {
	Name   string
	Module *wasm.Module
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
	vv                  = wasm.FunctionType{}
	v_i32               = wasm.FunctionType{Results: []wasm.ValueType{i32}}
	v_i32i32            = wasm.FunctionType{Results: []wasm.ValueType{i32, i32}}
	i32_v               = wasm.FunctionType{Params: []wasm.ValueType{i32}}
	i32_i32             = wasm.FunctionType{Params: []wasm.ValueType{i32}, Results: []wasm.ValueType{i32}}
	i32i32_i32          = wasm.FunctionType{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32}}
	i32i32_i32i32       = wasm.FunctionType{Params: []wasm.ValueType{i32, i32}, Results: []wasm.ValueType{i32, i32}}
	i32_i32i32          = wasm.FunctionType{Params: []wasm.ValueType{i32}, Results: []wasm.ValueType{i32, i32}}
	i32f32f64_v         = wasm.FunctionType{Params: []wasm.ValueType{i32, f32, f64}, Results: nil}
	i64f32f64_i64f32f64 = wasm.FunctionType{Params: []wasm.ValueType{i64, f32, f64}, Results: []wasm.ValueType{i64, f32, f64}}
)

const (
	i32 = wasm.ValueTypeI32
	i64 = wasm.ValueTypeI64
	f32 = wasm.ValueTypeF32
	f64 = wasm.ValueTypeF64

	blockSignature_vv = 0x40 // 0x40 is the v_v signature in 33-bit signed. See wasm.DecodeBlockType.
)
