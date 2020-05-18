package wasm

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type NumTestSuite struct {
	suite.Suite
	vm *VirtualMachine
}

func (suite *NumTestSuite) SetupTest() {
	suite.vm = &VirtualMachine{
		OperandStack: NewVirtualMachineOperandStack(),
	}
}

func (suite *NumTestSuite) Testi32eqz() {
	suite.vm.OperandStack.Push(0)
	i32eqz(suite.vm)
	suite.Equal(uint64(1), suite.vm.OperandStack.Pop())
}

func (suite *NumTestSuite) Testi32ne() {
	var testTable = []struct {
		input [2]int
		want  uint64
	}{
		{input: [2]int{3, 4}, want: 1},
		{input: [2]int{3, 3}, want: 0},
	}
	for _, tt := range testTable {
		suite.vm.OperandStack.Push(uint64(tt.input[0]))
		suite.vm.OperandStack.Push(uint64(tt.input[1]))
		i32ne(suite.vm)
		suite.Equal(tt.want, suite.vm.OperandStack.Pop())
	}
}

func (suite *NumTestSuite) Testi32lts() {
	var testTable = []struct {
		input [2]int
		want  uint64
	}{
		{input: [2]int{-4, 1}, want: 1},
		{input: [2]int{4, -1}, want: 0},
	}
	for _, tt := range testTable {
		suite.vm.OperandStack.Push(uint64(tt.input[0]))
		suite.vm.OperandStack.Push(uint64(tt.input[1]))
		i32lts(suite.vm)
		suite.Equal(tt.want, suite.vm.OperandStack.Pop())
	}
}

func (suite *NumTestSuite) Testi32ltu() {
	var testTable = []struct {
		input [2]int
		want  uint64
	}{
		{input: [2]int{1, 4}, want: 1},
		{input: [2]int{4, 1}, want: 0},
	}
	for _, tt := range testTable {
		suite.vm.OperandStack.Push(uint64(tt.input[0]))
		suite.vm.OperandStack.Push(uint64(tt.input[1]))
		i32ltu(suite.vm)
		suite.Equal(tt.want, suite.vm.OperandStack.Pop())
	}
}

func (suite *NumTestSuite) Testi32gts() {
	var testTable = []struct {
		input [2]int
		want  uint64
	}{
		{input: [2]int{1, -4}, want: 1},
		{input: [2]int{-4, 1}, want: 0},
	}
	for _, tt := range testTable {
		suite.vm.OperandStack.Push(uint64(tt.input[0]))
		suite.vm.OperandStack.Push(uint64(tt.input[1]))
		i32gts(suite.vm)
		suite.Equal(tt.want, suite.vm.OperandStack.Pop())
	}
}

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestRunSuite(t *testing.T) {
	suite.Run(t, new(NumTestSuite))
}
