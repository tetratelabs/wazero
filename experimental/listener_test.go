package experimental_test

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/wasi"
)

// loggerFactory implements experimental.FunctionListenerFactory to log all function calls to the console.
type loggerFactory struct{}

// Size implements the same method as documented on api.Memory.
func (f *loggerFactory) NewListener(fnd experimental.FunctionDefinition) experimental.FunctionListener {
	return &logger{funcName: []byte(fnd.ModuleName() + "." + funcName(fnd))}
}

// nestLevelKey holds state between logger.Before and logger.After to ensure call depth is reflected.
type nestLevelKey struct{}

// logger implements experimental.FunctionListener to log entrance and exit of each function call.
type logger struct{ funcName []byte }

// Before logs to stdout the module and function name, prefixed with '>>' and indented based on the call nesting level.
func (l *logger) Before(ctx context.Context, _ []uint64) context.Context {
	nestLevel, _ := ctx.Value(nestLevelKey{}).(int)

	l.writeIndented(os.Stdout, true, nestLevel+1)

	// Increase the next nesting level.
	return context.WithValue(ctx, nestLevelKey{}, nestLevel+1)
}

// After logs to stdout the module and function name, prefixed with '<<' and indented based on the call nesting level.
func (l *logger) After(ctx context.Context, _ error, _ []uint64) {
	// Note: We use the nest level directly even though it is the "next" nesting level.
	// This works because our indent of zero nesting is one tab.
	l.writeIndented(os.Stdout, true, ctx.Value(nestLevelKey{}).(int))
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

// This is a very basic integration of listener. The main goal is to show how it is configured.
func Example_listener() {
	// Set context to one that has an experimental listener
	ctx := context.WithValue(context.Background(), experimental.FunctionListenerFactoryKey{}, &loggerFactory{})

	r := wazero.NewRuntimeWithConfig(wazero.NewRuntimeConfigInterpreter())
	wm, err := wasi.InstantiateSnapshotPreview1(ctx, r)
	if err != nil {
		log.Fatal(err)
	}
	defer wm.Close(ctx)

	cfg := wazero.NewModuleConfig().WithStdout(os.Stdout)
	mod, err := r.InstantiateModuleFromCodeWithConfig(ctx, []byte(`(module $listener
  (import "wasi_snapshot_preview1" "random_get"
    (func $wasi.random_get (param $buf i32) (param $buf_len i32) (result (;errno;) i32)))
  (func i32.const 0 i32.const 4 call 0 drop) ;; write 4 bytes of random data
  (memory 1 1)
  (start 1) ;; call the second function
)`), cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer mod.Close(ctx)

	// Output:
	// >>	listener.[1]
	// >>		wasi_snapshot_preview1.random_get
	// >>		wasi_snapshot_preview1.random_get
	// >>	listener.[1]
}

// writeIndented writes an indented message like this: ">>\t\t\t$indentLevel$funcName\n"
func (l *logger) writeIndented(writer io.Writer, before bool, indentLevel int) {
	var message = make([]byte, 0, 2+indentLevel+len(l.funcName)+1)
	if before {
		message = append(message, '>', '>')
	} else { // after
		message = append(message, '<', '<')
	}

	for i := 0; i < indentLevel; i++ {
		message = append(message, '\t')
	}
	message = append(message, l.funcName...)
	message = append(message, '\n')
	_, _ = writer.Write(message)
}
