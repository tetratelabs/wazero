package naivevm

import (
	"github.com/tetratelabs/wazero/wasm"
	"github.com/tetratelabs/wazero/wasm/buildoptions"
)

const (
	initialOperandStackHeight = 1024
	initialLabelStackHeight   = 10
)

var callStackHeightLimit = buildoptions.CallStackHeightLimit

func drop(vm *naiveVirtualMachine) {
	vm.operands.drop()
	vm.activeFrame.pc++
}

func selectOp(vm *naiveVirtualMachine) {
	c := vm.operands.pop()
	v2 := vm.operands.pop()
	if c == 0 {
		_ = vm.operands.pop()
		vm.operands.push(v2)
	}
	vm.activeFrame.pc++
}

func newOperandStack() *operandStack {
	return &operandStack{
		stack: make([]uint64, initialOperandStackHeight),
		sp:    -1,
	}
}

type operandStack struct {
	stack []uint64
	sp    int
}

func (s *operandStack) pop() uint64 {
	ret := s.stack[s.sp]
	s.sp--
	return ret
}

func (s *operandStack) drop() {
	s.sp--
}

func (s *operandStack) peek() uint64 {
	return s.stack[s.sp]
}

func (s *operandStack) push(val uint64) {
	if s.sp+1 == len(s.stack) {
		// grow stack
		s.stack = append(s.stack, val)
	} else {
		s.stack[s.sp+1] = val
	}
	s.sp++
}

func (s *operandStack) pushBool(b bool) {
	if b {
		s.push(1)
	} else {
		s.push(0)
	}
}

type labelStack struct {
	stack []*label
	sp    int
}

type label struct {
	arity          int
	continuationPC uint64
	operandSP      int
}

func newLabelStack() *labelStack {
	return &labelStack{
		stack: make([]*label, initialLabelStackHeight),
		sp:    -1,
	}
}

func (s *labelStack) pop() *label {
	ret := s.stack[s.sp]
	s.sp--
	return ret
}

func (s *labelStack) push(val *label) {
	if s.sp+1 == len(s.stack) {
		// grow stack
		s.stack = append(s.stack, val)
	} else {
		s.stack[s.sp+1] = val
	}
	s.sp++
}

type frameStack struct {
	stack []*frame
	sp    int
}

type frame struct {
	pc     uint64
	locals []uint64
	f      *wasm.FunctionInstance
	labels *labelStack
}

func newFrameStack() *frameStack {
	return &frameStack{
		stack: make([]*frame, initialLabelStackHeight),
		sp:    -1,
	}
}

func (s *frameStack) peek() *frame {
	if s.sp < 0 {
		return nil
	}
	ret := s.stack[s.sp]
	return ret
}

func (s *frameStack) pop() *frame {
	ret := s.stack[s.sp]
	s.sp--
	return ret
}

func (s *frameStack) push(val *frame) {
	if s.sp+1 == len(s.stack) {
		if callStackHeightLimit <= s.sp {
			panic(wasm.ErrCallStackOverflow)
		}
		// grow stack
		s.stack = append(s.stack, val)
	} else {
		s.stack[s.sp+1] = val
	}
	s.sp++
}
