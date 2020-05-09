package hostfunc

import (
	"fmt"
	"reflect"

	"github.com/mathetake/gasm/wasm"
)

type ModuleBuilder struct {
	modules map[string]*wasm.Module
}

func NewModuleBuilder() *ModuleBuilder {
	return &ModuleBuilder{modules: map[string]*wasm.Module{}}
}

func NewModuleBuilderWith(in map[string]*wasm.Module) *ModuleBuilder {
	return &ModuleBuilder{modules: in}
}

func (m *ModuleBuilder) Done() map[string]*wasm.Module {
	return m.modules
}

func (m *ModuleBuilder) MustSetFunction(modName, funcName string, fn func(machine *wasm.VirtualMachine) reflect.Value) {
	if err := m.SetFunction(modName, funcName, fn); err != nil {
		panic(err)
	}
}

func (m *ModuleBuilder) SetFunction(modName, funcName string, fn func(machine *wasm.VirtualMachine) reflect.Value) error {

	mod, ok := m.modules[modName]
	if !ok {
		mod = &wasm.Module{IndexSpace: new(wasm.ModuleIndexSpace), SecExports: map[string]*wasm.ExportSegment{}}
		m.modules[modName] = mod
	}

	mod.SecExports[funcName] = &wasm.ExportSegment{
		Name: funcName,
		Desc: &wasm.ExportDesc{
			Kind:  wasm.ExportKindFunction,
			Index: uint32(len(mod.IndexSpace.Function)),
		},
	}

	sig, err := getSignature(fn(&wasm.VirtualMachine{}).Type())
	if err != nil {
		return fmt.Errorf("invalid signature: %w", err)
	}

	mod.IndexSpace.Function = append(mod.IndexSpace.Function, &wasm.HostFunction{
		ClosureGenerator: fn,
		Signature:        sig,
	})
	return nil
}

func getSignature(p reflect.Type) (*wasm.FunctionType, error) {
	var err error
	in := make([]wasm.ValueType, p.NumIn())
	for i := range in {
		in[i], err = getTypeOf(p.In(i).Kind())
		if err != nil {
			return nil, err
		}
	}

	out := make([]wasm.ValueType, p.NumOut())
	for i := range out {
		out[i], err = getTypeOf(p.Out(i).Kind())
		if err != nil {
			return nil, err
		}
	}
	return &wasm.FunctionType{InputTypes: in, ReturnTypes: out}, nil
}

func getTypeOf(kind reflect.Kind) (wasm.ValueType, error) {
	switch kind {
	case reflect.Float64:
		return wasm.ValueTypeF64, nil
	case reflect.Float32:
		return wasm.ValueTypeF32, nil
	case reflect.Int32, reflect.Uint32:
		return wasm.ValueTypeI32, nil
	case reflect.Int64, reflect.Uint64:
		return wasm.ValueTypeI64, nil
	default:
		return 0x00, fmt.Errorf("invalid type: %s", kind.String())
	}
}
