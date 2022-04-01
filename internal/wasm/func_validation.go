package wasm

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/tetratelabs/wazero/internal/leb128"
)

// validateFunction validates the instruction sequence of a function.
// following the specification https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#instructions%E2%91%A2.
//
// f is the validation target function instance.
// functions is the list of function indexes which are declared on a module from which the target is instantiated.
// globals is the list of global types which are declared on a module from which the target is instantiated.
// memories is the list of memory types which are declared on a module from which the target is instantiated.
// tables is the list of table types which are declared on a module from which the target is instantiated.
// types is the list of function types which are declared on a module from which the target is instantiated.
// maxStackValues is the maximum height of values stack which the target is allowed to reach.
//
// Returns an error if the instruction sequence is not valid,
// or potentially it can exceed the maximum number of values on the stack.
func validateFunction(
	enabledFeatures Features,
	functionType *FunctionType,
	body []byte,
	localTypes []ValueType,
	functions []Index,
	globals []*GlobalType,
	memory *Memory,
	table *Table,
	types []*FunctionType,
	maxStackValues int,
) error {
	// We start with the outermost control block which is for function return if the code branches into it.
	controlBlockStack := []*controlBlock{{blockType: functionType}}
	// Create the valueTypeStack to track the state of Wasm value stacks at anypoint of execution.
	valueTypeStack := &valueTypeStack{}

	// Now start walking through all the instructions in the body while tracking
	// control blocks and value types to check the validity of all instructions.
	for pc := uint64(0); pc < uint64(len(body)); pc++ {
		op := body[pc]
		if OpcodeI32Load <= op && op <= OpcodeI64Store32 {
			if memory == nil {
				return fmt.Errorf("unknown memory access")
			}
			pc++
			align, num, err := leb128.DecodeUint32(bytes.NewReader(body[pc:]))
			if err != nil {
				return fmt.Errorf("read memory align: %v", err)
			}
			switch op {
			case OpcodeI32Load:
				if 1<<align > 32/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeF32Load:
				if 1<<align > 32/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				valueTypeStack.push(ValueTypeF32)
			case OpcodeI32Store:
				if 1<<align > 32/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
			case OpcodeF32Store:
				if 1<<align > 32/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return err
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
			case OpcodeI64Load:
				if 1<<align > 64/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				valueTypeStack.push(ValueTypeI64)
			case OpcodeF64Load:
				if 1<<align > 64/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				valueTypeStack.push(ValueTypeF64)
			case OpcodeI64Store:
				if 1<<align > 64/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return err
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
			case OpcodeF64Store:
				if 1<<align > 64/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return err
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
			case OpcodeI32Load8S:
				if 1<<align > 1 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI32Load8U:
				if 1<<align > 1 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI64Load8S, OpcodeI64Load8U:
				if 1<<align > 1 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				valueTypeStack.push(ValueTypeI64)
			case OpcodeI32Store8:
				if 1<<align > 1 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
			case OpcodeI64Store8:
				if 1<<align > 1 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return err
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
			case OpcodeI32Load16S, OpcodeI32Load16U:
				if 1<<align > 16/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI64Load16S, OpcodeI64Load16U:
				if 1<<align > 16/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				valueTypeStack.push(ValueTypeI64)
			case OpcodeI32Store16:
				if 1<<align > 16/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
			case OpcodeI64Store16:
				if 1<<align > 16/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return err
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
			case OpcodeI64Load32S, OpcodeI64Load32U:
				if 1<<align > 32/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				valueTypeStack.push(ValueTypeI64)
			case OpcodeI64Store32:
				if 1<<align > 32/8 {
					return fmt.Errorf("invalid memory alignment")
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return err
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
			}
			pc += num
			// offset
			_, num, err = leb128.DecodeUint32(bytes.NewReader(body[pc:]))
			if err != nil {
				return fmt.Errorf("read memory offset: %v", err)
			}
			pc += num - 1
		} else if OpcodeMemorySize <= op && op <= OpcodeMemoryGrow {
			if memory == nil {
				return fmt.Errorf("unknown memory access")
			}
			pc++
			val, num, err := leb128.DecodeUint32(bytes.NewReader(body[pc:]))
			if err != nil {
				return fmt.Errorf("read immediate: %v", err)
			}
			if val != 0 || num != 1 {
				return fmt.Errorf("memory instruction reserved bytes not zero with 1 byte")
			}
			switch Opcode(op) {
			case OpcodeMemoryGrow:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return err
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeMemorySize:
				valueTypeStack.push(ValueTypeI32)
			}
			pc += num - 1
		} else if OpcodeI32Const <= op && op <= OpcodeF64Const {
			pc++
			switch Opcode(op) {
			case OpcodeI32Const:
				_, num, err := leb128.DecodeInt32(bytes.NewReader(body[pc:]))
				if err != nil {
					return fmt.Errorf("read i32 immediate: %s", err)
				}
				pc += num - 1
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI64Const:
				_, num, err := leb128.DecodeInt64(bytes.NewReader(body[pc:]))
				if err != nil {
					return fmt.Errorf("read i64 immediate: %v", err)
				}
				valueTypeStack.push(ValueTypeI64)
				pc += num - 1
			case OpcodeF32Const:
				valueTypeStack.push(ValueTypeF32)
				pc += 3
			case OpcodeF64Const:
				valueTypeStack.push(ValueTypeF64)
				pc += 7
			}
		} else if OpcodeLocalGet <= op && op <= OpcodeGlobalSet {
			pc++
			index, num, err := leb128.DecodeUint32(bytes.NewReader(body[pc:]))
			if err != nil {
				return fmt.Errorf("read immediate: %v", err)
			}
			pc += num - 1
			switch op {
			case OpcodeLocalGet:
				inputLen := uint32(len(functionType.Params))
				if l := uint32(len(localTypes)) + inputLen; index >= l {
					return fmt.Errorf("invalid local index for local.get %d >= %d(=len(locals)+len(parameters))", index, l)
				}
				if index < inputLen {
					valueTypeStack.push(functionType.Params[index])
				} else {
					valueTypeStack.push(localTypes[index-inputLen])
				}
			case OpcodeLocalSet:
				inputLen := uint32(len(functionType.Params))
				if l := uint32(len(localTypes)) + inputLen; index >= l {
					return fmt.Errorf("invalid local index for local.set %d >= %d(=len(locals)+len(parameters))", index, l)
				}
				var expType ValueType
				if index < inputLen {
					expType = functionType.Params[index]
				} else {
					expType = localTypes[index-inputLen]
				}
				if err := valueTypeStack.popAndVerifyType(expType); err != nil {
					return err
				}
			case OpcodeLocalTee:
				inputLen := uint32(len(functionType.Params))
				if l := uint32(len(localTypes)) + inputLen; index >= l {
					return fmt.Errorf("invalid local index for local.tee %d >= %d(=len(locals)+len(parameters))", index, l)
				}
				var expType ValueType
				if index < inputLen {
					expType = functionType.Params[index]
				} else {
					expType = localTypes[index-inputLen]
				}
				if err := valueTypeStack.popAndVerifyType(expType); err != nil {
					return err
				}
				valueTypeStack.push(expType)
			case OpcodeGlobalGet:
				if index >= uint32(len(globals)) {
					return fmt.Errorf("invalid global index")
				}
				valueTypeStack.push(globals[index].ValType)
			case OpcodeGlobalSet:
				if index >= uint32(len(globals)) {
					return fmt.Errorf("invalid global index")
				} else if !globals[index].Mutable {
					return fmt.Errorf("global.set when not mutable")
				} else if err := valueTypeStack.popAndVerifyType(
					globals[index].ValType); err != nil {
					return err
				}
			}
		} else if op == OpcodeBr {
			pc++
			index, num, err := leb128.DecodeUint32(bytes.NewReader(body[pc:]))
			if err != nil {
				return fmt.Errorf("read immediate: %v", err)
			} else if int(index) >= len(controlBlockStack) {
				return fmt.Errorf("invalid br operation: index out of range")
			}
			pc += num - 1
			// Check type soundness.
			target := controlBlockStack[len(controlBlockStack)-int(index)-1]
			targetResultType := target.blockType.Results
			if target.isLoop {
				// Loop operation doesn't require results since the continuation is
				// the beginning of the loop.
				targetResultType = []ValueType{}
			}
			if err := valueTypeStack.popResults(targetResultType, false); err != nil {
				return fmt.Errorf("type mismatch on the br operation: %v", err)
			}
			// br instruction is stack-polymorphic.
			valueTypeStack.unreachable()
		} else if op == OpcodeBrIf {
			pc++
			index, num, err := leb128.DecodeUint32(bytes.NewReader(body[pc:]))
			if err != nil {
				return fmt.Errorf("read immediate: %v", err)
			} else if int(index) >= len(controlBlockStack) {
				return fmt.Errorf(
					"invalid ln param given for br_if: index=%d with %d for the current lable stack length",
					index, len(controlBlockStack))
			}
			pc += num - 1
			if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
				return fmt.Errorf("cannot pop the required operand for br_if")
			}
			// Check type soundness.
			target := controlBlockStack[len(controlBlockStack)-int(index)-1]
			targetResultType := target.blockType.Results
			if target.isLoop {
				// Loop operation doesn't require results since the continuation is
				// the beginning of the loop.
				targetResultType = []ValueType{}
			}
			if err := valueTypeStack.popResults(targetResultType, false); err != nil {
				return fmt.Errorf("type mismatch on the br_if operation: %v", err)
			}
			// Push back the result
			for _, t := range targetResultType {
				valueTypeStack.push(t)
			}
		} else if op == OpcodeBrTable {
			pc++
			r := bytes.NewReader(body[pc:])
			nl, num, err := leb128.DecodeUint32(r)
			if err != nil {
				return fmt.Errorf("read immediate: %w", err)
			}

			list := make([]uint32, nl)
			for i := uint32(0); i < nl; i++ {
				l, n, err := leb128.DecodeUint32(r)
				if err != nil {
					return fmt.Errorf("read immediate: %w", err)
				}
				num += n
				list[i] = l
			}
			ln, n, err := leb128.DecodeUint32(r)
			if err != nil {
				return fmt.Errorf("read immediate: %w", err)
			} else if int(ln) >= len(controlBlockStack) {
				return fmt.Errorf(
					"invalid ln param given for br_table: ln=%d with %d for the current lable stack length",
					ln, len(controlBlockStack))
			}
			pc += n + num - 1
			// Check type soundness.
			if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
				return fmt.Errorf("cannot pop the required operand for br_table")
			}
			lnLabel := controlBlockStack[len(controlBlockStack)-1-int(ln)]
			expType := lnLabel.blockType.Results
			if lnLabel.isLoop {
				// Loop operation doesn't require results since the continuation is
				// the beginning of the loop.
				expType = []ValueType{}
			}
			for _, l := range list {
				if int(l) >= len(controlBlockStack) {
					return fmt.Errorf("invalid l param given for br_table")
				}
				label := controlBlockStack[len(controlBlockStack)-1-int(l)]
				expType2 := label.blockType.Results
				if label.isLoop {
					// Loop operation doesn't require results since the continuation is
					// the beginning of the loop.
					expType2 = []ValueType{}
				}
				if len(expType) != len(expType2) {
					return fmt.Errorf("incosistent block type length for br_table at %d; %v (ln=%d) != %v (l=%d)", l, expType, ln, expType2, l)
				}
				for i := range expType {
					if expType[i] != expType2[i] {
						return fmt.Errorf("incosistent block type for br_table at %d", l)
					}
				}
			}
			if err := valueTypeStack.popResults(expType, false); err != nil {
				return fmt.Errorf("type mismatch on the br_table operation: %v", err)
			}
			// br_table instruction is stack-polymorphic.
			valueTypeStack.unreachable()
		} else if op == OpcodeCall {
			pc++
			index, num, err := leb128.DecodeUint32(bytes.NewReader(body[pc:]))
			if err != nil {
				return fmt.Errorf("read immediate: %v", err)
			}
			pc += num - 1
			if int(index) >= len(functions) {
				return fmt.Errorf("invalid function index")
			}
			funcType := types[functions[index]]
			for i := 0; i < len(funcType.Params); i++ {
				if err := valueTypeStack.popAndVerifyType(funcType.Params[len(funcType.Params)-1-i]); err != nil {
					return fmt.Errorf("type mismatch on call operation param type")
				}
			}
			for _, exp := range funcType.Results {
				valueTypeStack.push(exp)
			}
		} else if op == OpcodeCallIndirect {
			pc++
			typeIndex, num, err := leb128.DecodeUint32(bytes.NewReader(body[pc:]))
			if err != nil {
				return fmt.Errorf("read immediate: %v", err)
			}
			pc += num - 1
			pc++
			if body[pc] != 0x00 {
				return fmt.Errorf("call_indirect reserved bytes not zero but got %d", body[pc])
			}
			if table == nil {
				return fmt.Errorf("table not given while having call_indirect")
			}
			if err = valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
				return fmt.Errorf("cannot pop the in table index's type for call_indirect")
			}
			if int(typeIndex) >= len(types) {
				return fmt.Errorf("invalid type index at call_indirect: %d", typeIndex)
			}
			funcType := types[typeIndex]
			for i := 0; i < len(funcType.Params); i++ {
				if err = valueTypeStack.popAndVerifyType(funcType.Params[len(funcType.Params)-1-i]); err != nil {
					return fmt.Errorf("type mismatch on call_indirect operation input type")
				}
			}
			for _, exp := range funcType.Results {
				valueTypeStack.push(exp)
			}
		} else if OpcodeI32Eqz <= op && op <= LastOpcode {
			switch op {
			case OpcodeI32Eqz:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the operand for i32.eqz: %v", err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI32Eq, OpcodeI32Ne, OpcodeI32LtS,
				OpcodeI32LtU, OpcodeI32GtS, OpcodeI32GtU, OpcodeI32LeS,
				OpcodeI32LeU, OpcodeI32GeS, OpcodeI32GeU:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the 1st i32 operand for 0x%x: %v", op, err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the 2nd i32 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI64Eqz:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the operand for i64.eqz: %v", err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI64Eq, OpcodeI64Ne, OpcodeI64LtS,
				OpcodeI64LtU, OpcodeI64GtS, OpcodeI64GtU,
				OpcodeI64LeS, OpcodeI64LeU, OpcodeI64GeS, OpcodeI64GeU:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the 1st i64 operand for 0x%x: %v", op, err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the 2nd i64 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeF32Eq, OpcodeF32Ne, OpcodeF32Lt, OpcodeF32Gt, OpcodeF32Le, OpcodeF32Ge:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the 1st f32 operand for 0x%x: %v", op, err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the 2nd f32 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeF64Eq, OpcodeF64Ne, OpcodeF64Lt, OpcodeF64Gt, OpcodeF64Le, OpcodeF64Ge:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the 1st f64 operand for 0x%x: %v", op, err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the 2nd f64 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI32Clz, OpcodeI32Ctz, OpcodeI32Popcnt:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the i32 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI32Add, OpcodeI32Sub, OpcodeI32Mul, OpcodeI32DivS,
				OpcodeI32DivU, OpcodeI32RemS, OpcodeI32RemU, OpcodeI32And,
				OpcodeI32Or, OpcodeI32Xor, OpcodeI32Shl, OpcodeI32ShrS,
				OpcodeI32ShrU, OpcodeI32Rotl, OpcodeI32Rotr:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the 1st i32 operand for 0x%x: %v", op, err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the 2nd i32 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI64Clz, OpcodeI64Ctz, OpcodeI64Popcnt:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the i64 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeI64)
			case OpcodeI64Add, OpcodeI64Sub, OpcodeI64Mul, OpcodeI64DivS,
				OpcodeI64DivU, OpcodeI64RemS, OpcodeI64RemU, OpcodeI64And,
				OpcodeI64Or, OpcodeI64Xor, OpcodeI64Shl, OpcodeI64ShrS,
				OpcodeI64ShrU, OpcodeI64Rotl, OpcodeI64Rotr:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the 1st i64 operand for 0x%x: %v", op, err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the 2nd i64 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeI64)
			case OpcodeF32Abs, OpcodeF32Neg, OpcodeF32Ceil,
				OpcodeF32Floor, OpcodeF32Trunc, OpcodeF32Nearest,
				OpcodeF32Sqrt:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the 1st f32 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeF32)
			case OpcodeF32Add, OpcodeF32Sub, OpcodeF32Mul,
				OpcodeF32Div, OpcodeF32Min, OpcodeF32Max,
				OpcodeF32Copysign:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the 1st f32 operand for 0x%x: %v", op, err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the 2nd f32 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeF32)
			case OpcodeF64Abs, OpcodeF64Neg, OpcodeF64Ceil,
				OpcodeF64Floor, OpcodeF64Trunc, OpcodeF64Nearest,
				OpcodeF64Sqrt:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the 1st f64 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeF64)
			case OpcodeF64Add, OpcodeF64Sub, OpcodeF64Mul,
				OpcodeF64Div, OpcodeF64Min, OpcodeF64Max,
				OpcodeF64Copysign:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the 1st f64 operand for 0x%x: %v", op, err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the 2nd f64 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeF64)
			case OpcodeI32WrapI64:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the operand for i32.wrap_i64: %v", err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI32TruncF32S, OpcodeI32TruncF32U:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the f32 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI32TruncF64S, OpcodeI32TruncF64U:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the f64 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI64ExtendI32S, OpcodeI64ExtendI32U:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the i32 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeI64)
			case OpcodeI64TruncF32S, OpcodeI64TruncF32U:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the f32 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeI64)
			case OpcodeI64TruncF64S, OpcodeI64TruncF64U:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the f64 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeI64)
			case OpcodeF32ConvertI32s, OpcodeF32ConvertI32U:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the i32 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeF32)
			case OpcodeF32ConvertI64S, OpcodeF32ConvertI64U:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the i64 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeF32)
			case OpcodeF32DemoteF64:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the operand for f32.demote_f64: %v", err)
				}
				valueTypeStack.push(ValueTypeF32)
			case OpcodeF64ConvertI32S, OpcodeF64ConvertI32U:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the i32 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeF64)
			case OpcodeF64ConvertI64S, OpcodeF64ConvertI64U:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the i64 operand for 0x%x: %v", op, err)
				}
				valueTypeStack.push(ValueTypeF64)
			case OpcodeF64PromoteF32:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the operand for f64.promote_f32: %v", err)
				}
				valueTypeStack.push(ValueTypeF64)
			case OpcodeI32ReinterpretF32:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the operand for i32.reinterpret_f32: %v", err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI64ReinterpretF64:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the operand for i64.reinterpret_f64: %v", err)
				}
				valueTypeStack.push(ValueTypeI64)
			case OpcodeF32ReinterpretI32:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the operand for f32.reinterpret_i32: %v", err)
				}
				valueTypeStack.push(ValueTypeF32)
			case OpcodeF64ReinterpretI64:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the operand for f64.reinterpret_i64: %v", err)
				}
				valueTypeStack.push(ValueTypeF64)
			case OpcodeI32Extend8S, OpcodeI32Extend16S:
				if err := enabledFeatures.Require(FeatureSignExtensionOps); err != nil {
					return fmt.Errorf("%s invalid as %v", instructionNames[op], err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the operand for %s: %v", instructionNames[op], err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI64Extend8S, OpcodeI64Extend16S, OpcodeI64Extend32S:
				if err := enabledFeatures.Require(FeatureSignExtensionOps); err != nil {
					return fmt.Errorf("%s invalid as %v", instructionNames[op], err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the operand for %s: %v", instructionNames[op], err)
				}
				valueTypeStack.push(ValueTypeI64)
			default:
				return fmt.Errorf("invalid numeric instruction 0x%x", op)
			}
		} else if op == OpcodeBlock {
			bt, num, err := decodeBlockType(types, bytes.NewReader(body[pc+1:]))
			if err != nil {
				return fmt.Errorf("read block: %w", err)
			}
			controlBlockStack = append(controlBlockStack, &controlBlock{
				startAt:        pc,
				blockType:      bt,
				blockTypeBytes: num,
			})
			valueTypeStack.pushStackLimit()
			pc += num
		} else if op == OpcodeLoop {
			bt, num, err := decodeBlockType(types, bytes.NewReader(body[pc+1:]))
			if err != nil {
				return fmt.Errorf("read block: %w", err)
			}
			controlBlockStack = append(controlBlockStack, &controlBlock{
				startAt:        pc,
				blockType:      bt,
				blockTypeBytes: num,
				isLoop:         true,
			})
			valueTypeStack.pushStackLimit()
			pc += num
		} else if op == OpcodeIf {
			bt, num, err := decodeBlockType(types, bytes.NewReader(body[pc+1:]))
			if err != nil {
				return fmt.Errorf("read block: %w", err)
			}
			controlBlockStack = append(controlBlockStack, &controlBlock{
				startAt:        pc,
				blockType:      bt,
				blockTypeBytes: num,
				isIf:           true,
			})
			if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
				return fmt.Errorf("cannot pop the operand for 'if': %v", err)
			}
			valueTypeStack.pushStackLimit()
			pc += num
		} else if op == OpcodeElse {
			bl := controlBlockStack[len(controlBlockStack)-1]
			bl.elseAt = pc
			// Check the type soundness of the instructions *before* entering this else Op.
			if err := valueTypeStack.popResults(bl.blockType.Results, true); err != nil {
				return fmt.Errorf("invalid instruction results in then instructions")
			}
			// Before entering instructions inside else, we pop all the values pushed by then block.
			valueTypeStack.resetAtStackLimit()
		} else if op == OpcodeEnd {
			bl := controlBlockStack[len(controlBlockStack)-1]
			bl.endAt = pc
			controlBlockStack = controlBlockStack[:len(controlBlockStack)-1]
			if bl.isIf && bl.elseAt <= bl.startAt {
				if len(bl.blockType.Results) > 0 {
					return fmt.Errorf("type mismatch between then and else blocks")
				}
				// To handle if block without else properly,
				// we set ElseAt to EndAt-1 so we can just skip else.
				bl.elseAt = bl.endAt - 1
			}
			// Check type soundness.
			if err := valueTypeStack.popResults(bl.blockType.Results, true); err != nil {
				return fmt.Errorf("invalid instruction results at end instruction; expected %v: %v", bl.blockType.Results, err)
			}
			// Put the result types at the end after resetting at the stack limit
			// since we might have Any type between the limit and the current top.
			valueTypeStack.resetAtStackLimit()
			for _, exp := range bl.blockType.Results {
				valueTypeStack.push(exp)
			}
			// We exit if/loop/block, so reset the constraints on the stack manipulation
			// on values previously pushed by outer blocks.
			valueTypeStack.popStackLimit()
		} else if op == OpcodeReturn {
			expTypes := functionType.Results
			for i := 0; i < len(expTypes); i++ {
				if err := valueTypeStack.popAndVerifyType(expTypes[len(expTypes)-1-i]); err != nil {
					return fmt.Errorf("return type mismatch on return: %v; want %v", err, expTypes)
				}
			}
			// return instruction is stack-polymorphic.
			valueTypeStack.unreachable()
		} else if op == OpcodeDrop {
			_, err := valueTypeStack.pop()
			if err != nil {
				return fmt.Errorf("invalid drop: %v", err)
			}
		} else if op == OpcodeSelect {
			if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
				return fmt.Errorf("type mismatch on 3rd select operand: %v", err)
			}
			v1, err := valueTypeStack.pop()
			if err != nil {
				return fmt.Errorf("invalid select: %v", err)
			}
			v2, err := valueTypeStack.pop()
			if err != nil {
				return fmt.Errorf("invalid select: %v", err)
			}
			if v1 != v2 && v1 != valueTypeUnknown && v2 != valueTypeUnknown {
				return fmt.Errorf("type mismatch on 1st and 2nd select operands")
			}
			if v1 == valueTypeUnknown {
				valueTypeStack.push(v2)
			} else {
				valueTypeStack.push(v1)
			}
		} else if op == OpcodeUnreachable {
			// unreachable instruction is stack-polymorphic.
			valueTypeStack.unreachable()
		} else if op == OpcodeNop {
		} else {
			return fmt.Errorf("invalid instruction 0x%x", op)
		}
	}

	if len(controlBlockStack) > 0 {
		return fmt.Errorf("ill-nested block exists")
	}
	if valueTypeStack.maximumStackPointer > maxStackValues {
		return fmt.Errorf("function may have %d stack values, which exceeds limit %d", valueTypeStack.maximumStackPointer, maxStackValues)
	}
	return nil
}

type valueTypeStack struct {
	stack               []ValueType
	stackLimits         []int
	maximumStackPointer int
}

const (
	// Only used in the analyzeFunction below.
	valueTypeUnknown = ValueType(0xFF)
)

func (s *valueTypeStack) pop() (ValueType, error) {
	limit := 0
	if len(s.stackLimits) > 0 {
		limit = s.stackLimits[len(s.stackLimits)-1]
	}
	if len(s.stack) <= limit {
		return 0, fmt.Errorf("invalid operation: trying to pop at %d with limit %d",
			len(s.stack), limit)
	} else if len(s.stack) == limit+1 && s.stack[limit] == valueTypeUnknown {
		return valueTypeUnknown, nil
	} else {
		ret := s.stack[len(s.stack)-1]
		s.stack = s.stack[:len(s.stack)-1]
		return ret, nil
	}
}

func (s *valueTypeStack) popAndVerifyType(expected ValueType) error {
	actual, err := s.pop()
	if err != nil {
		return err
	}
	if actual != expected && actual != valueTypeUnknown && expected != valueTypeUnknown {
		return fmt.Errorf("type mismatch")
	}
	return nil
}

func (s *valueTypeStack) push(v ValueType) {
	s.stack = append(s.stack, v)
	if sp := len(s.stack); sp > s.maximumStackPointer {
		s.maximumStackPointer = sp
	}
}

func (s *valueTypeStack) unreachable() {
	s.resetAtStackLimit()
	s.stack = append(s.stack, valueTypeUnknown)
}

func (s *valueTypeStack) resetAtStackLimit() {
	if len(s.stackLimits) != 0 {
		s.stack = s.stack[:s.stackLimits[len(s.stackLimits)-1]]
	} else {
		s.stack = []ValueType{}
	}
}

func (s *valueTypeStack) popStackLimit() {
	if len(s.stackLimits) != 0 {
		s.stackLimits = s.stackLimits[:len(s.stackLimits)-1]
	}
}

func (s *valueTypeStack) pushStackLimit() {
	s.stackLimits = append(s.stackLimits, len(s.stack))
}

func (s *valueTypeStack) popResults(expResults []ValueType, checkAboveLimit bool) error {
	limit := 0
	if len(s.stackLimits) > 0 {
		limit = s.stackLimits[len(s.stackLimits)-1]
	}
	for _, exp := range expResults {
		if err := s.popAndVerifyType(exp); err != nil {
			return err
		}
	}
	if checkAboveLimit {
		if !(limit == len(s.stack) || (limit+1 == len(s.stack) && s.stack[limit] == valueTypeUnknown)) {
			return fmt.Errorf("leftovers found in the stack")
		}
	}
	return nil
}

func (s *valueTypeStack) String() string {
	var typeStrs, limits []string
	for _, v := range s.stack {
		var str string
		if v == valueTypeUnknown {
			str = "unknown"
		} else if v == ValueTypeI32 {
			str = "i32"
		} else if v == ValueTypeI64 {
			str = "i64"
		} else if v == ValueTypeF32 {
			str = "f32"
		} else if v == ValueTypeF64 {
			str = "f64"
		}
		typeStrs = append(typeStrs, str)
	}
	for _, d := range s.stackLimits {
		limits = append(limits, fmt.Sprintf("%d", d))
	}
	return fmt.Sprintf("{stack: [%s], limits: [%s]}",
		strings.Join(typeStrs, ", "), strings.Join(limits, ","))
}

type controlBlock struct {
	startAt, elseAt, endAt uint64
	blockType              *FunctionType
	blockTypeBytes         uint64
	isLoop                 bool
	isIf                   bool
}

func decodeBlockType(types []*FunctionType, r *bytes.Reader) (*FunctionType, uint64, error) {
	return decodeBlockTypeImpl(func(index int64) (*FunctionType, error) {
		if index < 0 || (index >= int64(len(types))) {
			return nil, fmt.Errorf("type index out of range: %d", index)
		}
		return types[index], nil
	}, r)
}

// DecodeBlockType is exported for use in the compiler
func DecodeBlockType(types []*TypeInstance, r *bytes.Reader) (*FunctionType, uint64, error) {
	return decodeBlockTypeImpl(func(index int64) (*FunctionType, error) {
		if index < 0 || (index >= int64(len(types))) {
			return nil, fmt.Errorf("type index out of range: %d", index)
		}
		return types[index].Type, nil
	}, r)
}

func decodeBlockTypeImpl(functionTypeResolver func(index int64) (*FunctionType, error), r *bytes.Reader) (*FunctionType, uint64, error) {
	raw, num, err := leb128.DecodeInt33AsInt64(r)
	if err != nil {
		return nil, 0, fmt.Errorf("decode int33: %w", err)
	}

	var ret *FunctionType
	switch raw {
	case -64: // 0x40 in original byte = nil
		ret = &FunctionType{}
	case -1: // 0x7f in original byte = i32
		ret = &FunctionType{Results: []ValueType{ValueTypeI32}}
	case -2: // 0x7e in original byte = i64
		ret = &FunctionType{Results: []ValueType{ValueTypeI64}}
	case -3: // 0x7d in original byte = f32
		ret = &FunctionType{Results: []ValueType{ValueTypeF32}}
	case -4: // 0x7c in original byte = f64
		ret = &FunctionType{Results: []ValueType{ValueTypeF64}}
	default:
		ret, err = functionTypeResolver(raw)
	}
	return ret, num, err
}
