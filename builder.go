package wazero

import (
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/wasm"
)

// ModuleBuilder is a way to define a WebAssembly 1.0 (20191205) in Go.
//
// Ex. Below defines and instantiates a module named "env" with one function:
//
//	hello := func() {
//		fmt.Fprintln(stdout, "hello!")
//	}
//	_, err := r.NewModuleBuilder("env").ExportFunction("hello", hello).Instantiate()
//
// If the same module may be instantiated multiple times, it is more efficient to separate steps. Ex.
//
//	env, err := r.NewModuleBuilder("env").ExportFunction("get_random_string", getRandomString).Build()
//
//	_, err := r.InstantiateModule(env.WithName("env.1"))
//	_, err := r.InstantiateModule(env.WithName("env.2"))
//
// Note: Builder methods do not return errors, to allow chaining. Any validation errors are deferred until Build.
// Note: Insertion order is not retained. Anything defined by this builder is sorted lexicographically on Build.
type ModuleBuilder interface {

	// ExportFunction adds a function written in Go, which a WebAssembly module can import.
	//
	// * name - the name to export. Ex "random_get"
	// * goFunc - the `func` to export.
	//
	// Noting a context exception described later, all parameters or result types must match WebAssembly 1.0 (20191205) value
	// types. This means uint32, uint64, float32 or float64. Up to one result can be returned.
	//
	// Ex. This is a valid host function:
	//
	//	addInts := func(x uint32, uint32) uint32 {
	//		return x + y
	//	}
	//
	// Host functions may also have an initial parameter (param[0]) of type context.Context or api.Module.
	//
	// Ex. This uses a Go Context:
	//
	//	addInts := func(m context.Context, x uint32, uint32) uint32 {
	//		// add a little extra if we put some in the context!
	//		return x + y + m.Value(extraKey).(uint32)
	//	}
	//
	// The most sophisticated context is api.Module, which allows access to the Go context, but also
	// allows writing to memory. This is important because there are only numeric types in Wasm. The only way to share other
	// data is via writing memory and sharing offsets.
	//
	// Ex. This reads the parameters from!
	//
	//	addInts := func(m api.Module, offset uint32) uint32 {
	//		x, _ := m.Memory().ReadUint32Le(offset)
	//		y, _ := m.Memory().ReadUint32Le(offset + 4) // 32 bits == 4 bytes!
	//		return x + y
	//	}
	//
	// Note: If a function is already exported with the same name, this overwrites it.
	// See https://www.w3.org/TR/2019/REC-wasm-core-1-20191205/#host-functions%E2%91%A2
	ExportFunction(name string, goFunc interface{}) ModuleBuilder

	// ExportFunctions is a convenience that calls ExportFunction for each key/value in the provided map.
	ExportFunctions(nameToGoFunc map[string]interface{}) ModuleBuilder

	// Build returns a module to instantiate, or returns an error if any of the configuration is invalid.
	Build() (*CompiledCode, error)

	// Instantiate is a convenience that calls Build, then Runtime.InstantiateModule
	//
	// Note: Fields in the builder are copied during instantiation: Later changes do not affect the instantiated result.
	Instantiate() (api.Module, error)
}

// moduleBuilder implements ModuleBuilder
type moduleBuilder struct {
	r            *runtime
	moduleName   string
	nameToGoFunc map[string]interface{}
}

// NewModuleBuilder implements Runtime.NewModuleBuilder
func (r *runtime) NewModuleBuilder(moduleName string) ModuleBuilder {
	return &moduleBuilder{
		r:            r,
		moduleName:   moduleName,
		nameToGoFunc: map[string]interface{}{},
	}
}

// ExportFunction implements ModuleBuilder.ExportFunction
func (b *moduleBuilder) ExportFunction(name string, goFunc interface{}) ModuleBuilder {
	b.nameToGoFunc[name] = goFunc
	return b
}

// ExportFunctions implements ModuleBuilder.ExportFunctions
func (b *moduleBuilder) ExportFunctions(nameToGoFunc map[string]interface{}) ModuleBuilder {
	for k, v := range nameToGoFunc {
		b.ExportFunction(k, v)
	}
	return b
}

// Build implements ModuleBuilder.Build
func (b *moduleBuilder) Build() (*CompiledCode, error) {
	// TODO: we can use r.enabledFeatures to fail early on things like mutable globals
	if module, err := wasm.NewHostModule(b.moduleName, b.nameToGoFunc); err != nil {
		return nil, err
	} else {
		return &CompiledCode{module: module}, nil
	}
}

// Instantiate implements ModuleBuilder.Instantiate
func (b *moduleBuilder) Instantiate() (api.Module, error) {
	if module, err := b.Build(); err != nil {
		return nil, err
	} else {
		return b.r.InstantiateModuleWithConfig(module, NewModuleConfig().WithName(b.moduleName))
	}
}
