package gojs

import (
	"net/http"
)

const (
	// predefined

	idValueNaN uint32 = iota
	idValueZero
	idValueNull
	idValueTrue
	idValueFalse
	idValueGlobal
	idJsGo

	// The below are derived from analyzing `*_js.go` source.
	idObjectConstructor
	idArrayConstructor
	idJsProcess
	idJsfs
	idJsfsConstants
	idUint8ArrayConstructor
	idJsCrypto
	idJsDateConstructor
	idJsDate
	idHttpFetch
	idHttpHeaders
	nextID
)

const (
	refValueUndefined         = ref(0)
	refValueNaN               = (nanHead|ref(typeFlagNone))<<32 | ref(idValueNaN)
	refValueZero              = (nanHead|ref(typeFlagNone))<<32 | ref(idValueZero)
	refValueNull              = (nanHead|ref(typeFlagNone))<<32 | ref(idValueNull)
	refValueTrue              = (nanHead|ref(typeFlagNone))<<32 | ref(idValueTrue)
	refValueFalse             = (nanHead|ref(typeFlagNone))<<32 | ref(idValueFalse)
	refValueGlobal            = (nanHead|ref(typeFlagObject))<<32 | ref(idValueGlobal)
	refJsGo                   = (nanHead|ref(typeFlagObject))<<32 | ref(idJsGo)
	refObjectConstructor      = (nanHead|ref(typeFlagFunction))<<32 | ref(idObjectConstructor)
	refArrayConstructor       = (nanHead|ref(typeFlagFunction))<<32 | ref(idArrayConstructor)
	refJsProcess              = (nanHead|ref(typeFlagObject))<<32 | ref(idJsProcess)
	refJsfs                   = (nanHead|ref(typeFlagObject))<<32 | ref(idJsfs)
	refJsfsConstants          = (nanHead|ref(typeFlagObject))<<32 | ref(idJsfsConstants)
	refUint8ArrayConstructor  = (nanHead|ref(typeFlagFunction))<<32 | ref(idUint8ArrayConstructor)
	refJsCrypto               = (nanHead|ref(typeFlagFunction))<<32 | ref(idJsCrypto)
	refJsDateConstructor      = (nanHead|ref(typeFlagFunction))<<32 | ref(idJsDateConstructor)
	refJsDate                 = (nanHead|ref(typeFlagObject))<<32 | ref(idJsDate)
	refHttpFetch              = (nanHead|ref(typeFlagFunction))<<32 | ref(idHttpFetch)
	refHttpHeadersConstructor = (nanHead|ref(typeFlagFunction))<<32 | ref(idHttpHeaders)
)

// newJsGlobal = js.Global() // js.go init
func newJsGlobal(rt http.RoundTripper) *jsVal {
	var fetchProperty interface{} = undefined
	if rt != nil {
		fetchProperty = refHttpFetch
	}
	return newJsVal(refValueGlobal, "global").
		addProperties(map[string]interface{}{
			"Object":          objectConstructor,
			"Array":           arrayConstructor,
			"crypto":          jsCrypto,
			"Uint8Array":      uint8ArrayConstructor,
			"fetch":           fetchProperty,
			"AbortController": undefined,
			"Headers":         headersConstructor,
			"process":         jsProcess,
			"fs":              jsfs,
			"Date":            jsDateConstructor,
		}).
		addFunction("fetch", &fetch{})
}

var (
	// Values below are not built-in, but verifiable by looking at Go's source.
	// When marked "XX.go init", these are eagerly referenced during syscall.init

	// jsGo is not a constant

	// objectConstructor is used by js.ValueOf to make `map[string]any`.
	//	Get("Object") // js.go init
	objectConstructor = newJsVal(refObjectConstructor, "Object")

	// arrayConstructor is used by js.ValueOf to make `[]any`.
	//	Get("Array") // js.go init
	arrayConstructor = newJsVal(refArrayConstructor, "Array")

	// jsProcess = js.Global().Get("process") // fs_js.go init
	jsProcess = newJsVal(refJsProcess, "process").
			addProperties(map[string]interface{}{
			"pid":  float64(1),   // Get("pid").Int() in syscall_js.go for syscall.Getpid
			"ppid": refValueZero, // Get("ppid").Int() in syscall_js.go for syscall.Getppid
		}).
		addFunction("cwd", &cwd{}).                     // syscall.Cwd in fs_js.go
		addFunction("chdir", &chdir{}).                 // syscall.Chdir in fs_js.go
		addFunction("getuid", &returnZero{}).           // syscall.Getuid in syscall_js.go
		addFunction("getgid", &returnZero{}).           // syscall.Getgid in syscall_js.go
		addFunction("geteuid", &returnZero{}).          // syscall.Geteuid in syscall_js.go
		addFunction("getgroups", &returnSliceOfZero{}). // syscall.Getgroups in syscall_js.go
		addFunction("umask", &returnArg0{})             // syscall.Umask in syscall_js.go

	// uint8ArrayConstructor = js.Global().Get("Uint8Array")
	//	// fs_js.go, rand_js.go, roundtrip_js.go init
	//
	// It has only one invocation pattern: `buf := uint8Array.New(len(b))`
	uint8ArrayConstructor = newJsVal(refUint8ArrayConstructor, "Uint8Array")
)
