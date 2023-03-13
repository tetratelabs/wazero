package logging

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"strconv"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/gojs"
	"github.com/tetratelabs/wazero/internal/gojs/custom"
	"github.com/tetratelabs/wazero/internal/gojs/goarch"
	"github.com/tetratelabs/wazero/internal/gojs/goos"
	"github.com/tetratelabs/wazero/internal/logging"
	"github.com/tetratelabs/wazero/internal/sys"
)

// IsInLogScope returns true if the current function is in any of the scopes.
func IsInLogScope(fnd api.FunctionDefinition, scopes logging.LogScopes) bool {
	if scopes.IsEnabled(logging.LogScopeClock) {
		switch fnd.Name() {
		case custom.NameRuntimeNanotime1, custom.NameRuntimeWalltime:
			return true
		case custom.NameSyscallValueCall: // e.g. Date.getTimezoneOffset
			return true
		}
	}

	if scopes.IsEnabled(logging.LogScopeProc) {
		switch fnd.Name() {
		case custom.NameRuntimeWasmExit:
			return true
		case custom.NameSyscallValueCall: // e.g. proc.*
			return true
		}
	}

	if scopes.IsEnabled(logging.LogScopeFilesystem) {
		if fnd.Name() == custom.NameSyscallValueCall {
			return true // e.g. fs.open
		}
	}

	if scopes.IsEnabled(logging.LogScopeMemory) {
		switch fnd.Name() {
		case custom.NameRuntimeResetMemoryDataView:
			return true
		}
	}

	if scopes.IsEnabled(logging.LogScopePoll) {
		switch fnd.Name() {
		case custom.NameRuntimeScheduleTimeoutEvent, custom.NameRuntimeClearTimeoutEvent:
			return true
		}
	}

	if scopes.IsEnabled(logging.LogScopeRandom) {
		switch fnd.Name() {
		case custom.NameRuntimeGetRandomData:
			return true
		case custom.NameSyscallValueCall: // e.g. crypto.getRandomValues
			return true
		}
	}

	return scopes == logging.LogScopeAll
}

func Config(fnd api.FunctionDefinition, scopes logging.LogScopes) (pSampler logging.ParamSampler, pLoggers []logging.ParamLogger, rLoggers []logging.ResultLogger) {
	switch fnd.Name() {
	case custom.NameRuntimeWasmExit:
		pLoggers = []logging.ParamLogger{runtimeWasmExitParamLogger}
		// no results
	case custom.NameRuntimeWasmWrite:
		return // Don't log NameRuntimeWasmWrite as it is used in panics
	case custom.NameRuntimeResetMemoryDataView:
		// no params or results
	case custom.NameRuntimeNanotime1:
		// no params
		rLoggers = []logging.ResultLogger{runtimeNanotime1ResultLogger}
	case custom.NameRuntimeWalltime:
		// no params
		rLoggers = []logging.ResultLogger{runtimeWalltimeResultLogger}
	case custom.NameRuntimeScheduleTimeoutEvent:
		pLoggers = []logging.ParamLogger{runtimeScheduleTimeoutEventParamLogger}
		rLoggers = []logging.ResultLogger{runtimeScheduleTimeoutEventResultLogger}
	case custom.NameRuntimeClearTimeoutEvent:
		pLoggers = []logging.ParamLogger{runtimeClearTimeoutEventParamLogger}
		// no results
	case custom.NameRuntimeGetRandomData:
		pLoggers = []logging.ParamLogger{runtimeGetRandomDataParamLogger}
		// no results
	case custom.NameSyscallValueCall:
		p := &syscallValueCallParamSampler{scopes: scopes}
		pSampler = p.isSampled
		pLoggers = []logging.ParamLogger{syscallValueCallParamLogger}
		rLoggers = []logging.ResultLogger{syscallValueCallResultLogger}
	default: // TODO: make generic logger for gojs
	}
	return
}

func runtimeGetRandomDataParamLogger(_ context.Context, mod api.Module, w logging.Writer, params []uint64) {
	paramIdx := 1 /* there are two params, only write the length */
	writeParameter(w, custom.NameRuntimeGetRandomData, mod, params, paramIdx)
}

func runtimeScheduleTimeoutEventParamLogger(_ context.Context, mod api.Module, w logging.Writer, params []uint64) {
	writeParameter(w, custom.NameRuntimeScheduleTimeoutEvent, mod, params, 0)
}

func runtimeClearTimeoutEventParamLogger(_ context.Context, mod api.Module, w logging.Writer, params []uint64) {
	writeParameter(w, custom.NameRuntimeClearTimeoutEvent, mod, params, 0)
}

func runtimeWasmExitParamLogger(_ context.Context, mod api.Module, w logging.Writer, params []uint64) {
	writeParameter(w, custom.NameRuntimeWasmExit, mod, params, 0)
}

func writeParameter(w logging.Writer, funcName string, mod api.Module, params []uint64, paramIdx int) {
	paramNames := custom.NameSection[funcName].ParamNames

	stack := goos.NewStack(funcName, mod.Memory(), uint32(params[0]))
	w.WriteString(paramNames[paramIdx]) //nolint
	w.WriteByte('=')                    //nolint
	writeI32(w, stack.ParamUint32(paramIdx))
}

func runtimeNanotime1ResultLogger(_ context.Context, mod api.Module, w logging.Writer, params, _ []uint64) {
	writeResults(w, custom.NameRuntimeNanotime1, mod, params, 0)
}

func runtimeWalltimeResultLogger(_ context.Context, mod api.Module, w logging.Writer, params, _ []uint64) {
	writeResults(w, custom.NameRuntimeWalltime, mod, params, 0)
}

func runtimeScheduleTimeoutEventResultLogger(_ context.Context, mod api.Module, w logging.Writer, params, _ []uint64) {
	writeResults(w, custom.NameRuntimeScheduleTimeoutEvent, mod, params, 1)
}

func writeResults(w logging.Writer, funcName string, mod api.Module, params []uint64, resultOffset int) {
	stack := goos.NewStack(funcName, mod.Memory(), uint32(params[0]))

	resultNames := custom.NameSection[funcName].ResultNames
	results := make([]interface{}, len(resultNames))
	for i := range resultNames {
		results[i] = stack.ParamUint32(i + resultOffset)
	}

	w.WriteByte('(') //nolint
	writeVals(w, resultNames, results)
	w.WriteByte(')') //nolint
}

type syscallValueCallParamSampler struct {
	scopes logging.LogScopes
}

func (s *syscallValueCallParamSampler) isSampled(ctx context.Context, mod api.Module, params []uint64) bool {
	vRef, m, args := syscallValueCallParams(ctx, mod, params)

	switch vRef {
	case goos.RefJsCrypto:
		return logging.LogScopeRandom.IsEnabled(s.scopes)
	case goos.RefJsDate:
		return logging.LogScopeClock.IsEnabled(s.scopes)
	case goos.RefJsfs:
		if !logging.LogScopeFilesystem.IsEnabled(s.scopes) {
			return false
		}
		// Don't amplify logs with stdio reads or writes
		switch m {
		case custom.NameFsWrite, custom.NameFsRead:
			fd := goos.ValueToUint32(args[0])
			return fd > sys.FdStderr
		}
		return true
	case goos.RefJsProcess:
		return logging.LogScopeProc.IsEnabled(s.scopes)
	}

	return s.scopes == logging.LogScopeAll
}

func syscallValueCallParamLogger(ctx context.Context, mod api.Module, w logging.Writer, params []uint64) {
	vRef, m, args := syscallValueCallParams(ctx, mod, params)

	switch vRef {
	case goos.RefJsCrypto:
		logSyscallValueCallArgs(w, custom.NameCrypto, m, args)
	case goos.RefJsDate:
		logSyscallValueCallArgs(w, custom.NameDate, m, args)
	case goos.RefJsfs:
		logFsParams(m, w, args)
	case goos.RefJsProcess:
		logSyscallValueCallArgs(w, custom.NameProcess, m, args)
	default:
		// TODO: other scopes
	}
}

func logFsParams(m string, w logging.Writer, args []interface{}) {
	if m == custom.NameFsOpen {
		w.WriteString("fs.open(")       //nolint
		w.WriteString("path=")          //nolint
		w.WriteString(args[0].(string)) //nolint
		w.WriteString(",flags=")        //nolint
		writeOFlags(w, int(args[1].(float64)))
		w.WriteString(",perm=")                                        //nolint
		w.WriteString(fs.FileMode(uint32(args[2].(float64))).String()) //nolint
		w.WriteByte(')')                                               //nolint
		return
	}

	logSyscallValueCallArgs(w, custom.NameFs, m, args)
}

func logSyscallValueCallArgs(w logging.Writer, n, m string, args []interface{}) {
	argNames := custom.NameSectionSyscallValueCall[n][m].ParamNames
	w.WriteString(n) //nolint
	w.WriteByte('.') //nolint
	w.WriteString(m) //nolint
	w.WriteByte('(') //nolint
	writeVals(w, argNames, args)
	w.WriteByte(')') //nolint
}

func syscallValueCallParams(ctx context.Context, mod api.Module, params []uint64) (goos.Ref, string, []interface{}) {
	mem := mod.Memory()
	funcName := custom.NameSyscallValueCall
	stack := goos.NewStack(funcName, mem, uint32(params[0]))
	vRef := stack.ParamRef(0)               //nolint
	m := stack.ParamString(mem, 1 /*, 2 */) //nolint
	args := stack.ParamVals(ctx, mem, 3 /*, 4 */, gojs.LoadValue)
	return vRef, m, args
}

func syscallValueCallResultLogger(ctx context.Context, mod api.Module, w logging.Writer, params, results []uint64) {
	mem := mod.Memory()
	funcName := custom.NameSyscallValueCall
	stack := goos.NewStack(funcName, mem, goarch.GetSP(mod))
	vRef := stack.ParamRef(0)               //nolint
	m := stack.ParamString(mem, 1 /*, 2 */) //nolint

	var resultNames []string
	var resultVals []interface{}
	switch vRef {
	case goos.RefJsCrypto:
		resultNames = custom.CryptoNameSection[m].ResultNames
		rRef := stack.ParamVal(ctx, 6, gojs.LoadValue) // val is after padding
		resultVals = []interface{}{rRef}
	case goos.RefJsDate:
		resultNames = custom.DateNameSection[m].ResultNames
		rRef := stack.ParamVal(ctx, 6, gojs.LoadValue) // val is after padding
		resultVals = []interface{}{rRef}
	case goos.RefJsfs:
		resultNames = custom.FsNameSection[m].ResultNames
		resultVals = gojs.GetLastEventArgs(ctx)
	case goos.RefJsProcess:
		resultNames = custom.ProcessNameSection[m].ResultNames
		rRef := stack.ParamVal(ctx, 6, gojs.LoadValue) // val is after padding
		resultVals = []interface{}{rRef}
	default:
		// TODO: other scopes
	}

	w.WriteByte('(') //nolint
	writeVals(w, resultNames, resultVals)
	w.WriteByte(')') //nolint
}

func writeVals(w logging.Writer, names []string, vals []interface{}) {
	valLen := len(vals)
	if valLen > 0 {
		writeVal(w, names[0], vals[0])
		for i := 1; i < valLen; i++ {
			name := names[i]
			val := vals[i]

			switch name {
			case custom.NameCallback:
				return // last val
			case "buf": // always equal size with byteCount
				continue
			}

			w.WriteByte(',') //nolint
			writeVal(w, name, val)
		}
	}
}

func writeVal(w logging.Writer, name string, val interface{}) {
	if b, ok := val.(*goos.ByteArray); ok {
		// Write the length instead of a byte array.
		w.WriteString(name)                  //nolint
		w.WriteString("_len=")               //nolint
		writeI32(w, uint32(len(b.Unwrap()))) //nolint
		return
	}
	switch name {
	case "mask", "mode", "oldmask", "perm":
		w.WriteString(name) //nolint
		w.WriteByte('=')    //nolint
		perm := custom.FromJsMode(goos.ValueToUint32(val), 0)
		w.WriteString(perm.String()) //nolint
	default:
		w.WriteString(name)                   //nolint
		w.WriteByte('=')                      //nolint
		w.WriteString(fmt.Sprintf("%v", val)) //nolint
	}
}

func writeOFlags(w logging.Writer, f int) {
	// Iterate a subflagset in order to avoid OS differences, notably for windows
	first := true
	for i, sf := range oFlags {
		if f&sf != 0 {
			if !first {
				w.WriteByte('|') //nolint
			} else {
				first = false
			}
			w.WriteString(oflagToString[i]) //nolint
		}
	}
}

var oFlags = [...]int{
	os.O_RDONLY,
	os.O_WRONLY,
	os.O_RDWR,
	os.O_APPEND,
	os.O_CREATE,
	os.O_EXCL,
	os.O_SYNC,
	os.O_TRUNC,
}

var oflagToString = [...]string{
	"RDONLY",
	"WRONLY",
	"RDWR",
	"APPEND",
	"CREATE",
	"EXCL",
	"SYNC",
	"TRUNC",
}

func writeI32(w logging.Writer, v uint32) {
	w.WriteString(strconv.FormatInt(int64(int32(v)), 10)) //nolint
}
