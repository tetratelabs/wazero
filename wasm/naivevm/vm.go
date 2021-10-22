package naivevm

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"time"

	"github.com/mathetake/gasm/wasm"
	"github.com/mathetake/gasm/wasm/buildoptions"
	"github.com/mathetake/gasm/wasm/leb128"
)

type (
	naiveVirtualMachine struct {
		activeFrame *frame
		frames      *frameStack
		operands    *operandStack
	}
)

var _ wasm.Engine = &naiveVirtualMachine{}

func NewEngine() wasm.Engine {
	return &naiveVirtualMachine{
		frames:   newFrameStack(),
		operands: newOperandStack(),
	}
}

func (vm *naiveVirtualMachine) Call(f *wasm.FunctionInstance, args ...uint64) (returns []uint64, err error) {
	for _, arg := range args {
		vm.operands.push(arg)
	}

	if err := vm.execFunction(f); err != nil {
		return nil, err
	}

	ret := make([]uint64, len(f.Signature.ReturnTypes))
	for i := range ret {
		ret[len(ret)-1-i] = vm.operands.pop()
	}
	return ret, nil
}

func (vm *naiveVirtualMachine) execFunction(f *wasm.FunctionInstance) (errRet error) {
	al := len(f.Signature.InputTypes)
	locals := make([]uint64, f.NumLocals+uint32(al))
	for i := 0; i < al; i++ {
		locals[al-1-i] = vm.operands.pop()
	}
	frame := &frame{
		f:      f,
		locals: locals,
		labels: newLabelStack(),
	}
	frame.labels.push(&label{
		arity:          len(f.Signature.ReturnTypes),
		continuationPC: uint64(len(f.Body)) - 1, // At return.
		operandSP:      -1,
	})

	prevFrameSP := vm.frames.sp
	defer func() {
		if v := recover(); v != nil {
			// Stack Unwind.
			// TODO: include stack trace in the error message.
			vm.frames.sp = prevFrameSP
			err, ok := v.(error)
			if ok {
				errRet = err
			} else {
				errRet = fmt.Errorf("runtime error: %v", v)
			}
		}
	}()

	vm.frames.push(frame)
	vm.activeFrame = frame
	for vm.activeFrame != nil {
		if buildoptions.IsDebugMode {
			fmt.Printf("0x%x: op=%s (Label SP=%d, Operand SP=%d, Frame SP=%d) \n",
				vm.activeFrame.pc, buildoptions.OptcodeStrs[vm.activeFrame.f.Body[vm.activeFrame.pc]],
				vm.activeFrame.labels.sp, vm.operands.sp, vm.frames.sp)
			time.Sleep(time.Millisecond)
		}
		virtualMachineInstructions[vm.activeFrame.f.Body[vm.activeFrame.pc]](vm)
	}
	return
}

func (vm *naiveVirtualMachine) FetchInt32() int32 {
	ret, num, err := leb128.DecodeInt32(bytes.NewBuffer(
		vm.activeFrame.f.Body[vm.activeFrame.pc:]))
	if err != nil {
		panic(err)
	}
	vm.activeFrame.pc += num - 1
	return ret
}

func (vm *naiveVirtualMachine) FetchUint32() uint32 {
	ret, num, err := leb128.DecodeUint32(bytes.NewBuffer(
		vm.activeFrame.f.Body[vm.activeFrame.pc:]))
	if err != nil {
		panic(err)
	}
	vm.activeFrame.pc += num - 1
	return ret
}

func (vm *naiveVirtualMachine) FetchInt64() int64 {
	ret, num, err := leb128.DecodeInt64(bytes.NewBuffer(
		vm.activeFrame.f.Body[vm.activeFrame.pc:]))
	if err != nil {
		panic(err)
	}
	vm.activeFrame.pc += num - 1
	return ret
}

func (vm *naiveVirtualMachine) FetchFloat32() float32 {
	v := math.Float32frombits(binary.LittleEndian.Uint32(
		vm.activeFrame.f.Body[vm.activeFrame.pc:]))
	vm.activeFrame.pc += 3
	return v
}

func (vm *naiveVirtualMachine) FetchFloat64() float64 {
	v := math.Float64frombits(binary.LittleEndian.Uint64(
		vm.activeFrame.f.Body[vm.activeFrame.pc:]))
	vm.activeFrame.pc += 7
	return v
}

var virtualMachineInstructions = [256]func(vm *naiveVirtualMachine){
	wasm.OptCodeUnreachable:       func(vm *naiveVirtualMachine) { panic("unreachable") },
	wasm.OptCodeNop:               func(vm *naiveVirtualMachine) { vm.activeFrame.pc++ },
	wasm.OptCodeBlock:             block,
	wasm.OptCodeLoop:              loop,
	wasm.OptCodeIf:                ifOp,
	wasm.OptCodeElse:              elseOp,
	wasm.OptCodeEnd:               end,
	wasm.OptCodeBr:                br,
	wasm.OptCodeBrIf:              brIf,
	wasm.OptCodeBrTable:           brTable,
	wasm.OptCodeReturn:            returnOp,
	wasm.OptCodeCall:              call,
	wasm.OptCodeCallIndirect:      callIndirect,
	wasm.OptCodeDrop:              drop,
	wasm.OptCodeSelect:            selectOp,
	wasm.OptCodeLocalGet:          getLocal,
	wasm.OptCodeLocalSet:          setLocal,
	wasm.OptCodeLocalTee:          teeLocal,
	wasm.OptCodeGlobalGet:         getGlobal,
	wasm.OptCodeGlobalSet:         setGlobal,
	wasm.OptCodeI32Load:           i32Load,
	wasm.OptCodeI64Load:           i64Load,
	wasm.OptCodeF32Load:           f32Load,
	wasm.OptCodeF64Load:           f64Load,
	wasm.OptCodeI32Load8s:         i32Load8s,
	wasm.OptCodeI32Load8u:         i32Load8u,
	wasm.OptCodeI32Load16s:        i32Load16s,
	wasm.OptCodeI32Load16u:        i32Load16u,
	wasm.OptCodeI64Load8s:         i64Load8s,
	wasm.OptCodeI64Load8u:         i64Load8u,
	wasm.OptCodeI64Load16s:        i64Load16s,
	wasm.OptCodeI64Load16u:        i64Load16u,
	wasm.OptCodeI64Load32s:        i64Load32s,
	wasm.OptCodeI64Load32u:        i64Load32u,
	wasm.OptCodeI32Store:          i32Store,
	wasm.OptCodeI64Store:          i64Store,
	wasm.OptCodeF32Store:          f32Store,
	wasm.OptCodeF64Store:          f64Store,
	wasm.OptCodeI32Store8:         i32Store8,
	wasm.OptCodeI32Store16:        i32Store16,
	wasm.OptCodeI64Store8:         i64Store8,
	wasm.OptCodeI64Store16:        i64Store16,
	wasm.OptCodeI64Store32:        i64Store32,
	wasm.OptCodeMemorySize:        memorySize,
	wasm.OptCodeMemoryGrow:        memoryGrow,
	wasm.OptCodeI32Const:          i32Const,
	wasm.OptCodeI64Const:          i64Const,
	wasm.OptCodeF32Const:          f32Const,
	wasm.OptCodeF64Const:          f64Const,
	wasm.OptCodeI32eqz:            i32eqz,
	wasm.OptCodeI32eq:             i32eq,
	wasm.OptCodeI32ne:             i32ne,
	wasm.OptCodeI32lts:            i32lts,
	wasm.OptCodeI32ltu:            i32ltu,
	wasm.OptCodeI32gts:            i32gts,
	wasm.OptCodeI32gtu:            i32gtu,
	wasm.OptCodeI32les:            i32les,
	wasm.OptCodeI32leu:            i32leu,
	wasm.OptCodeI32ges:            i32ges,
	wasm.OptCodeI32geu:            i32geu,
	wasm.OptCodeI64eqz:            i64eqz,
	wasm.OptCodeI64eq:             i64eq,
	wasm.OptCodeI64ne:             i64ne,
	wasm.OptCodeI64lts:            i64lts,
	wasm.OptCodeI64ltu:            i64ltu,
	wasm.OptCodeI64gts:            i64gts,
	wasm.OptCodeI64gtu:            i64gtu,
	wasm.OptCodeI64les:            i64les,
	wasm.OptCodeI64leu:            i64leu,
	wasm.OptCodeI64ges:            i64ges,
	wasm.OptCodeI64geu:            i64geu,
	wasm.OptCodeF32eq:             f32eq,
	wasm.OptCodeF32ne:             f32ne,
	wasm.OptCodeF32lt:             f32lt,
	wasm.OptCodeF32gt:             f32gt,
	wasm.OptCodeF32le:             f32le,
	wasm.OptCodeF32ge:             f32ge,
	wasm.OptCodeF64eq:             f64eq,
	wasm.OptCodeF64ne:             f64ne,
	wasm.OptCodeF64lt:             f64lt,
	wasm.OptCodeF64gt:             f64gt,
	wasm.OptCodeF64le:             f64le,
	wasm.OptCodeF64ge:             f64ge,
	wasm.OptCodeI32clz:            i32clz,
	wasm.OptCodeI32ctz:            i32ctz,
	wasm.OptCodeI32popcnt:         i32popcnt,
	wasm.OptCodeI32add:            i32add,
	wasm.OptCodeI32sub:            i32sub,
	wasm.OptCodeI32mul:            i32mul,
	wasm.OptCodeI32divs:           i32divs,
	wasm.OptCodeI32divu:           i32divu,
	wasm.OptCodeI32rems:           i32rems,
	wasm.OptCodeI32remu:           i32remu,
	wasm.OptCodeI32and:            i32and,
	wasm.OptCodeI32or:             i32or,
	wasm.OptCodeI32xor:            i32xor,
	wasm.OptCodeI32shl:            i32shl,
	wasm.OptCodeI32shrs:           i32shrs,
	wasm.OptCodeI32shru:           i32shru,
	wasm.OptCodeI32rotl:           i32rotl,
	wasm.OptCodeI32rotr:           i32rotr,
	wasm.OptCodeI64clz:            i64clz,
	wasm.OptCodeI64ctz:            i64ctz,
	wasm.OptCodeI64popcnt:         i64popcnt,
	wasm.OptCodeI64add:            i64add,
	wasm.OptCodeI64sub:            i64sub,
	wasm.OptCodeI64mul:            i64mul,
	wasm.OptCodeI64divs:           i64divs,
	wasm.OptCodeI64divu:           i64divu,
	wasm.OptCodeI64rems:           i64rems,
	wasm.OptCodeI64remu:           i64remu,
	wasm.OptCodeI64and:            i64and,
	wasm.OptCodeI64or:             i64or,
	wasm.OptCodeI64xor:            i64xor,
	wasm.OptCodeI64shl:            i64shl,
	wasm.OptCodeI64shrs:           i64shrs,
	wasm.OptCodeI64shru:           i64shru,
	wasm.OptCodeI64rotl:           i64rotl,
	wasm.OptCodeI64rotr:           i64rotr,
	wasm.OptCodeF32abs:            f32abs,
	wasm.OptCodeF32neg:            f32neg,
	wasm.OptCodeF32ceil:           f32ceil,
	wasm.OptCodeF32floor:          f32floor,
	wasm.OptCodeF32trunc:          f32trunc,
	wasm.OptCodeF32nearest:        f32nearest,
	wasm.OptCodeF32sqrt:           f32sqrt,
	wasm.OptCodeF32add:            f32add,
	wasm.OptCodeF32sub:            f32sub,
	wasm.OptCodeF32mul:            f32mul,
	wasm.OptCodeF32div:            f32div,
	wasm.OptCodeF32min:            f32min,
	wasm.OptCodeF32max:            f32max,
	wasm.OptCodeF32copysign:       f32copysign,
	wasm.OptCodeF64abs:            f64abs,
	wasm.OptCodeF64neg:            f64neg,
	wasm.OptCodeF64ceil:           f64ceil,
	wasm.OptCodeF64floor:          f64floor,
	wasm.OptCodeF64trunc:          f64trunc,
	wasm.OptCodeF64nearest:        f64nearest,
	wasm.OptCodeF64sqrt:           f64sqrt,
	wasm.OptCodeF64add:            f64add,
	wasm.OptCodeF64sub:            f64sub,
	wasm.OptCodeF64mul:            f64mul,
	wasm.OptCodeF64div:            f64div,
	wasm.OptCodeF64min:            f64min,
	wasm.OptCodeF64max:            f64max,
	wasm.OptCodeF64copysign:       f64copysign,
	wasm.OptCodeI32wrapI64:        i32wrapi64,
	wasm.OptCodeI32truncf32s:      i32truncf32s,
	wasm.OptCodeI32truncf32u:      i32truncf32u,
	wasm.OptCodeI32truncf64s:      i32truncf64s,
	wasm.OptCodeI32truncf64u:      i32truncf64u,
	wasm.OptCodeI64Extendi32s:     i64extendi32s,
	wasm.OptCodeI64Extendi32u:     i64extendi32u,
	wasm.OptCodeI64TruncF32s:      i64truncf32s,
	wasm.OptCodeI64TruncF32u:      i64truncf32u,
	wasm.OptCodeI64Truncf64s:      i64truncf64s,
	wasm.OptCodeI64Truncf64u:      i64truncf64u,
	wasm.OptCodeF32Converti32s:    f32converti32s,
	wasm.OptCodeF32Converti32u:    f32converti32u,
	wasm.OptCodeF32Converti64s:    f32converti64s,
	wasm.OptCodeF32Converti64u:    f32converti64u,
	wasm.OptCodeF32Demotef64:      f32demotef64,
	wasm.OptCodeF64Converti32s:    f64converti32s,
	wasm.OptCodeF64Converti32u:    f64converti32u,
	wasm.OptCodeF64Converti64s:    f64converti64s,
	wasm.OptCodeF64Converti64u:    f64converti64u,
	wasm.OptCodeF64Promotef32:     f64promotef32,
	wasm.OptCodeI32reinterpretf32: func(vm *naiveVirtualMachine) { vm.activeFrame.pc++ },
	wasm.OptCodeI64reinterpretf64: func(vm *naiveVirtualMachine) { vm.activeFrame.pc++ },
	wasm.OptCodeF32reinterpreti32: func(vm *naiveVirtualMachine) { vm.activeFrame.pc++ },
	wasm.OptCodeF64reinterpreti64: func(vm *naiveVirtualMachine) { vm.activeFrame.pc++ },
}
