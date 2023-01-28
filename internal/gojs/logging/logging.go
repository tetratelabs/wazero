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
	if logging.LogScopeCrypto.IsInLogScope(scopes) {
		if fnd.Name() == custom.NameRuntimeGetRandomData {
			return true
		}
	}

	if logging.LogScopeFilesystem.IsInLogScope(scopes) {
		if fnd.Name() == custom.NameSyscallValueCall {
			return true
		}
	}

	return scopes == logging.LogScopeNone
}

func Config(fnd api.FunctionDefinition) (pSampler logging.ParamSampler, pLoggers []logging.ParamLogger, rLoggers []logging.ResultLogger) {
	switch fnd.Name() {
	// Don't log NameRuntimeWasmWrite as it is used in panics
	case custom.NameSyscallValueCall:
		pSampler = syscallValueCallParamSampler
		pLoggers = []logging.ParamLogger{syscallValueCallParamLogger}
		rLoggers = []logging.ResultLogger{syscallValueCallResultLogger}
	case custom.NameRuntimeGetRandomData:
		_, rLoggers = logging.Config(fnd)
		pLoggers = []logging.ParamLogger{syscallGetRandomParamLogger}
	default: // TODO: make generic logger for gojs
	}
	return
}

func syscallGetRandomParamLogger(_ context.Context, mod api.Module, w logging.Writer, params []uint64) {
	funcName := custom.NameRuntimeGetRandomData
	paramNames := custom.NameSection[funcName].ParamNames
	paramIdx := 1 /* there are two params, only write the length */

	stack := goos.NewStack(funcName, mod.Memory(), uint32(params[0]))
	w.WriteString(paramNames[paramIdx]) //nolint
	w.WriteByte('=')                    //nolint
	writeI32(w, stack.ParamUint32(paramIdx))
}

func syscallValueCallParamLogger(ctx context.Context, mod api.Module, w logging.Writer, params []uint64) {
	vRef, m, args := syscallValueCallParams(ctx, mod, params)

	// TODO: add more than just filesystem
	if vRef != goos.RefJsfs {
		return
	}

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

	argNames := custom.FsNameSection[m].ParamNames

	w.WriteString("fs.") //nolint
	w.WriteString(m)     //nolint
	w.WriteByte('(')     //nolint
	writeVals(w, args, argNames)
	w.WriteByte(')') //nolint
}

func syscallValueCallParamSampler(ctx context.Context, mod api.Module, params []uint64) bool {
	vRef, m, args := syscallValueCallParams(ctx, mod, params)

	// TODO: add more than just filesystem
	if vRef != goos.RefJsfs {
		return false
	}

	// Don't amplify logs with stdio reads or writes
	switch m {
	case custom.NameFsWrite, custom.NameFsRead:
		fd := goos.ValueToUint32(args[0])
		return fd > sys.FdStderr
	}
	return true
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

	// TODO: add more than just filesystem
	if vRef != goos.RefJsfs {
		return
	}

	args := gojs.GetLastEventArgs(ctx)
	argNames := custom.FsNameSection[m].ResultNames

	w.WriteByte('(') //nolint
	writeVals(w, args, argNames)
	w.WriteByte(')') //nolint
}

func writeVals(w logging.Writer, vals []interface{}, names []string) {
	valLen := len(vals)
	if valLen > 0 {
		w.WriteString(names[0]) //nolint
		w.WriteByte('=')        //nolint
		// TODO: learn the types of the vals.
		w.WriteString(fmt.Sprintf("%v", vals[0])) //nolint
		for i := 1; i < valLen; i++ {
			switch names[i] {
			case custom.NameCallback:
				return // last val
			case "buf": // always equal size with byteCount
				continue
			}

			w.WriteByte(',')                          //nolint
			w.WriteString(names[i])                   //nolint
			w.WriteByte('=')                          //nolint
			w.WriteString(fmt.Sprintf("%v", vals[i])) //nolint
		}
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
