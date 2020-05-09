package wasm

const (
	initialOperandStackHeight = 1024
	initialLabelStackHeight   = 10
)

func drop(vm *VirtualMachine) {
	vm.OperandStack.Drop()
}

func selectOp(vm *VirtualMachine) {
	c := vm.OperandStack.Pop()
	v2 := vm.OperandStack.Pop()
	if c == 0 {
		_ = vm.OperandStack.Pop()
		vm.OperandStack.Push(v2)
	}
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
	Arity                 int
	ContinuationPC, EndPC uint64
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
