package gojs

import (
	"net/http"

	"github.com/tetratelabs/wazero/internal/gojs/custom"
	"github.com/tetratelabs/wazero/internal/gojs/goos"
)

// newJsGlobal = js.Global() // js.go init
func newJsGlobal(rt http.RoundTripper) *jsVal {
	var fetchProperty interface{} = goos.Undefined
	if rt != nil {
		fetchProperty = goos.RefHttpFetch
	}
	return newJsVal(goos.RefValueGlobal, "global").
		addProperties(map[string]interface{}{
			"Object":          objectConstructor,
			"Array":           arrayConstructor,
			"crypto":          jsCrypto,
			"Uint8Array":      uint8ArrayConstructor,
			"fetch":           fetchProperty,
			"AbortController": goos.Undefined,
			"Headers":         headersConstructor,
			"process":         jsProcess,
			"fs":              jsfs,
			"Date":            jsDateConstructor,
		}).
		addFunction("fetch", &httpFetch{rt})
}

var (
	// Values below are not built-in, but verifiable by looking at Go's source.
	// When marked "XX.go init", these are eagerly referenced during syscall.init

	// jsGo is not a constant

	// objectConstructor is used by js.ValueOf to make `map[string]any`.
	//	Get("Object") // js.go init
	objectConstructor = newJsVal(goos.RefObjectConstructor, "Object")

	// arrayConstructor is used by js.ValueOf to make `[]any`.
	//	Get("Array") // js.go init
	arrayConstructor = newJsVal(goos.RefArrayConstructor, "Array")

	// jsProcess = js.Global().Get("process") // fs_js.go init
	jsProcess = newJsVal(goos.RefJsProcess, custom.NameProcess).
			addProperties(map[string]interface{}{
			"pid":  float64(1),        // Get("pid").Int() in syscall_js.go for syscall.Getpid
			"ppid": goos.RefValueZero, // Get("ppid").Int() in syscall_js.go for syscall.Getppid
		}).
		addFunction(custom.NameProcessCwd, processCwd{}).              // syscall.Cwd in fs_js.go
		addFunction(custom.NameProcessChdir, processChdir{}).          // syscall.Chdir in fs_js.go
		addFunction(custom.NameProcessGetuid, returnZero{}).           // syscall.Getuid in syscall_js.go
		addFunction(custom.NameProcessGetgid, returnZero{}).           // syscall.Getgid in syscall_js.go
		addFunction(custom.NameProcessGeteuid, returnZero{}).          // syscall.Geteuid in syscall_js.go
		addFunction(custom.NameProcessGetgroups, returnSliceOfZero{}). // syscall.Getgroups in syscall_js.go
		addFunction(custom.NameProcessUmask, processUmask{})           // syscall.Umask in syscall_js.go

	// uint8ArrayConstructor = js.Global().Get("Uint8Array")
	//	// fs_js.go, rand_js.go, roundtrip_js.go init
	//
	// It has only one invocation pattern: `buf := uint8Array.New(len(b))`
	uint8ArrayConstructor = newJsVal(goos.RefUint8ArrayConstructor, "Uint8Array")
)
