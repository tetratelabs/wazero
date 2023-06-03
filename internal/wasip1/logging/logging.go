package logging

import (
	"context"
	"encoding/binary"
	"strconv"
	"strings"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/logging"
	"github.com/tetratelabs/wazero/internal/sys"
	. "github.com/tetratelabs/wazero/internal/wasip1"
)

var le = binary.LittleEndian

func isClockFunction(fnd api.FunctionDefinition) bool {
	return strings.HasPrefix(fnd.Name(), "clock_")
}

func isProcFunction(fnd api.FunctionDefinition) bool {
	return fnd.Name() == ProcExitName
}

func isFilesystemFunction(fnd api.FunctionDefinition) bool {
	switch {
	case strings.HasPrefix(fnd.Name(), "path_"):
		return true
	case strings.HasPrefix(fnd.Name(), "fd_"):
		return true
	}
	return false
}

func isPollFunction(fnd api.FunctionDefinition) bool {
	return fnd.Name() == PollOneoffName
}

func isRandomFunction(fnd api.FunctionDefinition) bool {
	return fnd.Name() == RandomGetName
}

func isSockFunction(fnd api.FunctionDefinition) bool {
	return strings.HasPrefix(fnd.Name(), "sock_")
}

// IsInLogScope returns true if the current function is in any of the scopes.
func IsInLogScope(fnd api.FunctionDefinition, scopes logging.LogScopes) bool {
	if scopes.IsEnabled(logging.LogScopeClock) {
		if isClockFunction(fnd) {
			return true
		}
	}

	if scopes.IsEnabled(logging.LogScopeProc) {
		if isProcFunction(fnd) {
			return true
		}
	}

	if scopes.IsEnabled(logging.LogScopeFilesystem) {
		if isFilesystemFunction(fnd) {
			return true
		}
	}

	if scopes.IsEnabled(logging.LogScopePoll) {
		if isPollFunction(fnd) {
			return true
		}
	}

	if scopes.IsEnabled(logging.LogScopeRandom) {
		if isRandomFunction(fnd) {
			return true
		}
	}

	if scopes.IsEnabled(logging.LogScopeSock) {
		if isSockFunction(fnd) {
			return true
		}
	}

	return scopes == logging.LogScopeAll
}

func Config(fnd api.FunctionDefinition) (pSampler logging.ParamSampler, pLoggers []logging.ParamLogger, rLoggers []logging.ResultLogger) {
	types := fnd.ParamTypes()
	names := fnd.ParamNames()

	switch fnd.Name() {
	case FdPrestatGetName:
		pLoggers = []logging.ParamLogger{logging.NewParamLogger(0, "fd", logging.ValueTypeI32)}
		rLoggers = []logging.ResultLogger{resultParamLogger("prestat", logPrestat(1).Log), logErrno}
		return
	case ProcExitName:
		pLoggers, rLoggers = logging.Config(fnd)
		return
	case FdReadName, FdWriteName:
		pSampler = fdReadWriteSampler
	}

	for idx := uint32(0); idx < uint32(len(types)); idx++ {
		name := names[idx]
		var logger logging.ParamLogger

		if isLookupFlags(fnd, name) {
			lf := &logLookupflags{name, idx}
			logger = lf.Log
			pLoggers = append(pLoggers, logger)
			continue
		}

		isResult := strings.HasPrefix(name, "result.")

		if strings.Contains(name, "path") {
			if isResult {
				name = resultParamName(name)
				logger = logString(idx).Log
				rLoggers = append(rLoggers, resultParamLogger(name, logger))
			} else {
				logger = logging.NewParamLogger(idx, name, logging.ValueTypeString)
				pLoggers = append(pLoggers, logger)
			}
			idx++
			continue
		}

		if strings.HasPrefix(fnd.Name(), "clock_") {
			switch name {
			case "id":
				logger = logClockId(idx).Log
			case "result.resolution":
				name = resultParamName(name)
				logger = logMemI32(idx).Log
				rLoggers = append(rLoggers, resultParamLogger(name, logger))
				continue
			case "result.timestamp":
				name = resultParamName(name)
				logger = logMemI64(idx).Log
				rLoggers = append(rLoggers, resultParamLogger(name, logger))
				continue
			default:
				logger = logging.NewParamLogger(idx, name, types[idx])
			}
			pLoggers = append(pLoggers, logger)
			continue
		}

		if strings.HasPrefix(fnd.Name(), "sock_") {
			switch name {
			case "flags":
				logger = logFlags(idx).Log
			case "ri_flags":
				logger = logRiFlags(idx).Log
			case "si_flags":
				logger = logSiFlags(idx).Log
			case "how":
				logger = logSdFlags(idx).Log
			case "result.fd", "result.ro_datalen", "result.so_datalen":
				name = resultParamName(name)
				logger = logMemI32(idx).Log
				rLoggers = append(rLoggers, resultParamLogger(name, logger))
				continue
			case "result.ro_flags":
				logger = logRoFlags(idx).Log
				rLoggers = append(rLoggers, resultParamLogger("ro_flags", logger))
				continue
			default:
				logger = logging.NewParamLogger(idx, name, types[idx])
			}
			pLoggers = append(pLoggers, logger)
			continue
		}

		switch name {
		case "fdflags":
			logger = logFdflags(idx).Log
		case "flags":
			logger = logFlags(idx).Log
		case "fst_flags":
			logger = logFstflags(idx).Log
		case "oflags":
			logger = logOflags(idx).Log
		case "fs_rights_base":
			logger = logFsRightsBase(idx).Log
		case "fs_rights_inheriting":
			logger = logFsRightsInheriting(idx).Log
		case "result.nread", "result.nwritten", "result.opened_fd", "result.nevents", "result.bufused":
			name = resultParamName(name)
			logger = logMemI32(idx).Log
			rLoggers = append(rLoggers, resultParamLogger(name, logger))
			continue
		case "result.newoffset":
			name = resultParamName(name)
			logger = logMemI64(idx).Log
			rLoggers = append(rLoggers, resultParamLogger(name, logger))
			continue
		case "result.filestat":
			name = resultParamName(name)
			logger = logFilestat(idx).Log
			rLoggers = append(rLoggers, resultParamLogger(name, logger))
			continue
		case "result.stat":
			name = resultParamName(name)
			logger = logFdstat(idx).Log
			rLoggers = append(rLoggers, resultParamLogger(name, logger))
			continue
		default:
			logger = logging.NewParamLogger(idx, name, types[idx])
		}
		pLoggers = append(pLoggers, logger)
	}
	// All WASI functions except proc_after return only an logErrno result.
	rLoggers = append(rLoggers, logErrno)
	return
}

// Ensure we don't clutter log with reads and writes to stdio.
func fdReadWriteSampler(_ context.Context, _ api.Module, params []uint64) bool {
	fd := int32(params[0])
	return fd > sys.FdStderr
}

func isLookupFlags(fnd api.FunctionDefinition, name string) bool {
	switch fnd.Name() {
	case PathFilestatGetName, PathFilestatSetTimesName:
		return name == "flags"
	case PathLinkName:
		return name == "old_flags"
	case PathOpenName:
		return name == "dirflags"
	}
	return false
}

func logErrno(_ context.Context, _ api.Module, w logging.Writer, _, results []uint64) {
	errno := ErrnoName(uint32(results[0]))
	w.WriteString("errno=") //nolint
	w.WriteString(errno)    //nolint
}

type logMemI32 uint32

func (i logMemI32) Log(_ context.Context, mod api.Module, w logging.Writer, params []uint64) {
	if v, ok := mod.Memory().ReadUint32Le(uint32(params[i])); ok {
		writeI32(w, v)
	}
}

type logMemI64 uint32

func (i logMemI64) Log(_ context.Context, mod api.Module, w logging.Writer, params []uint64) {
	if v, ok := mod.Memory().ReadUint64Le(uint32(params[i])); ok {
		writeI64(w, v)
	}
}

type logFilestat uint32

func (i logFilestat) Log(_ context.Context, mod api.Module, w logging.Writer, params []uint64) {
	offset, byteCount := uint32(params[i]), uint32(64)
	if buf, ok := mod.Memory().Read(offset, byteCount); ok {
		w.WriteString("{filetype=")          //nolint
		w.WriteString(FiletypeName(buf[16])) //nolint
		w.WriteString(",size=")              //nolint
		writeI64(w, le.Uint64(buf[32:]))
		w.WriteString(",mtim=") //nolint
		writeI64(w, le.Uint64(buf[48:]))
		w.WriteString("}") //nolint
	}
}

type logFdstat uint32

func (i logFdstat) Log(_ context.Context, mod api.Module, w logging.Writer, params []uint64) {
	offset, byteCount := uint32(params[i]), uint32(24)
	if buf, ok := mod.Memory().Read(offset, byteCount); ok {
		w.WriteString("{filetype=")                           //nolint
		w.WriteString(FiletypeName(buf[0]))                   //nolint
		w.WriteString(",fdflags=")                            //nolint
		w.WriteString(FdFlagsString(int(le.Uint16(buf[2:])))) //nolint
		w.WriteString(",fs_rights_base=")                     //nolint
		w.WriteString(RightsString(int(le.Uint16(buf[8:]))))  //nolint
		w.WriteString(",fs_rights_inheriting=")               //nolint
		w.WriteString(RightsString(int(le.Uint16(buf[16:])))) //nolint
		w.WriteString("}")                                    //nolint
	}
}

type logString uint32

func (i logString) Log(_ context.Context, mod api.Module, w logging.Writer, params []uint64) {
	offset, byteCount := uint32(params[i]), uint32(params[i+1])
	if s, ok := mod.Memory().Read(offset, byteCount); ok {
		w.Write(s) //nolint
	}
}

type logPrestat uint32

// Log writes the only valid field: pr_name_len
// See https://github.com/WebAssembly/WASI/blob/snapshot-01/phases/snapshot/docs.md#prestat_dir
func (i logPrestat) Log(_ context.Context, mod api.Module, w logging.Writer, params []uint64) {
	offset := uint32(params[i]) + 4 // skip to pre_name_len field
	if nameLen, ok := mod.Memory().ReadUint32Le(offset); ok {
		w.WriteString("{pr_name_len=") //nolint
		writeI32(w, nameLen)
		w.WriteString("}") //nolint
	}
}

// resultParamLogger logs the value of the parameter on ESUCCESS.
func resultParamLogger(name string, pLogger logging.ParamLogger) logging.ResultLogger {
	prefix := name + "="
	return func(ctx context.Context, mod api.Module, w logging.Writer, params, results []uint64) {
		w.WriteString(prefix) //nolint
		if Errno(results[0]) == ErrnoSuccess {
			pLogger(ctx, mod, w, params)
		}
	}
}

type logClockId int

func (i logClockId) Log(_ context.Context, _ api.Module, w logging.Writer, params []uint64) {
	id := uint32(params[i])
	w.WriteString("id=") //nolint
	switch id {
	case ClockIDRealtime:
		w.WriteString("realtime") //nolint
	case ClockIDMonotonic:
		w.WriteString("monotonic") //nolint
	default:
		writeI32(w, id)
	}
}

type logFdflags int

func (i logFdflags) Log(_ context.Context, _ api.Module, w logging.Writer, params []uint64) {
	w.WriteString("fdflags=")                    //nolint
	w.WriteString(FdFlagsString(int(params[i]))) //nolint
}

type logLookupflags struct {
	name string
	i    uint32
}

func (l *logLookupflags) Log(_ context.Context, _ api.Module, w logging.Writer, params []uint64) {
	w.WriteString(l.name)                              //nolint
	w.WriteByte('=')                                   //nolint
	w.WriteString(LookupflagsString(int(params[l.i]))) //nolint
}

type logFsRightsBase uint32

func (i logFsRightsBase) Log(_ context.Context, _ api.Module, w logging.Writer, params []uint64) {
	w.WriteString("fs_rights_base=")            //nolint
	w.WriteString(RightsString(int(params[i]))) //nolint
}

type logFsRightsInheriting uint32

func (i logFsRightsInheriting) Log(_ context.Context, _ api.Module, w logging.Writer, params []uint64) {
	w.WriteString("fs_rights_inheriting=")      //nolint
	w.WriteString(RightsString(int(params[i]))) //nolint
}

type logOflags int

func (i logOflags) Log(_ context.Context, _ api.Module, w logging.Writer, params []uint64) {
	w.WriteString("oflags=")                    //nolint
	w.WriteString(OflagsString(int(params[i]))) //nolint
}

type logFstflags int

func (i logFstflags) Log(_ context.Context, _ api.Module, w logging.Writer, params []uint64) {
	w.WriteString("fst_flags=")                   //nolint
	w.WriteString(FstflagsString(int(params[i]))) //nolint
}

type logFlags int

func (i logFlags) Log(_ context.Context, _ api.Module, w logging.Writer, params []uint64) {
	w.WriteString("flags=")                      //nolint
	w.WriteString(FdFlagsString(int(params[i]))) //nolint
}

type logSdFlags int

func (i logSdFlags) Log(_ context.Context, _ api.Module, w logging.Writer, params []uint64) {
	w.WriteString("how=")                        //nolint
	w.WriteString(SdFlagsString(int(params[i]))) //nolint
}

type logSiFlags int

func (i logSiFlags) Log(_ context.Context, _ api.Module, w logging.Writer, params []uint64) {
	w.WriteString("si_flags=")                   //nolint
	w.WriteString(SiFlagsString(int(params[i]))) //nolint
}

type logRiFlags int

func (i logRiFlags) Log(_ context.Context, _ api.Module, w logging.Writer, params []uint64) {
	w.WriteString("ri_flags=")                   //nolint
	w.WriteString(RiFlagsString(int(params[i]))) //nolint
}

type logRoFlags int

func (i logRoFlags) Log(_ context.Context, _ api.Module, w logging.Writer, params []uint64) {
	w.WriteString(RoFlagsString(int(params[i]))) //nolint
}

func resultParamName(name string) string {
	return name[7:] // without "result."
}

func writeI32(w logging.Writer, v uint32) {
	w.WriteString(strconv.FormatInt(int64(int32(v)), 10)) //nolint
}

func writeI64(w logging.Writer, v uint64) {
	w.WriteString(strconv.FormatInt(int64(v), 10)) //nolint
}
