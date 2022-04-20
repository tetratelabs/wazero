package wasm

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/leb128"
)

// The wazero specific limitation described at RATIONALE.md.
const maximumValuesOnStack = 1 << 27

// validateFunction validates the instruction sequence of a function.
// following the specification https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#instructions%E2%91%A2.
//
// * idx is the index in the FunctionSection
// * functions are the function index namespace, which is prefixed by imports. The value is the TypeSection index.
// * globals are the global index namespace, which is prefixed by imports.
// * memory is the potentially imported memory and can be nil.
// * table is the potentially imported table and can be nil.
//
// Returns an error if the instruction sequence is not valid,
// or potentially it can exceed the maximum number of values on the stack.
func (m *Module) validateFunction(enabledFeatures Features, idx Index, functions []Index, globals []*GlobalType, memory *Memory, table *Table) error {
	return m.validateFunctionWithMaxStackValues(enabledFeatures, idx, functions, globals, memory, table, maximumValuesOnStack)
}

// validateFunctionWithMaxStackValues is like validateFunction, but allows overriding maxStackValues for testing.
//
// * maxStackValues is the maximum height of values stack which the target is allowed to reach.
func (m *Module) validateFunctionWithMaxStackValues(
	enabledFeatures Features,
	idx Index,
	functions []Index,
	globals []*GlobalType,
	memory *Memory,
	table *Table,
	maxStackValues int,
) error {
	functionType := m.TypeSection[m.FunctionSection[idx]]
	body := m.CodeSection[idx].Body
	localTypes := m.CodeSection[idx].LocalTypes
	types := m.TypeSection

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
					return fmt.Errorf("invalid local index for %s %d >= %d(=len(locals)+len(parameters))",
						OpcodeLocalGetName, index, l)
				}
				if index < inputLen {
					valueTypeStack.push(functionType.Params[index])
				} else {
					valueTypeStack.push(localTypes[index-inputLen])
				}
			case OpcodeLocalSet:
				inputLen := uint32(len(functionType.Params))
				if l := uint32(len(localTypes)) + inputLen; index >= l {
					return fmt.Errorf("invalid local index for %s %d >= %d(=len(locals)+len(parameters))",
						OpcodeLocalSetName, index, l)
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
					return fmt.Errorf("invalid local index for %s %d >= %d(=len(locals)+len(parameters))",
						OpcodeLocalTeeName, index, l)
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
					return fmt.Errorf("invalid index for %s", OpcodeGlobalGetName)
				}
				valueTypeStack.push(globals[index].ValType)
			case OpcodeGlobalSet:
				if index >= uint32(len(globals)) {
					return fmt.Errorf("invalid global index")
				} else if !globals[index].Mutable {
					return fmt.Errorf("%s when not mutable", OpcodeGlobalSetName)
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
				return fmt.Errorf("invalid %s operation: index out of range", OpcodeBrName)
			}
			pc += num - 1
			// Check type soundness.
			target := controlBlockStack[len(controlBlockStack)-int(index)-1]
			targetResultType := target.blockType.Results
			if target.op == OpcodeLoop {
				// Loop operation doesn't require results since the continuation is
				// the beginning of the loop.
				targetResultType = []ValueType{}
			}
			if err = valueTypeStack.popResults(op, targetResultType, false); err != nil {
				return err
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
					"invalid ln param given for %s: index=%d with %d for the current lable stack length",
					OpcodeBrIfName, index, len(controlBlockStack))
			}
			pc += num - 1
			if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
				return fmt.Errorf("cannot pop the required operand for %s", OpcodeBrIfName)
			}
			// Check type soundness.
			target := controlBlockStack[len(controlBlockStack)-int(index)-1]
			targetResultType := target.blockType.Results
			if target.op == OpcodeLoop {
				// Loop operation doesn't require results since the continuation is
				// the beginning of the loop.
				targetResultType = []ValueType{}
			}
			if err := valueTypeStack.popResults(op, targetResultType, false); err != nil {
				return err
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
					"invalid ln param given for %s: ln=%d with %d for the current lable stack length",
					OpcodeBrTableName, ln, len(controlBlockStack))
			}
			pc += n + num - 1
			// Check type soundness.
			if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
				return fmt.Errorf("cannot pop the required operand for %s", OpcodeBrTableName)
			}
			lnLabel := controlBlockStack[len(controlBlockStack)-1-int(ln)]
			expType := lnLabel.blockType.Results
			if lnLabel.op == OpcodeLoop {
				// Loop operation doesn't require results since the continuation is
				// the beginning of the loop.
				expType = []ValueType{}
			}
			for _, l := range list {
				if int(l) >= len(controlBlockStack) {
					return fmt.Errorf("invalid l param given for %s", OpcodeBrTableName)
				}
				label := controlBlockStack[len(controlBlockStack)-1-int(l)]
				expType2 := label.blockType.Results
				if label.op == OpcodeLoop {
					// Loop operation doesn't require results since the continuation is
					// the beginning of the loop.
					expType2 = []ValueType{}
				}
				if len(expType) != len(expType2) {
					return fmt.Errorf("incosistent block type length for %s at %d; %v (ln=%d) != %v (l=%d)", OpcodeBrTableName, l, expType, ln, expType2, l)
				}
				for i := range expType {
					if expType[i] != expType2[i] {
						return fmt.Errorf("incosistent block type for %s at %d", OpcodeBrTableName, l)
					}
				}
			}
			if err = valueTypeStack.popResults(op, expType, false); err != nil {
				return err
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
					return fmt.Errorf("type mismatch on %s operation param type: %v", OpcodeCallName, err)
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
				return fmt.Errorf("%s reserved bytes not zero but got %d", OpcodeCallIndirectName, body[pc])
			}
			if table == nil {
				return fmt.Errorf("table not given while having %s", OpcodeCallIndirectName)
			}
			if err = valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
				return fmt.Errorf("cannot pop the in table index's type for %s", OpcodeCallIndirectName)
			}
			if int(typeIndex) >= len(types) {
				return fmt.Errorf("invalid type index at %s: %d", OpcodeCallIndirectName, typeIndex)
			}
			funcType := types[typeIndex]
			for i := 0; i < len(funcType.Params); i++ {
				if err = valueTypeStack.popAndVerifyType(funcType.Params[len(funcType.Params)-1-i]); err != nil {
					return fmt.Errorf("type mismatch on %s operation input type", OpcodeCallIndirectName)
				}
			}
			for _, exp := range funcType.Results {
				valueTypeStack.push(exp)
			}
		} else if OpcodeI32Eqz <= op && op <= OpcodeI64Extend32S {
			switch op {
			case OpcodeI32Eqz:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the operand for %s: %v", OpcodeI32EqzName, err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI32Eq, OpcodeI32Ne, OpcodeI32LtS,
				OpcodeI32LtU, OpcodeI32GtS, OpcodeI32GtU, OpcodeI32LeS,
				OpcodeI32LeU, OpcodeI32GeS, OpcodeI32GeU:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the 1st i32 operand for %s: %v", InstructionName(op), err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the 2nd i32 operand for %s: %v", InstructionName(op), err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI64Eqz:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the operand for %s: %v", OpcodeI64EqzName, err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI64Eq, OpcodeI64Ne, OpcodeI64LtS,
				OpcodeI64LtU, OpcodeI64GtS, OpcodeI64GtU,
				OpcodeI64LeS, OpcodeI64LeU, OpcodeI64GeS, OpcodeI64GeU:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the 1st i64 operand for %s: %v", InstructionName(op), err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the 2nd i64 operand for %s: %v", InstructionName(op), err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeF32Eq, OpcodeF32Ne, OpcodeF32Lt, OpcodeF32Gt, OpcodeF32Le, OpcodeF32Ge:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the 1st f32 operand for %s: %v", InstructionName(op), err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the 2nd f32 operand for %s: %v", InstructionName(op), err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeF64Eq, OpcodeF64Ne, OpcodeF64Lt, OpcodeF64Gt, OpcodeF64Le, OpcodeF64Ge:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the 1st f64 operand for %s: %v", InstructionName(op), err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the 2nd f64 operand for %s: %v", InstructionName(op), err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI32Clz, OpcodeI32Ctz, OpcodeI32Popcnt:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the i32 operand for %s: %v", InstructionName(op), err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI32Add, OpcodeI32Sub, OpcodeI32Mul, OpcodeI32DivS,
				OpcodeI32DivU, OpcodeI32RemS, OpcodeI32RemU, OpcodeI32And,
				OpcodeI32Or, OpcodeI32Xor, OpcodeI32Shl, OpcodeI32ShrS,
				OpcodeI32ShrU, OpcodeI32Rotl, OpcodeI32Rotr:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the 1st operand for %s: %v", InstructionName(op), err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the 2nd operand for %s: %v", InstructionName(op), err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI64Clz, OpcodeI64Ctz, OpcodeI64Popcnt:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the i64 operand for %s: %v", InstructionName(op), err)
				}
				valueTypeStack.push(ValueTypeI64)
			case OpcodeI64Add, OpcodeI64Sub, OpcodeI64Mul, OpcodeI64DivS,
				OpcodeI64DivU, OpcodeI64RemS, OpcodeI64RemU, OpcodeI64And,
				OpcodeI64Or, OpcodeI64Xor, OpcodeI64Shl, OpcodeI64ShrS,
				OpcodeI64ShrU, OpcodeI64Rotl, OpcodeI64Rotr:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the 1st i64 operand for %s: %v", InstructionName(op), err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the 2nd i64 operand for %s: %v", InstructionName(op), err)
				}
				valueTypeStack.push(ValueTypeI64)
			case OpcodeF32Abs, OpcodeF32Neg, OpcodeF32Ceil,
				OpcodeF32Floor, OpcodeF32Trunc, OpcodeF32Nearest,
				OpcodeF32Sqrt:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the 1st f32 operand for %s: %v", InstructionName(op), err)
				}
				valueTypeStack.push(ValueTypeF32)
			case OpcodeF32Add, OpcodeF32Sub, OpcodeF32Mul,
				OpcodeF32Div, OpcodeF32Min, OpcodeF32Max,
				OpcodeF32Copysign:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the 1st f32 operand for %s: %v", InstructionName(op), err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the 2nd f32 operand for %s: %v", InstructionName(op), err)
				}
				valueTypeStack.push(ValueTypeF32)
			case OpcodeF64Abs, OpcodeF64Neg, OpcodeF64Ceil,
				OpcodeF64Floor, OpcodeF64Trunc, OpcodeF64Nearest,
				OpcodeF64Sqrt:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the 1st f64 operand for %s: %v", InstructionName(op), err)
				}
				valueTypeStack.push(ValueTypeF64)
			case OpcodeF64Add, OpcodeF64Sub, OpcodeF64Mul,
				OpcodeF64Div, OpcodeF64Min, OpcodeF64Max,
				OpcodeF64Copysign:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the 1st f64 operand for %s: %v", InstructionName(op), err)
				}
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the 2nd f64 operand for %s: %v", InstructionName(op), err)
				}
				valueTypeStack.push(ValueTypeF64)
			case OpcodeI32WrapI64:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the operand for %s: %v", OpcodeI32WrapI64Name, err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI32TruncF32S, OpcodeI32TruncF32U:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the f32 operand for %s: %v", InstructionName(op), err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI32TruncF64S, OpcodeI32TruncF64U:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the f64 operand for %s: %v", InstructionName(op), err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI64ExtendI32S, OpcodeI64ExtendI32U:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the i32 operand for %s: %v", InstructionName(op), err)
				}
				valueTypeStack.push(ValueTypeI64)
			case OpcodeI64TruncF32S, OpcodeI64TruncF32U:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the f32 operand for %s: %v", InstructionName(op), err)
				}
				valueTypeStack.push(ValueTypeI64)
			case OpcodeI64TruncF64S, OpcodeI64TruncF64U:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the f64 operand for %s: %v", InstructionName(op), err)
				}
				valueTypeStack.push(ValueTypeI64)
			case OpcodeF32ConvertI32s, OpcodeF32ConvertI32U:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the i32 operand for %s: %v", InstructionName(op), err)
				}
				valueTypeStack.push(ValueTypeF32)
			case OpcodeF32ConvertI64S, OpcodeF32ConvertI64U:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the i64 operand for %s: %v", InstructionName(op), err)
				}
				valueTypeStack.push(ValueTypeF32)
			case OpcodeF32DemoteF64:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the operand for %s: %v", OpcodeF32DemoteF64Name, err)
				}
				valueTypeStack.push(ValueTypeF32)
			case OpcodeF64ConvertI32S, OpcodeF64ConvertI32U:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the i32 operand for %s: %v", InstructionName(op), err)
				}
				valueTypeStack.push(ValueTypeF64)
			case OpcodeF64ConvertI64S, OpcodeF64ConvertI64U:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the i64 operand for %s: %v", InstructionName(op), err)
				}
				valueTypeStack.push(ValueTypeF64)
			case OpcodeF64PromoteF32:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the operand for %s: %v", OpcodeF64PromoteF32Name, err)
				}
				valueTypeStack.push(ValueTypeF64)
			case OpcodeI32ReinterpretF32:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF32); err != nil {
					return fmt.Errorf("cannot pop the operand for %s: %v", OpcodeI32ReinterpretF32Name, err)
				}
				valueTypeStack.push(ValueTypeI32)
			case OpcodeI64ReinterpretF64:
				if err := valueTypeStack.popAndVerifyType(ValueTypeF64); err != nil {
					return fmt.Errorf("cannot pop the operand for %s: %v", OpcodeI64ReinterpretF64Name, err)
				}
				valueTypeStack.push(ValueTypeI64)
			case OpcodeF32ReinterpretI32:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
					return fmt.Errorf("cannot pop the operand for %s: %v", OpcodeF32ReinterpretI32Name, err)
				}
				valueTypeStack.push(ValueTypeF32)
			case OpcodeF64ReinterpretI64:
				if err := valueTypeStack.popAndVerifyType(ValueTypeI64); err != nil {
					return fmt.Errorf("cannot pop the operand for %s: %v", OpcodeF64ReinterpretI64Name, err)
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
		} else if op == OpcodeMiscPrefix {
			pc++
			miscOpcode := body[pc]
			if miscOpcode >= OpcodeMiscI32TruncSatF32S && miscOpcode <= OpcodeMiscI64TruncSatF64U {
				if err := enabledFeatures.Require(FeatureNonTrappingFloatToIntConversion); err != nil {
					return fmt.Errorf("%s invalid as %v", miscInstructionNames[miscOpcode], err)
				}
				var inType, outType ValueType
				switch miscOpcode {
				case OpcodeMiscI32TruncSatF32S, OpcodeMiscI32TruncSatF32U:
					inType, outType = ValueTypeF32, ValueTypeI32
				case OpcodeMiscI32TruncSatF64S, OpcodeMiscI32TruncSatF64U:
					inType, outType = ValueTypeF64, ValueTypeI32
				case OpcodeMiscI64TruncSatF32S, OpcodeMiscI64TruncSatF32U:
					inType, outType = ValueTypeF32, ValueTypeI64
				case OpcodeMiscI64TruncSatF64S, OpcodeMiscI64TruncSatF64U:
					inType, outType = ValueTypeF64, ValueTypeI64
				}
				if err := valueTypeStack.popAndVerifyType(inType); err != nil {
					return fmt.Errorf("cannot pop the operand for %s: %v", miscInstructionNames[miscOpcode], err)
				}
				valueTypeStack.push(outType)
			}
		} else if op == OpcodeBlock {
			bt, num, err := DecodeBlockType(types, bytes.NewReader(body[pc+1:]), enabledFeatures)
			if err != nil {
				return fmt.Errorf("read block: %w", err)
			}
			controlBlockStack = append(controlBlockStack, &controlBlock{
				startAt:        pc,
				blockType:      bt,
				blockTypeBytes: num,
			})
			valueTypeStack.pushStackLimit(len(bt.Params))
			pc += num
		} else if op == OpcodeLoop {
			bt, num, err := DecodeBlockType(types, bytes.NewReader(body[pc+1:]), enabledFeatures)
			if err != nil {
				return fmt.Errorf("read block: %w", err)
			}
			controlBlockStack = append(controlBlockStack, &controlBlock{
				startAt:        pc,
				blockType:      bt,
				blockTypeBytes: num,
				op:             op,
			})
			if err = valueTypeStack.popParams(op, bt.Params, false); err != nil {
				return err
			}
			// Plus we have to push any block params again.
			for _, p := range bt.Params {
				valueTypeStack.push(p)
			}
			valueTypeStack.pushStackLimit(len(bt.Params))
			pc += num
		} else if op == OpcodeIf {
			bt, num, err := DecodeBlockType(types, bytes.NewReader(body[pc+1:]), enabledFeatures)
			if err != nil {
				return fmt.Errorf("read block: %w", err)
			}
			controlBlockStack = append(controlBlockStack, &controlBlock{
				startAt:        pc,
				blockType:      bt,
				blockTypeBytes: num,
				op:             op,
			})
			if err = valueTypeStack.popAndVerifyType(ValueTypeI32); err != nil {
				return fmt.Errorf("cannot pop the operand for 'if': %v", err)
			}
			if err = valueTypeStack.popParams(op, bt.Params, false); err != nil {
				return err
			}
			// Plus we have to push any block params again.
			for _, p := range bt.Params {
				valueTypeStack.push(p)
			}
			valueTypeStack.pushStackLimit(len(bt.Params))
			pc += num
		} else if op == OpcodeElse {
			bl := controlBlockStack[len(controlBlockStack)-1]
			bl.elseAt = pc
			// Check the type soundness of the instructions *before* entering this else Op.
			if err := valueTypeStack.popResults(OpcodeIf, bl.blockType.Results, true); err != nil {
				return err
			}
			// Before entering instructions inside else, we pop all the values pushed by then block.
			valueTypeStack.resetAtStackLimit()
			// Plus we have to push any block params again.
			for _, p := range bl.blockType.Params {
				valueTypeStack.push(p)
			}
		} else if op == OpcodeEnd {
			bl := controlBlockStack[len(controlBlockStack)-1]
			bl.endAt = pc
			controlBlockStack = controlBlockStack[:len(controlBlockStack)-1]

			// OpcodeEnd can end a block or the function itself. Check to see what it is:

			ifMissingElse := bl.op == OpcodeIf && bl.elseAt <= bl.startAt
			if ifMissingElse {
				// If this is the end of block without else, the number of block's results and params must be same.
				// Otherwise, the value stack would result in the inconsistent state at runtime.
				if !bytes.Equal(bl.blockType.Results, bl.blockType.Params) {
					return typeCountError(false, OpcodeElseName, bl.blockType.Params, bl.blockType.Results)
				}
				// -1 skips else, to handle if block without else properly.
				bl.elseAt = bl.endAt - 1
			}

			// Determine the block context
			ctx := "" // the outer-most block: the function return
			if bl.op == OpcodeIf && !ifMissingElse && bl.elseAt > 0 {
				ctx = OpcodeElseName
			} else if bl.op != 0 {
				ctx = InstructionName(bl.op)
			}

			// Check return types match
			if err := valueTypeStack.requireStackValues(false, ctx, bl.blockType.Results, true); err != nil {
				return err
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
			// Same formatting as OpcodeEnd on the outer-most block
			if err := valueTypeStack.requireStackValues(false, "", functionType.Results, false); err != nil {
				return err
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

func (s *valueTypeStack) tryPop() (vt ValueType, limit int, ok bool) {
	if len(s.stackLimits) > 0 {
		limit = s.stackLimits[len(s.stackLimits)-1]
	}
	stackLen := len(s.stack)
	if stackLen <= limit {
		return
	} else if stackLen == limit+1 && s.stack[limit] == valueTypeUnknown {
		vt = valueTypeUnknown
		ok = true
		return
	} else {
		vt = s.stack[stackLen-1]
		s.stack = s.stack[:stackLen-1]
		ok = true
		return
	}
}

func (s *valueTypeStack) pop() (ValueType, error) {
	if vt, limit, ok := s.tryPop(); ok {
		return vt, nil
	} else {
		return 0, fmt.Errorf("invalid operation: trying to pop at %d with limit %d", len(s.stack), limit)
	}
}

// popAndVerifyType returns an error if the stack value is unexpected.
func (s *valueTypeStack) popAndVerifyType(expected ValueType) error {
	have, _, ok := s.tryPop()
	if !ok {
		return fmt.Errorf("%s missing", ValueTypeName(expected))
	}
	if have != expected && have != valueTypeUnknown && expected != valueTypeUnknown {
		return fmt.Errorf("type mismatch: expected %s, but was %s", ValueTypeName(expected), ValueTypeName(have))
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

// pushStackLimit pushes the control frame's bottom of the stack.
func (s *valueTypeStack) pushStackLimit(params int) {
	limit := len(s.stack) - params
	s.stackLimits = append(s.stackLimits, limit)
}

func (s *valueTypeStack) popParams(oc Opcode, want []ValueType, checkAboveLimit bool) error {
	return s.requireStackValues(true, InstructionName(oc), want, checkAboveLimit)
}

func (s *valueTypeStack) popResults(oc Opcode, want []ValueType, checkAboveLimit bool) error {
	return s.requireStackValues(false, InstructionName(oc), want, checkAboveLimit)
}

func (s *valueTypeStack) requireStackValues(
	isParam bool,
	context string,
	want []ValueType,
	checkAboveLimit bool,
) error {
	limit := 0
	if len(s.stackLimits) > 0 {
		limit = s.stackLimits[len(s.stackLimits)-1]
	}
	// Iterate backwards as we are comparing the desired slice against stack value types.
	countWanted := len(want)

	// First, check if there are enough values on the stack.
	have := make([]ValueType, 0, countWanted)
	for i := countWanted - 1; i >= 0; i-- {
		popped, _, ok := s.tryPop()
		if !ok {
			if len(have) > len(want) {
				return typeCountError(isParam, context, have, want)
			}
			return typeCountError(isParam, context, have, want)
		}
		have = append(have, popped)
	}

	// Now, check if there are too many values.
	if checkAboveLimit {
		if !(limit == len(s.stack) || (limit+1 == len(s.stack) && s.stack[limit] == valueTypeUnknown)) {
			return typeCountError(isParam, context, append(s.stack, want...), want)
		}
	}

	// Finally, check the types of the values:
	for i, v := range have {
		nextWant := want[countWanted-i-1] // have is in reverse order (stack)
		if v != nextWant && v != valueTypeUnknown && nextWant != valueTypeUnknown {
			return typeMismatchError(isParam, context, v, nextWant, i)
		}
	}
	return nil
}

// typeMismatchError returns an error similar to go compiler's error on type mismatch.
func typeMismatchError(isParam bool, context string, have ValueType, want ValueType, i int) error {
	var ret strings.Builder
	ret.WriteString("cannot use ")
	ret.WriteString(ValueTypeName(have))
	if context != "" {
		ret.WriteString(" in ")
		ret.WriteString(context)
		ret.WriteString(" block")
	}
	if isParam {
		ret.WriteString(" as param")
	} else {
		ret.WriteString(" as result")
	}
	ret.WriteString("[")
	ret.WriteString(strconv.Itoa(i))
	ret.WriteString("] type ")
	ret.WriteString(ValueTypeName(want))
	return errors.New(ret.String())
}

// typeCountError returns an error similar to go compiler's error on type count mismatch.
func typeCountError(isParam bool, context string, have []ValueType, want []ValueType) error {
	var ret strings.Builder
	if len(have) > len(want) {
		ret.WriteString("too many ")
	} else {
		ret.WriteString("not enough ")
	}
	if isParam {
		ret.WriteString("params")
	} else {
		ret.WriteString("results")
	}
	if context != "" {
		if isParam {
			ret.WriteString(" for ")
		} else {
			ret.WriteString(" in ")
		}
		ret.WriteString(context)
		ret.WriteString(" block")
	}
	ret.WriteString("\n\thave (")
	writeValueTypes(have, &ret)
	ret.WriteString(")\n\twant (")
	writeValueTypes(want, &ret)
	ret.WriteByte(')')
	return errors.New(ret.String())
}

func writeValueTypes(vts []ValueType, ret *strings.Builder) {
	switch len(vts) {
	case 0:
	case 1:
		ret.WriteString(api.ValueTypeName(vts[0]))
	default:
		ret.WriteString(api.ValueTypeName(vts[0]))
		for _, vt := range vts[1:] {
			ret.WriteString(", ")
			ret.WriteString(api.ValueTypeName(vt))
		}
	}
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
	// op is zero when the outermost block
	op Opcode
}

func DecodeBlockType(types []*FunctionType, r *bytes.Reader, enabledFeatures Features) (*FunctionType, uint64, error) {
	return decodeBlockTypeImpl(func(index int64) (*FunctionType, error) {
		if index < 0 || (index >= int64(len(types))) {
			return nil, fmt.Errorf("type index out of range: %d", index)
		}
		return types[index], nil
	}, r, enabledFeatures)
}

// decodeBlockTypeImpl decodes the type index from a positive 33-bit signed integer. Negative numbers indicate up to one
// WebAssembly 1.0 (20191205) compatible result type. Positive numbers are decoded when `enabledFeatures` include
// FeatureMultiValue and include an index in the Module.TypeSection.
//
// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#binary-blocktype
// See https://github.com/WebAssembly/spec/blob/main/proposals/multi-value/Overview.md
func decodeBlockTypeImpl(functionTypeResolver func(index int64) (*FunctionType, error), r *bytes.Reader, enabledFeatures Features) (*FunctionType, uint64, error) {
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
		if err = enabledFeatures.Require(FeatureMultiValue); err != nil {
			return nil, num, fmt.Errorf("block with function type return invalid as %v", err)
		}
		ret, err = functionTypeResolver(raw)
	}
	return ret, num, err
}
