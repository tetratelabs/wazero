package logging

import (
	"context"
	"encoding/binary"
	"strconv"
	"strings"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/logging"
	"github.com/tetratelabs/wazero/internal/wasi_snapshot_preview1"
)

var le = binary.LittleEndian

func IsFilesystemFunction(fnd api.FunctionDefinition) bool {
	switch {
	case strings.HasPrefix(fnd.Name(), "path_"):
		return true
	case strings.HasPrefix(fnd.Name(), "fd_"):
		return true
	}
	return false
}

func Config(fnd api.FunctionDefinition) (pLoggers []logging.ParamLogger, rLoggers []logging.ResultLogger) {
	switch fnd.Name() {
	case "fd_prestat_get":
		pLoggers = []logging.ParamLogger{logging.NewParamLogger(0, "fd", logging.ValueTypeI32)}
		rLoggers = []logging.ResultLogger{resultParamLogger("prestat", logPrestat(1).Log), logErrno}
		return
	case "proc_exit":
		return logging.ValueLoggers(fnd)
	}

	for idx := uint32(0); idx < uint32(len(fnd.ParamTypes())); idx++ {
		name := fnd.ParamNames()[idx]
		var logger logging.ParamLogger

		if isLookupFlags(fnd, name) {
			logger = (&logLookupflags{name, idx}).Log
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

		switch name {
		case "fdflags":
			logger = logFdflags(idx).Log
		case "oflags":
			logger = logOflags(idx).Log
		case "fs_rights_base":
			logger = logFsRightsBase(idx).Log
		case "fs_rights_inheriting":
			logger = logFsRightsInheriting(idx).Log
		case "result.nread", "result.nwritten", "result.opened_fd":
			name = resultParamName(name)
			logger = logMemI32(idx).Log
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
			logger = logging.NewParamLogger(idx, name, fnd.ParamTypes()[idx])
		}
		pLoggers = append(pLoggers, logger)
	}
	// All WASI functions except proc_after return only an logErrno result.
	rLoggers = append(rLoggers, logErrno)
	return
}

func isLookupFlags(fnd api.FunctionDefinition, name string) bool {
	switch fnd.Name() {
	case "path_filestat_get", "path_filestat_set_times":
		return name == "flags"
	case "path_link":
		return name == "old_flags"
	case "path_open":
		return name == "dirflags"
	}
	return false
}

func logErrno(_ context.Context, _ api.Module, w logging.Writer, _, results []uint64) {
	errno := wasi_snapshot_preview1.ErrnoName(uint32(results[0]))
	w.WriteString("errno=") //nolint
	w.WriteString(errno)    //nolint
}

type logMemI32 uint32

func (i logMemI32) Log(_ context.Context, mod api.Module, w logging.Writer, params []uint64) {
	if v, ok := mod.Memory().ReadUint32Le(uint32(params[i])); ok {
		writeI32(w, v)
	}
}

type logFilestat uint32

func (i logFilestat) Log(_ context.Context, mod api.Module, w logging.Writer, params []uint64) {
	offset, byteCount := uint32(params[i]), uint32(64)
	if buf, ok := mod.Memory().Read(offset, byteCount); ok {
		w.WriteString("{filetype=")                                 //nolint
		w.WriteString(wasi_snapshot_preview1.FiletypeName(buf[16])) //nolint
		w.WriteString(",size=")                                     //nolint
		writeI64(w, le.Uint64(buf[32:]))
		w.WriteString(",mtim=") //nolint
		writeI64(w, le.Uint64(buf[40:]))
		w.WriteString("}") //nolint
	}
}

type logFdstat uint32

func (i logFdstat) Log(_ context.Context, mod api.Module, w logging.Writer, params []uint64) {
	offset, byteCount := uint32(params[i]), uint32(24)
	if buf, ok := mod.Memory().Read(offset, byteCount); ok {
		w.WriteString("{filetype=")                                //nolint
		w.WriteString(wasi_snapshot_preview1.FiletypeName(buf[0])) //nolint
		w.WriteString(",fdflags=")                                 //nolint
		writeFlags(w, fdflagNames[:], int(le.Uint16(buf[2:])))
		w.WriteString(",fs_rights_base=") //nolint
		writeFlags(w, rightNames[:], int(le.Uint16(buf[8:])))
		w.WriteString(",fs_rights_inheriting=") //nolint
		writeFlags(w, rightNames[:], int(le.Uint16(buf[16:])))
		w.WriteString("}") //nolint
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
		if results[0] == 0 {  // ESUCCESS
			pLogger(ctx, mod, w, params)
		}
	}
}

type logFdflags int

func (i logFdflags) Log(_ context.Context, _ api.Module, w logging.Writer, params []uint64) {
	w.WriteString("fdflags=") //nolint
	writeFlags(w, fdflagNames[:], int(params[i]))
}

type logLookupflags struct {
	name string
	i    uint32
}

func (l *logLookupflags) Log(_ context.Context, _ api.Module, w logging.Writer, params []uint64) {
	w.WriteString(l.name) //nolint
	w.WriteByte('=')      //nolint
	writeFlags(w, lookupflagNames[:], int(params[l.i]))
}

type logFsRightsBase uint32

func (i logFsRightsBase) Log(_ context.Context, _ api.Module, w logging.Writer, params []uint64) {
	w.WriteString("fs_rights_base=") //nolint
	writeFlags(w, rightNames[:], int(params[i]))
}

type logFsRightsInheriting uint32

func (i logFsRightsInheriting) Log(_ context.Context, _ api.Module, w logging.Writer, params []uint64) {
	w.WriteString("fs_rights_inheriting=") //nolint
	writeFlags(w, rightNames[:], int(params[i]))
}

type logOflags int

func (i logOflags) Log(_ context.Context, _ api.Module, w logging.Writer, params []uint64) {
	w.WriteString("oflags=") //nolint
	writeFlags(w, oflagNames[:], int(params[i]))
}

func writeFlags(w logging.Writer, names []string, f int) {
	first := true
	for i, sf := range names {
		target := 1 << i
		if target&f != 0 {
			if !first {
				w.WriteByte('|') //nolint
			} else {
				first = false
			}
			w.WriteString(sf) //nolint
		}
	}
}

var oflagNames = [...]string{
	"CREAT",
	"DIRECTORY",
	"EXCL",
	"TRUNC",
}

var fdflagNames = [...]string{
	"APPEND",
	"DSYNC",
	"NONBLOCK",
	"RSYNC",
	"SYNC",
}

var lookupflagNames = [...]string{
	"SYMLINK_FOLLOW",
}

var rightNames = [...]string{
	"FD_DATASYNC",
	"FD_READ",
	"FD_SEEK",
	"FDSTAT_SET_FLAGS",
	"FD_SYNC",
	"FD_TELL",
	"FD_WRITE",
	"FD_ADVISE",
	"FD_ALLOCATE",
	"PATH_CREATE_DIRECTORY",
	"PATH_CREATE_FILE",
	"PATH_LINK_SOURCE",
	"PATH_LINK_TARGET",
	"PATH_OPEN",
	"FD_READDIR",
	"PATH_READLINK",
	"PATH_RENAME_SOURCE",
	"PATH_RENAME_TARGET",
	"PATH_FILESTAT_GET",
	"PATH_FILESTAT_SET_SIZE",
	"PATH_FILESTAT_SET_TIMES",
	"FD_FILESTAT_GET",
	"FD_FILESTAT_SET_SIZE",
	"FD_FILESTAT_SET_TIMES",
	"PATH_SYMLINK",
	"PATH_REMOVE_DIRECTORY",
	"PATH_UNLINK_FILE",
	"POLL_FD_READWRITE",
	"SOCK_SHUTDOWN",
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
