//go:build amd64 && cgo && wasmedge

// wasmedge depends on manual installation of a shared library, so is guarded by a flag by default.

package vs

import (
	"context"
	"fmt"

	"github.com/second-state/WasmEdge-go/wasmedge"
)

func init() {
	runtimeTesters["WasmEdge-go"] = newWasmedgeTester
}

func newWasmedgeTester() runtimeTester {
	return &wasmedgeTester{}
}

type wasmedgeTester struct {
	conf  *wasmedge.Configure
	store *wasmedge.Store
	vm    *wasmedge.VM
}

func (w *wasmedgeTester) Init(_ context.Context, cfg *runtimeConfig) (err error) {
	w.conf = wasmedge.NewConfigure()
	w.store = wasmedge.NewStore()
	w.vm = wasmedge.NewVMWithConfigAndStore(w.conf, w.store)

	if err = w.vm.RegisterWasmBuffer(cfg.moduleName, cfg.moduleWasm); err != nil {
		return
	}
	if err = w.vm.LoadWasmBuffer(cfg.moduleWasm); err != nil {
		return
	}
	if err = w.vm.Validate(); err != nil {
		return
	}
	if err = w.vm.Instantiate(); err != nil {
		return
	}
	for _, funcName := range cfg.funcNames {
		if fn := w.vm.GetFunctionType(funcName); fn == nil {
			return fmt.Errorf("%s is not an exported function", funcName)
		}
	}
	return
}

func (w *wasmedgeTester) CallI64_I64(_ context.Context, funcName string, param uint64) (uint64, error) {
	if result, err := w.vm.Execute(funcName, int64(param)); err != nil {
		return 0, err
	} else {
		return uint64(result[0].(int64)), nil
	}
}

func (w *wasmedgeTester) Close() error {
	for _, closer := range []func(){w.vm.Release, w.store.Release, w.conf.Release} {
		if closer == nil {
			continue
		}
		closer()
	}
	w.vm = nil
	w.store = nil
	w.conf = nil
	return nil
}
