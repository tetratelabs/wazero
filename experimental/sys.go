package experimental

// SysKey is a context.Context Value key. Its associated value should be a Sys.
//
// See https://github.com/tetratelabs/wazero/issues/491
type SysKey struct{}

// Sys controls experimental aspects currently only used by WASI.
type Sys interface {
	// TimeNowUnixNano allows you to control the value otherwise returned by time.Now().UnixNano()
	TimeNowUnixNano() uint64
}
