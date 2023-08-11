package frontend

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
	"github.com/tetratelabs/wazero/internal/leb128"
	"github.com/tetratelabs/wazero/internal/wasm"
)

type (
	// loweringState is used to keep the state of lowering.
	loweringState struct {
		// values holds the values on the Wasm stack.
		values           []ssa.Value
		controlFrames    []controlFrame
		unreachable      bool
		unreachableDepth int
		pc               int
	}
	controlFrame struct {
		kind controlFrameKind
		// originalStackLen holds the number of values on the Wasm stack
		// when start executing this control frame minus params for the block.
		originalStackLenWithoutParam int
		// blk is the loop header if this is loop, and is the else-block if this is an if frame.
		blk,
		// followingBlock is the basic block we enter if we reach "end" of block.
		followingBlock ssa.BasicBlock
		blockType *wasm.FunctionType
		// clonedArgs hold the arguments to Else block.
		clonedArgs []ssa.Value
	}

	controlFrameKind byte
)

// String implements fmt.Stringer for debugging.
func (l *loweringState) String() string {
	var str []string
	for _, v := range l.values {
		str = append(str, fmt.Sprintf("v%v", v.ID()))
	}
	return strings.Join(str, ", ")
}

const (
	controlFrameKindFunction = iota + 1
	controlFrameKindLoop
	controlFrameKindIfWithElse
	controlFrameKindIfWithoutElse
	controlFrameKindBlock
)

// isLoop returns true if this is a loop frame.
func (ctrl *controlFrame) isLoop() bool {
	return ctrl.kind == controlFrameKindLoop
}

// reset resets the state of loweringState for reuse.
func (l *loweringState) reset() {
	l.values = l.values[:0]
	l.controlFrames = l.controlFrames[:0]
	l.pc = 0
	l.unreachable = false
	l.unreachableDepth = 0
}

func (l *loweringState) peek() (ret ssa.Value) {
	tail := len(l.values) - 1
	return l.values[tail]
}

func (l *loweringState) pop() (ret ssa.Value) {
	tail := len(l.values) - 1
	ret = l.values[tail]
	l.values = l.values[:tail]
	return
}

func (l *loweringState) push(ret ssa.Value) {
	l.values = append(l.values, ret)
}

func (l *loweringState) nPopInto(n int, dst []ssa.Value) {
	if n == 0 {
		return
	}
	tail := len(l.values)
	begin := tail - n
	view := l.values[begin:tail]
	copy(dst, view)
	l.values = l.values[:begin]
}

func (l *loweringState) nPeekDup(n int) []ssa.Value {
	if n == 0 {
		return nil
	}
	tail := len(l.values)
	view := l.values[tail-n : tail]
	return cloneValuesList(view)
}

func (l *loweringState) ctrlPop() (ret controlFrame) {
	tail := len(l.controlFrames) - 1
	ret = l.controlFrames[tail]
	l.controlFrames = l.controlFrames[:tail]
	return
}

func (l *loweringState) ctrlPush(ret controlFrame) {
	l.controlFrames = append(l.controlFrames, ret)
}

func (l *loweringState) ctrlPeekAt(n int) (ret *controlFrame) {
	tail := len(l.controlFrames) - 1
	return &l.controlFrames[tail-n]
}

const debug = false

// lowerBody lowers the body of the Wasm function to the SSA form.
func (c *Compiler) lowerBody(entryBlk ssa.BasicBlock) {
	c.ssaBuilder.Seal(entryBlk)

	// Pushes the empty control frame which corresponds to the function return.
	c.loweringState.ctrlPush(controlFrame{
		kind:           controlFrameKindFunction,
		blockType:      c.wasmFunctionTyp,
		followingBlock: c.ssaBuilder.ReturnBlock(),
	})

	for c.loweringState.pc < len(c.wasmFunctionBody) {
		op := c.wasmFunctionBody[c.loweringState.pc]
		c.lowerOpcode(op)
		if debug {
			fmt.Println("--------- Translated " + wasm.InstructionName(op) + " --------")
			fmt.Println("Stack: " + c.loweringState.String())
			fmt.Println(c.formatBuilder())
			fmt.Println("--------------------------")
		}
		c.loweringState.pc++
	}
}

func (c *Compiler) state() *loweringState {
	return &c.loweringState
}

func (c *Compiler) lowerOpcode(op wasm.Opcode) {
	builder := c.ssaBuilder
	state := c.state()
	switch op {
	case wasm.OpcodeI32Const:
		c := c.readI32s()
		if state.unreachable {
			return
		}

		iconst := builder.AllocateInstruction()
		iconst.AsIconst32(uint32(c))
		builder.InsertInstruction(iconst)
		value := iconst.Return()
		state.push(value)
	case wasm.OpcodeI64Const:
		c := c.readI64s()
		if state.unreachable {
			return
		}
		iconst := builder.AllocateInstruction()
		iconst.AsIconst64(uint64(c))
		builder.InsertInstruction(iconst)
		value := iconst.Return()
		state.push(value)
	case wasm.OpcodeF32Const:
		if state.unreachable {
			return
		}
		f32const := builder.AllocateInstruction()
		f32const.AsF32const(c.readF32())
		builder.InsertInstruction(f32const)
		value := f32const.Return()
		state.push(value)
	case wasm.OpcodeF64Const:
		if state.unreachable {
			return
		}
		f64const := builder.AllocateInstruction()
		f64const.AsF64const(c.readF64())
		builder.InsertInstruction(f64const)
		value := f64const.Return()
		state.push(value)
	case wasm.OpcodeI32Add, wasm.OpcodeI64Add:
		if state.unreachable {
			return
		}
		y, x := state.pop(), state.pop()
		iadd := builder.AllocateInstruction()
		iadd.AsIadd(x, y)
		builder.InsertInstruction(iadd)
		value := iadd.Return()
		state.push(value)
	case wasm.OpcodeI32Sub, wasm.OpcodeI64Sub:
		if state.unreachable {
			return
		}
		y, x := state.pop(), state.pop()
		isub := builder.AllocateInstruction()
		isub.AsIsub(x, y)
		builder.InsertInstruction(isub)
		value := isub.Return()
		state.push(value)
	case wasm.OpcodeF32Add, wasm.OpcodeF64Add:
		if state.unreachable {
			return
		}
		y, x := state.pop(), state.pop()
		iadd := builder.AllocateInstruction()
		iadd.AsFadd(x, y)
		builder.InsertInstruction(iadd)
		value := iadd.Return()
		state.push(value)
	case wasm.OpcodeI32Mul, wasm.OpcodeI64Mul:
		if state.unreachable {
			return
		}
		y, x := state.pop(), state.pop()
		imul := builder.AllocateInstruction()
		imul.AsImul(x, y)
		builder.InsertInstruction(imul)
		value := imul.Return()
		state.push(value)
	case wasm.OpcodeF32Sub, wasm.OpcodeF64Sub:
		if state.unreachable {
			return
		}
		y, x := state.pop(), state.pop()
		isub := builder.AllocateInstruction()
		isub.AsFsub(x, y)
		builder.InsertInstruction(isub)
		value := isub.Return()
		state.push(value)
	case wasm.OpcodeF32Mul, wasm.OpcodeF64Mul:
		if state.unreachable {
			return
		}
		y, x := state.pop(), state.pop()
		isub := builder.AllocateInstruction()
		isub.AsFmul(x, y)
		builder.InsertInstruction(isub)
		value := isub.Return()
		state.push(value)
	case wasm.OpcodeF32Div, wasm.OpcodeF64Div:
		if state.unreachable {
			return
		}
		y, x := state.pop(), state.pop()
		isub := builder.AllocateInstruction()
		isub.AsFdiv(x, y)
		builder.InsertInstruction(isub)
		value := isub.Return()
		state.push(value)
	case wasm.OpcodeF32Max, wasm.OpcodeF64Max:
		if state.unreachable {
			return
		}
		y, x := state.pop(), state.pop()
		isub := builder.AllocateInstruction()
		isub.AsFmax(x, y)
		builder.InsertInstruction(isub)
		value := isub.Return()
		state.push(value)
	case wasm.OpcodeF32Min, wasm.OpcodeF64Min:
		if state.unreachable {
			return
		}
		y, x := state.pop(), state.pop()
		isub := builder.AllocateInstruction()
		isub.AsFmin(x, y)
		builder.InsertInstruction(isub)
		value := isub.Return()
		state.push(value)
	case wasm.OpcodeI64Extend8S:
		if state.unreachable {
			return
		}
		c.insertIntegerExtend(true, 8, 64)
	case wasm.OpcodeI64Extend16S:
		if state.unreachable {
			return
		}
		c.insertIntegerExtend(true, 16, 64)
	case wasm.OpcodeI64Extend32S, wasm.OpcodeI64ExtendI32S:
		if state.unreachable {
			return
		}
		c.insertIntegerExtend(true, 32, 64)
	case wasm.OpcodeI64ExtendI32U:
		if state.unreachable {
			return
		}
		c.insertIntegerExtend(false, 32, 64)
	case wasm.OpcodeI32Extend8S:
		if state.unreachable {
			return
		}
		c.insertIntegerExtend(true, 8, 32)
	case wasm.OpcodeI32Extend16S:
		if state.unreachable {
			return
		}
		c.insertIntegerExtend(true, 16, 32)
	case wasm.OpcodeI32Eq, wasm.OpcodeI64Eq:
		if state.unreachable {
			return
		}
		c.insertIcmp(ssa.IntegerCmpCondEqual)
	case wasm.OpcodeI32Ne, wasm.OpcodeI64Ne:
		if state.unreachable {
			return
		}
		c.insertIcmp(ssa.IntegerCmpCondNotEqual)
	case wasm.OpcodeI32LtS, wasm.OpcodeI64LtS:
		if state.unreachable {
			return
		}
		c.insertIcmp(ssa.IntegerCmpCondSignedLessThan)
	case wasm.OpcodeI32LtU, wasm.OpcodeI64LtU:
		if state.unreachable {
			return
		}
		c.insertIcmp(ssa.IntegerCmpCondUnsignedLessThan)
	case wasm.OpcodeI32GtS, wasm.OpcodeI64GtS:
		if state.unreachable {
			return
		}
		c.insertIcmp(ssa.IntegerCmpCondSignedGreaterThan)
	case wasm.OpcodeI32GtU, wasm.OpcodeI64GtU:
		if state.unreachable {
			return
		}
		c.insertIcmp(ssa.IntegerCmpCondUnsignedGreaterThan)
	case wasm.OpcodeI32LeS, wasm.OpcodeI64LeS:
		if state.unreachable {
			return
		}
		c.insertIcmp(ssa.IntegerCmpCondSignedLessThanOrEqual)
	case wasm.OpcodeI32LeU, wasm.OpcodeI64LeU:
		if state.unreachable {
			return
		}
		c.insertIcmp(ssa.IntegerCmpCondUnsignedLessThanOrEqual)
	case wasm.OpcodeI32GeS, wasm.OpcodeI64GeS:
		if state.unreachable {
			return
		}
		c.insertIcmp(ssa.IntegerCmpCondSignedGreaterThanOrEqual)
	case wasm.OpcodeI32GeU, wasm.OpcodeI64GeU:
		if state.unreachable {
			return
		}
		c.insertIcmp(ssa.IntegerCmpCondUnsignedGreaterThanOrEqual)

	case wasm.OpcodeF32Eq, wasm.OpcodeF64Eq:
		if state.unreachable {
			return
		}
		c.insertFcmp(ssa.FloatCmpCondEqual)
	case wasm.OpcodeF32Ne, wasm.OpcodeF64Ne:
		if state.unreachable {
			return
		}
		c.insertFcmp(ssa.FloatCmpCondNotEqual)
	case wasm.OpcodeF32Lt, wasm.OpcodeF64Lt:
		if state.unreachable {
			return
		}
		c.insertFcmp(ssa.FloatCmpCondLessThan)
	case wasm.OpcodeF32Gt, wasm.OpcodeF64Gt:
		if state.unreachable {
			return
		}
		c.insertFcmp(ssa.FloatCmpCondGreaterThan)
	case wasm.OpcodeF32Le, wasm.OpcodeF64Le:
		if state.unreachable {
			return
		}
		c.insertFcmp(ssa.FloatCmpCondLessThanOrEqual)
	case wasm.OpcodeF32Ge, wasm.OpcodeF64Ge:
		if state.unreachable {
			return
		}
		c.insertFcmp(ssa.FloatCmpCondGreaterThanOrEqual)
	case wasm.OpcodeI32Shl, wasm.OpcodeI64Shl:
		if state.unreachable {
			return
		}
		y, x := state.pop(), state.pop()
		ishl := builder.AllocateInstruction()
		ishl.AsIshl(x, y)
		builder.InsertInstruction(ishl)
		value := ishl.Return()
		state.push(value)
	case wasm.OpcodeI32ShrU, wasm.OpcodeI64ShrU:
		if state.unreachable {
			return
		}
		y, x := state.pop(), state.pop()
		ishl := builder.AllocateInstruction()
		ishl.AsUshr(x, y)
		builder.InsertInstruction(ishl)
		value := ishl.Return()
		state.push(value)
	case wasm.OpcodeI32ShrS, wasm.OpcodeI64ShrS:
		if state.unreachable {
			return
		}
		y, x := state.pop(), state.pop()
		ishl := builder.AllocateInstruction()
		ishl.AsSshr(x, y)
		builder.InsertInstruction(ishl)
		value := ishl.Return()
		state.push(value)
	case wasm.OpcodeI32Clz, wasm.OpcodeI64Clz:
		if state.unreachable {
			return
		}
		x := state.pop()
		clz := builder.AllocateInstruction()
		clz.AsClz(x)
		builder.InsertInstruction(clz)
		value := clz.Return()
		state.push(value)
	case wasm.OpcodeI32Ctz, wasm.OpcodeI64Ctz:
		if state.unreachable {
			return
		}
		x := state.pop()
		ctz := builder.AllocateInstruction()
		ctz.AsCtz(x)
		builder.InsertInstruction(ctz)
		value := ctz.Return()
		state.push(value)
	case wasm.OpcodeGlobalGet:
		index := c.readI32u()
		if state.unreachable {
			return
		}
		v := c.getWasmGlobalValue(index, false)
		state.push(v)
	case wasm.OpcodeGlobalSet:
		index := c.readI32u()
		if state.unreachable {
			return
		}
		v := state.pop()
		c.setWasmGlobalValue(index, v)
	case wasm.OpcodeLocalGet:
		index := c.readI32u()
		if state.unreachable {
			return
		}
		variable := c.localVariable(index)
		v := builder.MustFindValue(variable)
		state.push(v)
	case wasm.OpcodeLocalSet:
		index := c.readI32u()
		if state.unreachable {
			return
		}
		variable := c.localVariable(index)
		newValue := state.pop()
		builder.DefineVariableInCurrentBB(variable, newValue)

	case wasm.OpcodeLocalTee:
		index := c.readI32u()
		if state.unreachable {
			return
		}
		variable := c.localVariable(index)
		newValue := state.peek()
		builder.DefineVariableInCurrentBB(variable, newValue)

	case wasm.OpcodeSelect, wasm.OpcodeTypedSelect:
		if op == wasm.OpcodeTypedSelect {
			state.pc += 2 // ignores the type which is only needed during validation.
		}

		if state.unreachable {
			return
		}

		cond := state.pop()
		v2 := state.pop()
		v1 := state.pop()

		sl := builder.AllocateInstruction()
		sl.AsSelect(cond, v1, v2)
		builder.InsertInstruction(sl)
		state.push(sl.Return())

	case wasm.OpcodeMemorySize:
		state.pc++ // skips the memory index.
		if state.unreachable {
			return
		}

		var memSizeInBytes ssa.Value
		if c.offset.LocalMemoryBegin < 0 {
			loadMemInstPtr := builder.AllocateInstruction()
			loadMemInstPtr.AsLoad(c.moduleCtxPtrValue, c.offset.ImportedMemoryBegin.U32(), ssa.TypeI64)
			builder.InsertInstruction(loadMemInstPtr)
			memInstPtr := loadMemInstPtr.Return()

			loadBufSizePtr := builder.AllocateInstruction()
			loadBufSizePtr.AsLoad(memInstPtr, memoryInstanceBufSizeOffset, ssa.TypeI64)
			builder.InsertInstruction(loadBufSizePtr)
			memSizeInBytes = loadBufSizePtr.Return()

		} else {
			load := builder.AllocateInstruction()
			load.AsLoad(c.moduleCtxPtrValue, c.offset.LocalMemoryLen().U32(), ssa.TypeI32)
			builder.InsertInstruction(load)
			memSizeInBytes = load.Return()
		}

		amount := builder.AllocateInstruction()
		amount.AsIconst32(uint32(wasm.MemoryPageSizeInBits))
		builder.InsertInstruction(amount)
		memSize := builder.AllocateInstruction()
		memSize.AsUshr(memSizeInBytes, amount.Return())
		builder.InsertInstruction(memSize)
		state.push(memSize.Return())

	case wasm.OpcodeMemoryGrow:
		state.pc++ // skips the memory index.
		if state.unreachable {
			return
		}

		c.storeCallerModuleContext()

		pages := state.pop()
		loadPtr := builder.AllocateInstruction()
		loadPtr.AsLoad(c.execCtxPtrValue,
			wazevoapi.ExecutionContextOffsets.MemoryGrowTrampolineAddress.U32(), ssa.TypeI64)
		builder.InsertInstruction(loadPtr)

		// TODO: reuse the slice.
		args := []ssa.Value{c.execCtxPtrValue, pages}
		callGrow := builder.AllocateInstruction()
		callGrow.AsCallIndirect(loadPtr.Return(), &c.memoryGrowSig, args)
		builder.InsertInstruction(callGrow)

		state.push(callGrow.Return())

		// After the memory grow, reload the cached memory base and len.
		c.reloadMemoryBaseLen()

	case wasm.OpcodeI32Store,
		wasm.OpcodeI64Store,
		wasm.OpcodeF32Store,
		wasm.OpcodeF64Store,
		wasm.OpcodeI32Store8,
		wasm.OpcodeI32Store16,
		wasm.OpcodeI64Store8,
		wasm.OpcodeI64Store16,
		wasm.OpcodeI64Store32:

		_, offset := c.readMemArg()
		if state.unreachable {
			return
		}

		var opSize uint64
		var opcode ssa.Opcode
		switch op {
		case wasm.OpcodeI32Store, wasm.OpcodeF32Store:
			opcode = ssa.OpcodeStore
			opSize = 4
		case wasm.OpcodeI64Store, wasm.OpcodeF64Store:
			opcode = ssa.OpcodeStore
			opSize = 8
		case wasm.OpcodeI32Store8, wasm.OpcodeI64Store8:
			opcode = ssa.OpcodeIstore8
			opSize = 1
		case wasm.OpcodeI32Store16, wasm.OpcodeI64Store16:
			opcode = ssa.OpcodeIstore16
			opSize = 2
		case wasm.OpcodeI64Store32:
			opcode = ssa.OpcodeIstore32
			opSize = 4
		default:
			panic("BUG")
		}

		value := state.pop()
		baseAddr := state.pop()
		addr := c.memOpSetup(baseAddr, uint64(offset), opSize)

		store := builder.AllocateInstruction()
		store.AsStore(opcode, value, addr, offset)
		builder.InsertInstruction(store)

	case wasm.OpcodeI32Load,
		wasm.OpcodeI64Load,
		wasm.OpcodeF32Load,
		wasm.OpcodeF64Load,
		wasm.OpcodeI32Load8S,
		wasm.OpcodeI32Load8U,
		wasm.OpcodeI32Load16S,
		wasm.OpcodeI32Load16U,
		wasm.OpcodeI64Load8S,
		wasm.OpcodeI64Load8U,
		wasm.OpcodeI64Load16S,
		wasm.OpcodeI64Load16U,
		wasm.OpcodeI64Load32S,
		wasm.OpcodeI64Load32U:
		_, offset := c.readMemArg()
		if state.unreachable {
			return
		}

		var opSize uint64
		switch op {
		case wasm.OpcodeI32Load, wasm.OpcodeF32Load:
			opSize = 4
		case wasm.OpcodeI64Load, wasm.OpcodeF64Load:
			opSize = 8
		case wasm.OpcodeI32Load8S, wasm.OpcodeI32Load8U:
			opSize = 1
		case wasm.OpcodeI32Load16S, wasm.OpcodeI32Load16U:
			opSize = 2
		case wasm.OpcodeI64Load8S, wasm.OpcodeI64Load8U:
			opSize = 1
		case wasm.OpcodeI64Load16S, wasm.OpcodeI64Load16U:
			opSize = 2
		case wasm.OpcodeI64Load32S, wasm.OpcodeI64Load32U:
			opSize = 4
		default:
			panic("BUG")
		}

		baseAddr := state.pop()
		addr := c.memOpSetup(baseAddr, uint64(offset), opSize)
		load := builder.AllocateInstruction()
		switch op {
		case wasm.OpcodeI32Load:
			load.AsLoad(addr, offset, ssa.TypeI32)
		case wasm.OpcodeI64Load:
			load.AsLoad(addr, offset, ssa.TypeI64)
		case wasm.OpcodeF32Load:
			load.AsLoad(addr, offset, ssa.TypeF32)
		case wasm.OpcodeF64Load:
			load.AsLoad(addr, offset, ssa.TypeF64)
		case wasm.OpcodeI32Load8S:
			load.AsExtLoad(ssa.OpcodeSload8, addr, offset, false)
		case wasm.OpcodeI32Load8U:
			load.AsExtLoad(ssa.OpcodeUload8, addr, offset, false)
		case wasm.OpcodeI32Load16S:
			load.AsExtLoad(ssa.OpcodeSload16, addr, offset, false)
		case wasm.OpcodeI32Load16U:
			load.AsExtLoad(ssa.OpcodeUload16, addr, offset, false)
		case wasm.OpcodeI64Load8S:
			load.AsExtLoad(ssa.OpcodeSload8, addr, offset, true)
		case wasm.OpcodeI64Load8U:
			load.AsExtLoad(ssa.OpcodeUload8, addr, offset, true)
		case wasm.OpcodeI64Load16S:
			load.AsExtLoad(ssa.OpcodeSload16, addr, offset, true)
		case wasm.OpcodeI64Load16U:
			load.AsExtLoad(ssa.OpcodeUload16, addr, offset, true)
		case wasm.OpcodeI64Load32S:
			load.AsExtLoad(ssa.OpcodeSload32, addr, offset, true)
		case wasm.OpcodeI64Load32U:
			load.AsExtLoad(ssa.OpcodeUload32, addr, offset, true)
		default:
			panic("BUG")
		}
		builder.InsertInstruction(load)
		state.push(load.Return())
	case wasm.OpcodeBlock:
		// Note: we do not need to create a BB for this as that would always have only one predecessor
		// which is the current BB, and therefore it's always ok to merge them in any way.

		bt := c.readBlockType()

		if state.unreachable {
			state.unreachableDepth++
			return
		}

		followingBlk := builder.AllocateBasicBlock()
		c.addBlockParamsFromWasmTypes(bt.Results, followingBlk)

		state.ctrlPush(controlFrame{
			kind:                         controlFrameKindBlock,
			originalStackLenWithoutParam: len(state.values) - len(bt.Params),
			followingBlock:               followingBlk,
			blockType:                    bt,
		})
	case wasm.OpcodeLoop:
		bt := c.readBlockType()

		if state.unreachable {
			state.unreachableDepth++
			return
		}

		loopHeader, afterLoopBlock := builder.AllocateBasicBlock(), builder.AllocateBasicBlock()
		c.addBlockParamsFromWasmTypes(bt.Params, loopHeader)
		c.addBlockParamsFromWasmTypes(bt.Results, afterLoopBlock)

		originalLen := len(state.values) - len(bt.Params)
		state.ctrlPush(controlFrame{
			originalStackLenWithoutParam: originalLen,
			kind:                         controlFrameKindLoop,
			blk:                          loopHeader,
			followingBlock:               afterLoopBlock,
			blockType:                    bt,
		})

		var args []ssa.Value
		if len(bt.Params) > 0 {
			args = cloneValuesList(state.values[originalLen:])
		}

		// Insert the jump to the header of loop.
		br := builder.AllocateInstruction()
		br.AsJump(args, loopHeader)
		builder.InsertInstruction(br)

		c.switchTo(originalLen, loopHeader)

	case wasm.OpcodeIf:
		bt := c.readBlockType()

		if state.unreachable {
			state.unreachableDepth++
			return
		}

		v := state.pop()
		thenBlk, elseBlk, followingBlk := builder.AllocateBasicBlock(), builder.AllocateBasicBlock(), builder.AllocateBasicBlock()

		// We do not make the Wasm-level block parameters as SSA-level block params for if-else blocks
		// since they won't be PHI and the definition is unique.

		// On the other hand, the following block after if-else-end will likely have
		// multiple definitions (one in Then and another in Else blocks).
		c.addBlockParamsFromWasmTypes(bt.Results, followingBlk)

		var args []ssa.Value
		if len(bt.Params) > 0 {
			args = cloneValuesList(state.values[len(state.values)-1-len(bt.Params):])
		}

		// Insert the conditional jump to the Else block.
		brz := builder.AllocateInstruction()
		brz.AsBrz(v, nil, elseBlk)
		builder.InsertInstruction(brz)

		// Then, insert the jump to the Then block.
		br := builder.AllocateInstruction()
		br.AsJump(nil, thenBlk)
		builder.InsertInstruction(br)

		state.ctrlPush(controlFrame{
			kind:                         controlFrameKindIfWithoutElse,
			originalStackLenWithoutParam: len(state.values) - len(bt.Params),
			blk:                          elseBlk,
			followingBlock:               followingBlk,
			blockType:                    bt,
			clonedArgs:                   args,
		})

		builder.SetCurrentBlock(thenBlk)

		// Then and Else (if exists) have only one predecessor.
		builder.Seal(thenBlk)
		builder.Seal(elseBlk)
	case wasm.OpcodeElse:
		ifctrl := state.ctrlPeekAt(0)
		ifctrl.kind = controlFrameKindIfWithElse

		if unreachable := state.unreachable; unreachable && state.unreachableDepth > 0 {
			// If it is currently in unreachable and is a nested if,
			// we just remove the entire else block.
			return
		}

		if !state.unreachable {
			// If this Then block is currently reachable, we have to insert the branching to the following BB.
			followingBlk := ifctrl.followingBlock // == the BB after if-then-else.
			args := c.loweringState.nPeekDup(len(ifctrl.blockType.Results))
			c.insertJumpToBlock(args, followingBlk)
		} else {
			state.unreachable = false
		}

		// Reset the stack so that we can correctly handle the else block.
		state.values = state.values[:ifctrl.originalStackLenWithoutParam]
		elseBlk := ifctrl.blk
		for _, arg := range ifctrl.clonedArgs {
			state.push(arg)
		}

		builder.SetCurrentBlock(elseBlk)

	case wasm.OpcodeEnd:
		ctrl := state.ctrlPop()
		followingBlk := ctrl.followingBlock

		if !state.unreachable {
			// Top n-th args will be used as a result of the current control frame.
			args := c.loweringState.nPeekDup(len(ctrl.blockType.Results))

			// Insert the unconditional branch to the target.
			c.insertJumpToBlock(args, followingBlk)
		} else { // unreachable.
			if state.unreachableDepth > 0 {
				state.unreachableDepth--
				return
			} else {
				state.unreachable = false
			}
		}

		switch ctrl.kind {
		case controlFrameKindFunction:
			return // This is the very end of function.
		case controlFrameKindLoop:
			// Loop header block can be reached from any br/br_table contained in the loop,
			// so now that we've reached End of it, we can seal it.
			builder.Seal(ctrl.blk)
		case controlFrameKindIfWithoutElse:
			// If this is the end of Then block, we have to emit the empty Else block.
			elseBlk := ctrl.blk
			builder.SetCurrentBlock(elseBlk)
			c.insertJumpToBlock(nil, followingBlk)
		}

		builder.Seal(ctrl.followingBlock)

		// Ready to start translating the following block.
		c.switchTo(ctrl.originalStackLenWithoutParam, followingBlk)

	case wasm.OpcodeBr:
		labelIndex := c.readI32u()
		if state.unreachable {
			return
		}

		targetFrame := state.ctrlPeekAt(int(labelIndex))
		var targetBlk ssa.BasicBlock
		var argNum int
		if targetFrame.isLoop() {
			targetBlk, argNum = targetFrame.blk, len(targetFrame.blockType.Params)
		} else {
			targetBlk, argNum = targetFrame.followingBlock, len(targetFrame.blockType.Results)
		}
		args := c.loweringState.nPeekDup(argNum)
		c.insertJumpToBlock(args, targetBlk)

		state.unreachable = true

	case wasm.OpcodeBrIf:
		labelIndex := c.readI32u()
		if state.unreachable {
			return
		}

		v := state.pop()

		targetFrame := state.ctrlPeekAt(int(labelIndex))
		var targetBlk ssa.BasicBlock
		var argNum int
		if targetFrame.isLoop() {
			targetBlk, argNum = targetFrame.blk, len(targetFrame.blockType.Params)
		} else {
			targetBlk, argNum = targetFrame.followingBlock, len(targetFrame.blockType.Results)
		}
		args := c.loweringState.nPeekDup(argNum)

		// Insert the conditional jump to the target block.
		brnz := builder.AllocateInstruction()
		brnz.AsBrnz(v, args, targetBlk)
		builder.InsertInstruction(brnz)

		// Insert the unconditional jump to the Else block which corresponds to after br_if.
		elseBlk := builder.AllocateBasicBlock()
		c.insertJumpToBlock(nil, elseBlk)

		// Now start translating the instructions after br_if.
		builder.SetCurrentBlock(elseBlk)

	case wasm.OpcodeNop:
	case wasm.OpcodeReturn:
		results := c.loweringState.nPeekDup(c.results())
		instr := builder.AllocateInstruction()

		instr.AsReturn(results)
		builder.InsertInstruction(instr)
		state.unreachable = true

	case wasm.OpcodeUnreachable:
		exit := builder.AllocateInstruction()
		exit.AsExitWithCode(c.execCtxPtrValue, wazevoapi.ExitCodeUnreachable)
		builder.InsertInstruction(exit)
		state.unreachable = true

	case wasm.OpcodeCallIndirect:
		typeIndex := c.readI32u()
		tableIndex := c.readI32u()
		if state.unreachable {
			return
		}
		c.lowerCallIndirect(typeIndex, tableIndex)

	case wasm.OpcodeCall:
		fnIndex := c.readI32u()
		if state.unreachable {
			return
		}

		// Before transfer the control to the callee, we have to store the current module's moduleContextPtr
		// into execContext.callerModuleContextPtr in case when the callee is a Go function.
		//
		// TODO: maybe this can be optimized out if this is in-module function calls. Investigate later.
		c.storeCallerModuleContext()

		var typIndex wasm.Index
		if fnIndex < c.m.ImportFunctionCount {
			var fi int
			for i := range c.m.ImportSection {
				imp := &c.m.ImportSection[i]
				if imp.Type == wasm.ExternTypeFunc {
					if fi == int(fnIndex) {
						typIndex = imp.DescFunc
						break
					}
					fi++
				}
			}
		} else {
			fnIndex -= c.m.ImportFunctionCount
			typIndex = c.m.FunctionSection[fnIndex]
		}
		typ := &c.m.TypeSection[typIndex]

		// TODO: reuse slice?
		argN := len(typ.Params)
		args := make([]ssa.Value, argN+2)
		args[0] = c.execCtxPtrValue
		state.nPopInto(argN, args[2:])

		sig := c.signatures[typ]
		call := builder.AllocateInstruction()
		if fnIndex >= c.m.ImportFunctionCount {
			args[1] = c.moduleCtxPtrValue // This case the callee module is itself.
			call.AsCall(FunctionIndexToFuncRef(fnIndex), sig, args)
			builder.InsertInstruction(call)
		} else {
			// This case we have to read the address of the imported function from the module context.
			moduleCtx := c.moduleCtxPtrValue
			loadFuncPtr, loadModuleCtxPtr := builder.AllocateInstruction(), builder.AllocateInstruction()
			funcPtrOffset, moduleCtxPtrOffset, _ := c.offset.ImportedFunctionOffset(fnIndex)
			loadFuncPtr.AsLoad(moduleCtx, funcPtrOffset.U32(), ssa.TypeI64)
			loadModuleCtxPtr.AsLoad(moduleCtx, moduleCtxPtrOffset.U32(), ssa.TypeI64)
			builder.InsertInstruction(loadFuncPtr)
			builder.InsertInstruction(loadModuleCtxPtr)

			args[1] = loadModuleCtxPtr.Return() // This case the callee module is itself.

			call.AsCallIndirect(loadFuncPtr.Return(), sig, args)
			builder.InsertInstruction(call)
		}

		first, rest := call.Returns()
		if first.Valid() {
			state.push(first)
		}
		for _, v := range rest {
			state.push(v)
		}

		c.reloadAfterCall()

	case wasm.OpcodeDrop:
		_ = state.pop()
	default:
		panic("TODO: unsupported in wazevo yet: " + wasm.InstructionName(op))
	}
}

const (
	tableInstanceBaseAddressOffset = 0
	tableInstanceLenOffset         = tableInstanceBaseAddressOffset + 8
)

func (c *Compiler) lowerCallIndirect(typeIndex, tableIndex uint32) {
	builder := c.ssaBuilder
	state := c.state()

	targetOffsetInTable := state.pop()

	// Load the table.
	tableOffset := c.offset.TableOffset(int(tableIndex))
	loadTableInstancePtr := builder.AllocateInstruction()
	loadTableInstancePtr.AsLoad(c.moduleCtxPtrValue, tableOffset.U32(), ssa.TypeI64)
	builder.InsertInstruction(loadTableInstancePtr)
	tableInstancePtr := loadTableInstancePtr.Return()

	// Load the table's length.
	loadTableLen := builder.AllocateInstruction()
	loadTableLen.AsLoad(tableInstancePtr, tableInstanceLenOffset, ssa.TypeI32)
	builder.InsertInstruction(loadTableLen)
	tableLen := loadTableLen.Return()

	// Compare the length and the target, and trap if out of bounds.
	checkOOB := builder.AllocateInstruction()
	checkOOB.AsIcmp(targetOffsetInTable, tableLen, ssa.IntegerCmpCondUnsignedGreaterThanOrEqual)
	builder.InsertInstruction(checkOOB)
	exitIfOOB := builder.AllocateInstruction()
	exitIfOOB.AsExitIfTrueWithCode(c.execCtxPtrValue, checkOOB.Return(), wazevoapi.ExitCodeTableOutOfBounds)
	builder.InsertInstruction(exitIfOOB)

	// Get the base address of wasm.TableInstance.References.
	loadTableBaseAddress := builder.AllocateInstruction()
	loadTableBaseAddress.AsLoad(tableInstancePtr, tableInstanceBaseAddressOffset, ssa.TypeI64)
	builder.InsertInstruction(loadTableBaseAddress)
	tableBase := loadTableBaseAddress.Return()

	// Calculate the address of the target function. First we need to multiply targetOffsetInTable by 8 (pointer size).
	multiplyBy8 := builder.AllocateInstruction()
	three := builder.AllocateInstruction()
	three.AsIconst64(3)
	builder.InsertInstruction(three)
	multiplyBy8.AsIshl(targetOffsetInTable, three.Return())
	builder.InsertInstruction(multiplyBy8)
	targetOffsetInTableMultipliedBy8 := multiplyBy8.Return()
	// Then add the multiplied value to the base which results in the address of the target function (*wazevo.functionInstance)
	calcFunctionInstancePtrAddressInTable := builder.AllocateInstruction()
	calcFunctionInstancePtrAddressInTable.AsIadd(tableBase, targetOffsetInTableMultipliedBy8)
	builder.InsertInstruction(calcFunctionInstancePtrAddressInTable)
	functionInstancePtrAddress := calcFunctionInstancePtrAddressInTable.Return()
	loadFunctionInstancePtr := builder.AllocateInstruction()
	loadFunctionInstancePtr.AsLoad(functionInstancePtrAddress, 0, ssa.TypeI64)
	builder.InsertInstruction(loadFunctionInstancePtr)
	functionInstancePtr := loadFunctionInstancePtr.Return()

	// Check if it is not the null pointer.
	zero := builder.AllocateInstruction()
	zero.AsIconst64(0)
	builder.InsertInstruction(zero)
	checkNull := builder.AllocateInstruction()
	checkNull.AsIcmp(functionInstancePtr, zero.Return(), ssa.IntegerCmpCondEqual)
	builder.InsertInstruction(checkNull)
	exitIfNull := builder.AllocateInstruction()
	exitIfNull.AsExitIfTrueWithCode(c.execCtxPtrValue, checkNull.Return(), wazevoapi.ExitCodeIndirectCallNullPointer)
	builder.InsertInstruction(exitIfNull)

	// We need to do the type check. First, load the target function instance's typeID.
	loadTypeID := builder.AllocateInstruction()
	loadTypeID.AsLoad(functionInstancePtr, wazevoapi.FunctionInstanceTypeIDOffset, ssa.TypeI32)
	builder.InsertInstruction(loadTypeID)
	actualTypeID := loadTypeID.Return()

	// Next, we load the expected TypeID:
	loadTypeIDsBegin := builder.AllocateInstruction()
	loadTypeIDsBegin.AsLoad(c.moduleCtxPtrValue, c.offset.TypeIDs1stElement.U32(), ssa.TypeI64)
	builder.InsertInstruction(loadTypeIDsBegin)
	typeIDsBegin := loadTypeIDsBegin.Return()

	loadExpectedTypeID := builder.AllocateInstruction()
	loadExpectedTypeID.AsLoad(typeIDsBegin, uint32(typeIndex)*4 /* size of wasm.FunctionTypeID */, ssa.TypeI32)
	builder.InsertInstruction(loadExpectedTypeID)
	expectedTypeID := loadExpectedTypeID.Return()

	// Check if the type ID matches.
	checkTypeID := builder.AllocateInstruction()
	checkTypeID.AsIcmp(actualTypeID, expectedTypeID, ssa.IntegerCmpCondNotEqual)
	builder.InsertInstruction(checkTypeID)
	exitIfNotMatch := builder.AllocateInstruction()
	exitIfNotMatch.AsExitIfTrueWithCode(c.execCtxPtrValue, checkTypeID.Return(), wazevoapi.ExitCodeIndirectCallTypeMismatch)
	builder.InsertInstruction(exitIfNotMatch)

	// Now ready to call the function. Load the executable and moduleContextOpaquePtr from the function instance.
	loadExecutablePtr := builder.AllocateInstruction()
	loadExecutablePtr.AsLoad(functionInstancePtr, wazevoapi.FunctionInstanceExecutableOffset, ssa.TypeI64)
	builder.InsertInstruction(loadExecutablePtr)
	executablePtr := loadExecutablePtr.Return()
	loadModuleContextOpaquePtr := builder.AllocateInstruction()
	loadModuleContextOpaquePtr.AsLoad(functionInstancePtr, wazevoapi.FunctionInstanceModuleContextOpaquePtrOffset, ssa.TypeI64)
	builder.InsertInstruction(loadModuleContextOpaquePtr)
	moduleContextOpaquePtr := loadModuleContextOpaquePtr.Return()

	// TODO: reuse slice?
	typ := &c.m.TypeSection[typeIndex]
	argN := len(typ.Params)
	args := make([]ssa.Value, argN+2)
	args[0] = c.execCtxPtrValue
	args[1] = moduleContextOpaquePtr
	state.nPopInto(argN, args[2:])

	// Before transfer the control to the callee, we have to store the current module's moduleContextPtr
	// into execContext.callerModuleContextPtr in case when the callee is a Go function.
	c.storeCallerModuleContext()

	call := builder.AllocateInstruction()
	call.AsCallIndirect(executablePtr, c.signatures[typ], args)
	builder.InsertInstruction(call)

	first, rest := call.Returns()
	if first.Valid() {
		state.push(first)
	}
	for _, v := range rest {
		state.push(v)
	}

	c.reloadAfterCall()
}

// memOpSetup inserts the bounds check and calculates the address of the memory operation (loads/stores).
func (c *Compiler) memOpSetup(baseAddr ssa.Value, constOffset, operationSizeInBytes uint64) (address ssa.Value) {
	builder := c.ssaBuilder

	ceil := constOffset + operationSizeInBytes
	ceilConst := builder.AllocateInstruction()
	ceilConst.AsIconst64(ceil)
	builder.InsertInstruction(ceilConst)

	// We calculate the offset in 64-bit space.
	extBaseAddr := builder.AllocateInstruction()
	extBaseAddr.AsUExtend(baseAddr, 32, 64)
	builder.InsertInstruction(extBaseAddr)

	// Note: memLen is already zero extended to 64-bit space at the load time.
	memLen := c.getMemoryLenValue(false)

	// baseAddrPlusCeil = baseAddr + ceil
	baseAddrPlusCeil := builder.AllocateInstruction()
	baseAddrPlusCeil.AsIadd(extBaseAddr.Return(), ceilConst.Return())
	builder.InsertInstruction(baseAddrPlusCeil)

	// Check for out of bounds memory access: `memLen >= baseAddrPlusCeil`.
	cmp := builder.AllocateInstruction()
	cmp.AsIcmp(memLen, baseAddrPlusCeil.Return(), ssa.IntegerCmpCondUnsignedLessThan)
	builder.InsertInstruction(cmp)
	exitIfNZ := builder.AllocateInstruction()
	exitIfNZ.AsExitIfTrueWithCode(c.execCtxPtrValue, cmp.Return(), wazevoapi.ExitCodeMemoryOutOfBounds)
	builder.InsertInstruction(exitIfNZ)

	// Load the value from memBase + extBaseAddr.
	memBase := c.getMemoryBaseValue(false)
	addrCalc := builder.AllocateInstruction()
	addrCalc.AsIadd(memBase, extBaseAddr.Return())
	builder.InsertInstruction(addrCalc)
	return addrCalc.Return()
}

func (c *Compiler) reloadAfterCall() {
	// Note that when these are not used in the following instructions, they will be optimized out.
	// So in any ways, we define them!

	// After calling any function, memory buffer might have changed. So we need to re-defined the variable.
	if c.needMemory {
		c.reloadMemoryBaseLen()
	}

	// Also, any mutable Global can change.
	for _, index := range c.mutableGlobalVariablesIndexes {
		_ = c.getWasmGlobalValue(index, true)
	}
}

func (c *Compiler) reloadMemoryBaseLen() {
	_ = c.getMemoryBaseValue(true)
	_ = c.getMemoryLenValue(true)
}

// globalInstanceValueOffset is the offsetOf .Value field of wasm.GlobalInstance.
const globalInstanceValueOffset = 8

func (c *Compiler) setWasmGlobalValue(index wasm.Index, v ssa.Value) {
	variable := c.globalVariables[index]
	instanceOffset := c.offset.GlobalInstanceOffset(index)

	builder := c.ssaBuilder
	loadGlobalInstPtr := builder.AllocateInstruction()
	loadGlobalInstPtr.AsLoad(c.moduleCtxPtrValue, uint32(instanceOffset), ssa.TypeI64)
	builder.InsertInstruction(loadGlobalInstPtr)

	store := builder.AllocateInstruction()
	store.AsStore(ssa.OpcodeStore, v, loadGlobalInstPtr.Return(), uint32(globalInstanceValueOffset))
	builder.InsertInstruction(store)

	// The value has changed to `v`, so we record it.
	builder.DefineVariableInCurrentBB(variable, v)
}

func (c *Compiler) getWasmGlobalValue(index wasm.Index, forceLoad bool) ssa.Value {
	variable := c.globalVariables[index]
	typ := c.globalVariablesTypes[index]
	instanceOffset := c.offset.GlobalInstanceOffset(index)

	builder := c.ssaBuilder
	if !forceLoad {
		if v := builder.FindValue(variable); v.Valid() {
			return v
		}
	}

	loadGlobalInstPtr := builder.AllocateInstruction()
	loadGlobalInstPtr.AsLoad(c.moduleCtxPtrValue, uint32(instanceOffset), ssa.TypeI64)
	builder.InsertInstruction(loadGlobalInstPtr)

	load := builder.AllocateInstruction()
	load.AsLoad(loadGlobalInstPtr.Return(), uint32(globalInstanceValueOffset), typ)
	builder.InsertInstruction(load)
	ret := load.Return()
	builder.DefineVariableInCurrentBB(variable, ret)
	return ret
}

const (
	memoryInstanceBufOffset     = 0
	memoryInstanceBufSizeOffset = memoryInstanceBufOffset + 8
)

func (c *Compiler) getMemoryBaseValue(forceReload bool) ssa.Value {
	builder := c.ssaBuilder
	variable := c.memoryBaseVariable
	if !forceReload {
		if v := builder.FindValue(variable); v.Valid() {
			return v
		}
	}

	var ret ssa.Value
	if c.offset.LocalMemoryBegin < 0 {
		loadMemInstPtr := builder.AllocateInstruction()
		loadMemInstPtr.AsLoad(c.moduleCtxPtrValue, c.offset.ImportedMemoryBegin.U32(), ssa.TypeI64)
		builder.InsertInstruction(loadMemInstPtr)
		memInstPtr := loadMemInstPtr.Return()

		loadBufPtr := builder.AllocateInstruction()
		loadBufPtr.AsLoad(memInstPtr, memoryInstanceBufOffset, ssa.TypeI64)
		builder.InsertInstruction(loadBufPtr)
		ret = loadBufPtr.Return()
	} else {
		load := builder.AllocateInstruction()
		load.AsLoad(c.moduleCtxPtrValue, c.offset.LocalMemoryBase().U32(), ssa.TypeI64)
		builder.InsertInstruction(load)
		ret = load.Return()
	}

	builder.DefineVariableInCurrentBB(variable, ret)
	return ret
}

func (c *Compiler) getMemoryLenValue(forceReload bool) ssa.Value {
	variable := c.memoryLenVariable
	builder := c.ssaBuilder
	if !forceReload {
		if v := builder.FindValue(variable); v.Valid() {
			return v
		}
	}

	var ret ssa.Value
	if c.offset.LocalMemoryBegin < 0 {
		loadMemInstPtr := builder.AllocateInstruction()
		loadMemInstPtr.AsLoad(c.moduleCtxPtrValue, c.offset.ImportedMemoryBegin.U32(), ssa.TypeI64)
		builder.InsertInstruction(loadMemInstPtr)
		memInstPtr := loadMemInstPtr.Return()

		loadBufSizePtr := builder.AllocateInstruction()
		loadBufSizePtr.AsLoad(memInstPtr, memoryInstanceBufSizeOffset, ssa.TypeI64)
		builder.InsertInstruction(loadBufSizePtr)

		ret = loadBufSizePtr.Return()
	} else {
		load := builder.AllocateInstruction()
		load.AsExtLoad(ssa.OpcodeUload32, c.moduleCtxPtrValue, c.offset.LocalMemoryLen().U32(), true)
		builder.InsertInstruction(load)
		ret = load.Return()
	}

	builder.DefineVariableInCurrentBB(variable, ret)
	return ret
}

func (c *Compiler) insertIcmp(cond ssa.IntegerCmpCond) {
	state, builder := c.state(), c.ssaBuilder
	y, x := state.pop(), state.pop()
	cmp := builder.AllocateInstruction()
	cmp.AsIcmp(x, y, cond)
	builder.InsertInstruction(cmp)
	value := cmp.Return()
	state.push(value)
}

func (c *Compiler) insertFcmp(cond ssa.FloatCmpCond) {
	state, builder := c.state(), c.ssaBuilder
	y, x := state.pop(), state.pop()
	cmp := builder.AllocateInstruction()
	cmp.AsFcmp(x, y, cond)
	builder.InsertInstruction(cmp)
	value := cmp.Return()
	state.push(value)
}

// storeCallerModuleContext stores the current module's moduleContextPtr into execContext.callerModuleContextPtr.
func (c *Compiler) storeCallerModuleContext() {
	builder := c.ssaBuilder
	execCtx := c.execCtxPtrValue
	store := builder.AllocateInstruction()
	store.AsStore(ssa.OpcodeStore,
		c.moduleCtxPtrValue, execCtx, wazevoapi.ExecutionContextOffsets.CallerModuleContextPtr.U32())
	builder.InsertInstruction(store)
}

func (c *Compiler) readI32u() uint32 {
	v, n, err := leb128.LoadUint32(c.wasmFunctionBody[c.loweringState.pc+1:])
	if err != nil {
		panic(err) // shouldn't be reached since compilation comes after validation.
	}
	c.loweringState.pc += int(n)
	return v
}

func (c *Compiler) readI32s() int32 {
	v, n, err := leb128.LoadInt32(c.wasmFunctionBody[c.loweringState.pc+1:])
	if err != nil {
		panic(err) // shouldn't be reached since compilation comes after validation.
	}
	c.loweringState.pc += int(n)
	return v
}

func (c *Compiler) readI64s() int64 {
	v, n, err := leb128.LoadInt64(c.wasmFunctionBody[c.loweringState.pc+1:])
	if err != nil {
		panic(err) // shouldn't be reached since compilation comes after validation.
	}
	c.loweringState.pc += int(n)
	return v
}

func (c *Compiler) readF32() float32 {
	v := math.Float32frombits(binary.LittleEndian.Uint32(c.wasmFunctionBody[c.loweringState.pc+1:]))
	c.loweringState.pc += 4
	return v
}

func (c *Compiler) readF64() float64 {
	v := math.Float64frombits(binary.LittleEndian.Uint64(c.wasmFunctionBody[c.loweringState.pc+1:]))
	c.loweringState.pc += 8
	return v
}

// readBlockType reads the block type from the current position of the bytecode reader.
func (c *Compiler) readBlockType() *wasm.FunctionType {
	state := c.state()

	c.br.Reset(c.wasmFunctionBody[state.pc+1:])
	bt, num, err := wasm.DecodeBlockType(c.m.TypeSection, c.br, api.CoreFeaturesV2)
	if err != nil {
		panic(err) // shouldn't be reached since compilation comes after validation.
	}
	state.pc += int(num)

	return bt
}

func (c *Compiler) readMemArg() (align, offset uint32) {
	state := c.state()

	align, num, err := leb128.LoadUint32(c.wasmFunctionBody[state.pc+1:])
	if err != nil {
		panic(fmt.Errorf("read memory align: %v", err))
	}

	state.pc += int(num)
	offset, num, err = leb128.LoadUint32(c.wasmFunctionBody[state.pc+1:])
	if err != nil {
		panic(fmt.Errorf("read memory offset: %v", err))
	}

	state.pc += int(num)
	return align, offset
}

// insertJumpToBlock inserts a jump instruction to the given block in the current block.
func (c *Compiler) insertJumpToBlock(args []ssa.Value, targetBlk ssa.BasicBlock) {
	builder := c.ssaBuilder
	jmp := builder.AllocateInstruction()
	jmp.AsJump(args, targetBlk)
	builder.InsertInstruction(jmp)
}

func (c *Compiler) insertIntegerExtend(signed bool, from, to byte) {
	state := c.state()
	builder := c.ssaBuilder
	v := state.pop()
	extend := builder.AllocateInstruction()
	if signed {
		extend.AsSExtend(v, from, to)
	} else {
		extend.AsUExtend(v, from, to)
	}
	builder.InsertInstruction(extend)
	value := extend.Return()
	state.push(value)
}

func (c *Compiler) switchTo(originalStackLen int, targetBlk ssa.BasicBlock) {
	if targetBlk.Preds() == 0 {
		c.loweringState.unreachable = true
	}

	// Now we should adjust the stack and start translating the continuation block.
	c.loweringState.values = c.loweringState.values[:originalStackLen]

	c.ssaBuilder.SetCurrentBlock(targetBlk)

	// At this point, blocks params consist only of the Wasm-level parameters,
	// (since it's added only when we are trying to resolve variable *inside* this block).
	for i := 0; i < targetBlk.Params(); i++ {
		value := targetBlk.Param(i)
		c.loweringState.push(value)
	}
}

// cloneValuesList clones the given values list.
func cloneValuesList(in []ssa.Value) (ret []ssa.Value) {
	ret = make([]ssa.Value, len(in))
	for i := range ret {
		ret[i] = in[i]
	}
	return
}

// results returns the number of results of the current function.
func (c *Compiler) results() int {
	return len(c.wasmFunctionTyp.Results)
}
