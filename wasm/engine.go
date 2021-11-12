package wasm

const PageSize uint64 = 65536

type Engine interface {
	Call(f *FunctionInstance, args ...uint64) (returns []uint64, err error)
	Compile(f *FunctionInstance) error
}

type NopEngine struct{}

func (n *NopEngine) Call(f *FunctionInstance, args ...uint64) (returns []uint64, err error) {
	return nil, nil
}

func (n *NopEngine) Compile(f *FunctionInstance) error {
	return nil
}
