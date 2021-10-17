package wasm

const (
	initialOperandStackHeight = 1024
	initialLabelStackHeight   = 10
)

func drop(vm *VirtualMachine) {
	vm.Operands.Drop()
	vm.ActiveFrame.PC++
}

func selectOp(vm *VirtualMachine) {
	c := vm.Operands.Pop()
	v2 := vm.Operands.Pop()
	if c == 0 {
		_ = vm.Operands.Pop()
		vm.Operands.Push(v2)
	}
	vm.ActiveFrame.PC++
}

func NewVirtualMachineOperandStack() *VirtualMachineOperandStack {
	return &VirtualMachineOperandStack{
		Stack: make([]uint64, initialOperandStackHeight),
		SP:    -1,
	}
}

type VirtualMachineOperandStack struct {
	Stack []uint64
	SP    int
}

func (s *VirtualMachineOperandStack) Pop() uint64 {
	ret := s.Stack[s.SP]
	s.SP--
	return ret
}

func (s *VirtualMachineOperandStack) Drop() {
	s.SP--
}

func (s *VirtualMachineOperandStack) Peek() uint64 {
	return s.Stack[s.SP]
}

func (s *VirtualMachineOperandStack) Push(val uint64) {
	if s.SP+1 == len(s.Stack) {
		// grow stack
		s.Stack = append(s.Stack, val)
	} else {
		s.Stack[s.SP+1] = val
	}
	s.SP++
}

func (s *VirtualMachineOperandStack) PushBool(b bool) {
	if b {
		s.Push(1)
	} else {
		s.Push(0)
	}
}

type VirtualMachineLabelStack struct {
	Stack []*Label
	SP    int
}

type Label struct {
	Arity          int
	ContinuationPC uint64
	OperandSP      int
}

func NewVirtualMachineLabelStack() *VirtualMachineLabelStack {
	return &VirtualMachineLabelStack{
		Stack: make([]*Label, initialLabelStackHeight),
		SP:    -1,
	}
}

func (s *VirtualMachineLabelStack) Pop() *Label {
	ret := s.Stack[s.SP]
	s.SP--
	return ret
}

func (s *VirtualMachineLabelStack) Push(val *Label) {
	if s.SP+1 == len(s.Stack) {
		// grow stack
		s.Stack = append(s.Stack, val)
	} else {
		s.Stack[s.SP+1] = val
	}
	s.SP++
}

type VirtualMachineFrameStack struct {
	Stack []*VirtualMachineFrame
	SP    int
}

type VirtualMachineFrame struct {
	PC     uint64
	Locals []uint64
	F      *FunctionInstance
	Labels *VirtualMachineLabelStack
}

func NewVirtualMachineFrames() *VirtualMachineFrameStack {
	return &VirtualMachineFrameStack{
		Stack: make([]*VirtualMachineFrame, initialLabelStackHeight),
		SP:    -1,
	}
}

func (s *VirtualMachineFrameStack) Peek() *VirtualMachineFrame {
	if s.SP < 0 {
		return nil
	}
	ret := s.Stack[s.SP]
	return ret
}

func (s *VirtualMachineFrameStack) Pop() *VirtualMachineFrame {
	ret := s.Stack[s.SP]
	s.SP--
	return ret
}

func (s *VirtualMachineFrameStack) Push(val *VirtualMachineFrame) {
	if s.SP+1 == len(s.Stack) {
		// grow stack
		s.Stack = append(s.Stack, val)
	} else {
		s.Stack[s.SP+1] = val
	}
	s.SP++
}
