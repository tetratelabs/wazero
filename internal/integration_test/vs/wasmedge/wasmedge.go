//go:build amd64 && cgo && !windows && wasmedge

// Note: WasmEdge depends on manual installation of a shared library.
// e.g. wget -qO- https://raw.githubusercontent.com/WasmEdge/WasmEdge/master/utils/install.sh | \
//     sudo bash -s -- -p /usr/local -e none -v ${WASMEDGE_VERSION}

package wasmedge

import (
	"context"
	"fmt"
	"os"

	"github.com/second-state/WasmEdge-go/wasmedge"

	"github.com/tetratelabs/wazero/internal/integration_test/vs"
)

func newWasmedgeRuntime() vs.Runtime {
	return &wasmedgeRuntime{}
}

type wasmedgeRuntime struct {
	conf  *wasmedge.Configure
	logFn func([]byte) error
}

type wasmedgeModule struct {
	store *wasmedge.Store
	vm    *wasmedge.VM
	env   *wasmedge.Module
}

func (r *wasmedgeRuntime) Name() string {
	return "wasmedge"
}

func (r *wasmedgeRuntime) log(_ interface{}, callframe *wasmedge.CallingFrame, params []interface{}) ([]interface{}, wasmedge.Result) {
	offset := params[0].(int32)
	byteCount := params[1].(int32)
	mem := callframe.GetMemoryByIndex(0)
	buf, err := mem.GetData(uint(offset), uint(byteCount))
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return nil, wasmedge.Result_Fail
	}
	if err = r.logFn(buf); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		panic(err)
	}
	return nil, wasmedge.Result_Success
}

func (r *wasmedgeRuntime) Compile(_ context.Context, cfg *vs.RuntimeConfig) (err error) {
	if cfg.NeedsWASI {
		r.conf = wasmedge.NewConfigure(wasmedge.WASI)
	} else {
		r.conf = wasmedge.NewConfigure()
	}
	// We can't re-use a store because "module name conflict" occurs even after releasing a VM
	return
}

func (r *wasmedgeRuntime) Instantiate(_ context.Context, cfg *vs.RuntimeConfig) (mod vs.Module, err error) {
	m := &wasmedgeModule{store: wasmedge.NewStore()}
	m.vm = wasmedge.NewVMWithConfigAndStore(r.conf, m.store)

	// Instantiate WASI, if configured.
	if cfg.NeedsWASI {
		wasi := m.vm.GetImportModule(wasmedge.WASI)
		wasi.InitWasi(nil, nil, nil)
	}

	// Instantiate the host module, "env", if configured.
	if cfg.LogFn != nil {
		r.logFn = cfg.LogFn
		m.env = wasmedge.NewModule("env")
		logType := wasmedge.NewFunctionType([]wasmedge.ValType{wasmedge.ValType_I32, wasmedge.ValType_I32}, nil)
		m.env.AddFunction("log", wasmedge.NewFunction(logType, r.log, nil, 0))
		if err = m.vm.RegisterModule(m.env); err != nil {
			return nil, err
		}
	} else if cfg.EnvFReturnValue != 0 {
		m.env = wasmedge.NewModule("env")
		fType := wasmedge.NewFunctionType([]wasmedge.ValType{wasmedge.ValType_I64}, []wasmedge.ValType{wasmedge.ValType_I64})
		m.env.AddFunction("f", wasmedge.NewFunction(fType, func(interface{}, *wasmedge.CallingFrame, []interface{}) ([]interface{}, wasmedge.Result) {
			return []interface{}{int64(cfg.EnvFReturnValue)}, wasmedge.Result_Success
		}, nil, 0))
		if err = m.vm.RegisterModule(m.env); err != nil {
			return nil, err
		}
	}

	// Instantiate the module.
	if err = m.vm.RegisterWasmBuffer(cfg.ModuleName, cfg.ModuleWasm); err != nil {
		return
	}
	if err = m.vm.LoadWasmBuffer(cfg.ModuleWasm); err != nil {
		return
	}
	if err = m.vm.Validate(); err != nil {
		return
	}
	if err = m.vm.Instantiate(); err != nil {
		return
	}

	// If WASI is needed, we have to go back and invoke the _start function.
	if cfg.NeedsWASI {
		if _, err = m.vm.Execute("_start"); err != nil {
			return
		}
	}

	// Ensure function exports exist.
	for _, funcName := range cfg.FuncNames {
		if fn := m.vm.GetFunctionType(funcName); fn == nil {
			err = fmt.Errorf("%s is not an exported function", funcName)
			return
		}
	}

	mod = m
	return
}

func (r *wasmedgeRuntime) Close(context.Context) error {
	if conf := r.conf; conf != nil {
		conf.Release()
	}
	r.conf = nil
	return nil
}

func (m *wasmedgeModule) Memory() []byte {
	mod := m.vm.GetActiveModule()
	mem := mod.FindMemory("memory")
	d, err := mem.GetData(0, mem.GetPageSize()*65536)
	if err != nil {
		panic(err)
	}
	return d
}

func (m *wasmedgeModule) CallI32_I32(_ context.Context, funcName string, param uint32) (uint32, error) {
	if result, err := m.vm.Execute(funcName, int32(param)); err != nil {
		return 0, err
	} else {
		return uint32(result[0].(int32)), nil
	}
}

func (m *wasmedgeModule) CallI32I32_V(_ context.Context, funcName string, x, y uint32) (err error) {
	_, err = m.vm.Execute(funcName, int32(x), int32(y))
	return
}

func (m *wasmedgeModule) CallV_V(_ context.Context, funcName string) (err error) {
	_, err = m.vm.Execute(funcName)
	return
}

func (m *wasmedgeModule) CallI32_V(_ context.Context, funcName string, param uint32) (err error) {
	_, err = m.vm.Execute(funcName, int32(param))
	return
}

func (m *wasmedgeModule) CallI64_I64(_ context.Context, funcName string, param uint64) (uint64, error) {
	if result, err := m.vm.Execute(funcName, int64(param)); err != nil {
		return 0, err
	} else {
		return uint64(result[0].(int64)), nil
	}
}

func (m *wasmedgeModule) WriteMemory(offset uint32, bytes []byte) error {
	mod := m.vm.GetActiveModule()
	mem := mod.FindMemory("memory")
	if unsafeSlice, err := mem.GetData(uint(offset), uint(len(bytes))); err != nil {
		return err
	} else {
		copy(unsafeSlice, bytes)
	}
	return nil
}

func (m *wasmedgeModule) Close(context.Context) error {
	if env := m.env; env != nil {
		env.Release()
	}
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
