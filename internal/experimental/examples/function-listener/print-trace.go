package print_trace

import (
	"context"
	_ "embed"
	"fmt"
	"log"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/wasmdebug"
)

type stackKey struct{}

type callListener struct{ message string }

func (l *callListener) Before(ctx context.Context, _ []uint64) context.Context {
	currStack, _ := ctx.Value(stackKey{}).([]string)
	return context.WithValue(ctx, stackKey{}, append(currStack, l.message))
}

func (l *callListener) After(context.Context, []uint64) {
}

type callListenerFactory struct{}

func (f *callListenerFactory) NewListener(fnd experimental.FunctionDefinition) experimental.FunctionListener {
	return &callListener{
		message: wasmdebug.Signature(fnd.ModuleName()+"."+funcName(fnd), fnd.ParamTypes(), fnd.ResultTypes()),
	}
}

// funcName returns the name in priority order: first export name, module-defined name, numeric index.
func funcName(fnd experimental.FunctionDefinition) string {
	if len(fnd.ExportNames()) > 0 {
		return fnd.ExportNames()[0]
	}
	if fnd.Name() != "" {
		return fnd.Name()
	}
	return fmt.Sprintf("[%d]", fnd.Index())
}

// main shows how to define, import and call a Go-defined function from a
// WebAssembly-defined function.
//
// See README.md for a full description.
func main() {
	// FunctionListenerFactory is an experimental API, so the only way to configure it is via context key.
	ctx := context.WithValue(context.Background(),
		experimental.FunctionListenerFactoryKey{}, &callListenerFactory{})

	// Create a new WebAssembly Runtime.
	// Note: This is interpreter-only for now!
	r := wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfigInterpreter())

	// Instantiate a Go-defined module named "env" that exports functions to
	// get the current year and log to the console.
	//
	// Note: As noted on ExportFunction documentation, function signatures are
	// constrained to a subset of numeric types.
	// Note: "env" is a module name conventionally used for arbitrary
	// host-defined functions, but any name would do.
	env, err := r.NewModuleBuilder("env").
		ExportFunction("host1", host1).
		ExportFunction("print_trace", printTrace).
		Instantiate(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer env.Close(ctx)

	// Instantiate a WebAssembly module named "listener" that imports
	// functions defined in "env".
	//
	// Note: The import syntax in both Text and Binary format is the same
	// regardless of if the function was defined in Go or WebAssembly.
	listener, err := r.InstantiateModuleFromCode(ctx, []byte(`
;; Define the optional module name. '$' prefixing is a part of the text format.
(module $listener

  ;; In WebAssembly, you don't import an entire module, rather each function.
  ;; This imports the functions and gives them names which are easier to read
  ;; than the alternative (zero-based index).
  ;;
  ;; Note: Importing unused functions is not an error in WebAssembly.
  (import "env" "host1" (func $host1 (param i32)))
  (import "env" "print_trace" (func $print_trace))

  ;; wasm1 calls host1.
  (func $wasm1 (param $val1 i32)
                 ;; stack: []
    local.get 0  ;; stack: [$value]
    call $host1  ;; stack: []
  )
  ;; export allows api.Module to return this via ExportedFunction("wasm1")
  (export "wasm1" (func $wasm1))

  ;; wasm1 calls print_trace.
  (func $wasm2 (param $val2 i32)
                        ;; stack: []
    call $print_trace   ;; stack: []
  )
  ;; export allows api.Module to return this via ExportedFunction("wasm2")
  (export "wasm2" (func $wasm2))
)`))
	if err != nil {
		log.Fatal(err)
	}
	defer listener.Close(ctx)

	// First, try calling the "get_age" function and printing to the console externally.
	_, err = listener.ExportedFunction("wasm1").Call(ctx, 100)
	if err != nil {
		log.Fatal(err)
	}
}

func host1(ctx context.Context, m api.Module, val uint32) {
	hostOnly(ctx, m, val)
}

// Wazero cannot intercept host->host calls as it is precompiled by Go. But since
// ctx is propagated, such calls can still participate in the trace manually if
// they want.
func hostOnly(ctx context.Context, m api.Module, val uint32) {
	ctx = (&callListener{message: "host_only(i32)"}).Before(ctx, nil)
	host2(ctx, m, val)
}

func host2(ctx context.Context, m api.Module, val uint32) {
	_, err := m.ExportedFunction("wasm2").Call(ctx, uint64(val))
	if err != nil {
		log.Fatalf("Could not invoke wasm2: %v", err)
	}
}

func printTrace(ctx context.Context) {
	stack := ctx.Value(stackKey{}).([]string)
	for _, f := range stack {
		fmt.Println(f)
	}
}
