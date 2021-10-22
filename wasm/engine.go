package wasm

const PageSize uint64 = 65536

type Engine interface {
	Call(f *FunctionInstance, args ...uint64) (returns []uint64, err error)
}
