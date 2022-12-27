package logging

import (
	"context"
	"fmt"
	"io/fs"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/gojs"
	"github.com/tetratelabs/wazero/internal/gojs/custom"
	"github.com/tetratelabs/wazero/internal/gojs/goarch"
	"github.com/tetratelabs/wazero/internal/gojs/goos"
	"github.com/tetratelabs/wazero/internal/logging"
)

func ValueLoggers(fnd api.FunctionDefinition) (pLoggers []logging.ParamLogger, rLoggers []logging.ResultLogger) {
	if fnd.Name() != custom.NameSyscallValueCall {
		return
	}
	pLoggers = []logging.ParamLogger{LogSyscallValueCallParams}
	rLoggers = []logging.ResultLogger{LogSyscallValueCallResults}
	return
}

func ParamSampler(ctx context.Context, mod api.Module, params []uint64) bool {
	vRef, _, _ := syscallValueCallParams(ctx, mod, params)

	// TODO: add more than just filesystem
	return vRef == goos.RefJsfs
}

func LogSyscallValueCallParams(ctx context.Context, mod api.Module, w logging.Writer, params []uint64) {
	vRef, m, args := syscallValueCallParams(ctx, mod, params)

	// TODO: add more than just filesystem
	if vRef != goos.RefJsfs {
		return
	}

	if m == custom.NameFsOpen {
		w.WriteString(fmt.Sprintf("fs.open(name=%s,flags=%016x,perm=%s", //nolint
			args[0], uint32(args[1].(float64)), fs.FileMode(uint32(args[2].(float64)))))
		return
	}

	argNames := custom.FsNameSection[m].ParamNames

	w.WriteString("fs.") //nolint
	w.WriteString(m)     //nolint
	w.WriteByte('(')     //nolint
	writeVals(w, args, argNames)
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

func LogSyscallValueCallResults(ctx context.Context, mod api.Module, w logging.Writer, params, results []uint64) {
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

			w.WriteByte(',')                          // nolint
			w.WriteString(names[i])                   //nolint
			w.WriteByte('=')                          //nolint
			w.WriteString(fmt.Sprintf("%v", vals[i])) //nolint
		}
	}
	w.WriteByte(')') // nolint
}
