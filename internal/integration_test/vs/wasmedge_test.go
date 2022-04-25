//go:build amd64 && cgo && wasmedge

// wasmedge depends on manual installation of a shared library, so is guarded by a flag by default.

package vs

import (
	"context"
	"fmt"

	"github.com/second-state/WasmEdge-go/wasmedge"
)

func init() {
	runtimes["WasmEdge-go"] = newWasmedgeRuntime
}

func newWasmedgeRuntime() runtime {
	return &wasmedgeRuntime{}
}

type wasmedgeRuntime struct {
	conf *wasmedge.Configure
}

type wasmedgeModule struct {
	store *wasmedge.Store
	vm    *wasmedge.VM
}

func (r *wasmedgeRuntime) Compile(_ context.Context, _ *runtimeConfig) (err error) {
	r.conf = wasmedge.NewConfigure()
	// We can't re-use a store because "module name conflict" occurs even after releasing a VM
	return
}

func (r *wasmedgeRuntime) Instantiate(_ context.Context, cfg *runtimeConfig) (mod module, err error) {
	m := &wasmedgeModule{store: wasmedge.NewStore()}
	m.vm = wasmedge.NewVMWithConfigAndStore(r.conf, m.store)
	if err = m.vm.RegisterWasmBuffer(cfg.moduleName, cfg.moduleWasm); err != nil {
		return
	}
	if err = m.vm.LoadWasmBuffer(cfg.moduleWasm); err != nil {
		return
	}
	if err = m.vm.Validate(); err != nil {
		return
	}
	if err = m.vm.Instantiate(); err != nil {
		return
	}
	for _, funcName := range cfg.funcNames {
		if fn := m.vm.GetFunctionType(funcName); fn == nil {
			err = fmt.Errorf("%s is not an exported function", funcName)
			return
		}
	}
	mod = m
	return
}

func (r *wasmedgeRuntime) Close(_ context.Context) error {
	if conf := r.conf; conf != nil {
		conf.Release()
	}
	r.conf = nil
	return nil
}

func (m *wasmedgeModule) CallI64_I64(_ context.Context, funcName string, param uint64) (uint64, error) {
	if result, err := m.vm.Execute(funcName, int64(param)); err != nil {
		return 0, err
	} else {
		return uint64(result[0].(int64)), nil
	}
}

func (m *wasmedgeModule) Close(_ context.Context) error {
	if vm := m.vm; vm != nil {
		vm.Release()
	}
	m.vm = nil
	if store := m.store; store != nil {
		store.Release()
	}
	m.store = nil
	return nil
}
