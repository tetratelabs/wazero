package logging

import (
	"bufio"
	"context"
	"io"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	aslogging "github.com/tetratelabs/wazero/internal/assemblyscript/logging"
	"github.com/tetratelabs/wazero/internal/logging"
	"github.com/tetratelabs/wazero/internal/wasip1"
	wasilogging "github.com/tetratelabs/wazero/internal/wasip1/logging"
)

type Writer interface {
	io.Writer
	io.StringWriter
}

// LogScopes is a bit flag of host function groups to log. e.g. LogScopeRandom.
//
// To specify all scopes, use LogScopeAll. For multiple scopes, OR them
// together like this:
//
//	scope = logging.LogScopeRandom | logging.LogScopeFilesystem
//
// Note: Numeric values are not intended to be interpreted except as bit flags.
type LogScopes = logging.LogScopes

const (
	// LogScopeNone means nothing should be logged
	LogScopeNone = logging.LogScopeNone
	// LogScopeClock enables logging for functions such as `clock_time_get`.
	LogScopeClock = logging.LogScopeClock
	// LogScopeProc enables logging for functions such as `proc_exit`.
	//
	// Note: This includes functions that both log and exit. e.g. `abort`.
	LogScopeProc = logging.LogScopeProc
	// LogScopeFilesystem enables logging for functions such as `path_open`.
	//
	// Note: This doesn't log writes to the console.
	LogScopeFilesystem = logging.LogScopeFilesystem
	// LogScopeMemory enables logging for functions such as
	// `emscripten_notify_memory_growth`.
	LogScopeMemory = logging.LogScopeMemory
	// LogScopePoll enables logging for functions such as `poll_oneoff`.
	LogScopePoll = logging.LogScopePoll
	// LogScopeRandom enables logging for functions such as `random_get`.
	LogScopeRandom = logging.LogScopeRandom
	// LogScopeSock enables logging for functions such as `sock_accept`.
	LogScopeSock = logging.LogScopeSock
	// LogScopeAll means all functions should be logged.
	LogScopeAll = logging.LogScopeAll
)

// NewLoggingListenerFactory is an experimental.FunctionListenerFactory that
// logs all functions that have a name to the writer.
//
// Use NewHostLoggingListenerFactory if only interested in host interactions.
func NewLoggingListenerFactory(w Writer) experimental.FunctionListenerFactory {
	return &loggingListenerFactory{w: toInternalWriter(w), scopes: LogScopeAll}
}

// NewHostLoggingListenerFactory is an experimental.FunctionListenerFactory
// that logs exported and host functions to the writer.
//
// This is an alternative to NewLoggingListenerFactory, and would weed out
// guest defined functions such as those implementing garbage collection.
//
// For example, "_start" is defined by the guest, but exported, so would be
// written to the w in order to provide minimal context needed to
// understand host calls such as "args_get".
//
// The scopes parameter can be set to LogScopeAll or constrained.
func NewHostLoggingListenerFactory(w Writer, scopes logging.LogScopes) experimental.FunctionListenerFactory {
	return &loggingListenerFactory{w: toInternalWriter(w), hostOnly: true, scopes: scopes}
}

func toInternalWriter(w Writer) logging.Writer {
	if w, ok := w.(logging.Writer); ok {
		return w
	}
	return bufio.NewWriter(w)
}

type loggingListenerFactory struct {
	w        logging.Writer
	hostOnly bool
	scopes   logging.LogScopes
	stack    logStack
}

type flusher interface {
	Flush() error
}

// NewFunctionListener implements the same method as documented on
// experimental.FunctionListener.
func (f *loggingListenerFactory) NewFunctionListener(fnd api.FunctionDefinition) experimental.FunctionListener {
	exported := len(fnd.ExportNames()) > 0
	if f.hostOnly && // choose functions defined or callable by the host
		fnd.GoFunction() == nil && // not defined by the host
		!exported { // not callable by the host
		return nil
	}

	var pLoggers []logging.ParamLogger
	var pSampler logging.ParamSampler
	var rLoggers []logging.ResultLogger
	switch fnd.ModuleName() {
	case wasip1.InternalModuleName:
		if !wasilogging.IsInLogScope(fnd, f.scopes) {
			return nil
		}
		pSampler, pLoggers, rLoggers = wasilogging.Config(fnd)
	case "env":
		// env is difficult because the same module name is used for different
		// ABI.
		pLoggers, rLoggers = logging.Config(fnd)
		switch fnd.Name() {
		case "emscripten_notify_memory_growth":
			if !logging.LogScopeMemory.IsEnabled(f.scopes) {
				return nil
			}
		default:
			if !aslogging.IsInLogScope(fnd, f.scopes) {
				return nil
			}
		}
	default:
		// We don't know the scope of the function, so compare against all.
		if f.scopes != logging.LogScopeAll {
			return nil
		}
		pLoggers, rLoggers = logging.Config(fnd)
	}

	var before, after string
	if fnd.GoFunction() != nil {
		before = "==> " + fnd.DebugName()
		after = "<=="
	} else {
		before = "--> " + fnd.DebugName()
		after = "<--"
	}
	return &loggingListener{
		w:            f.w,
		beforePrefix: before,
		afterPrefix:  after,
		pLoggers:     pLoggers,
		pSampler:     pSampler,
		rLoggers:     rLoggers,
		stack:        &f.stack,
	}
}

type logStack struct {
	params [][]uint64
}

func (s *logStack) push(params []uint64) {
	s.params = append(s.params, params)
}

func (s *logStack) pop() []uint64 {
	i := len(s.params) - 1
	params := s.params[i]
	s.params[i] = nil
	s.params = s.params[:i]
	return params
}

func (s *logStack) count() (n int) {
	for _, p := range s.params {
		if p != nil {
			n++
		}
	}
	return n
}

// loggingListener implements experimental.FunctionListener to log entrance and after
// of each function call.
type loggingListener struct {
	w                         logging.Writer
	beforePrefix, afterPrefix string
	pLoggers                  []logging.ParamLogger
	pSampler                  logging.ParamSampler
	rLoggers                  []logging.ResultLogger
	stack                     *logStack
}

// Before logs to stdout the module and function name, prefixed with '-->' and
// indented based on the call nesting level.
func (l *loggingListener) Before(ctx context.Context, mod api.Module, def api.FunctionDefinition, params []uint64, _ experimental.StackIterator) {
	// First, see if this invocation is sampled.
	sampled := true
	if s := l.pSampler; s != nil {
		sampled = s(ctx, mod, params)
	}

	if sampled {
		l.logIndented(l.stack.count(), l.beforePrefix, func() {
			l.logParams(ctx, mod, params)
		})
		params = append([]uint64{}, params...)
	} else {
		params = nil
	}

	l.stack.push(params)
}

// After logs to stdout the module and function name, prefixed with '<--' and
// indented based on the call nesting level.
func (l *loggingListener) After(ctx context.Context, mod api.Module, def api.FunctionDefinition, results []uint64) {
	if params := l.stack.pop(); params != nil {
		l.logIndented(l.stack.count(), l.afterPrefix, func() {
			l.logResults(ctx, mod, params, results)
		})
	}
}

func (l *loggingListener) Abort(ctx context.Context, mod api.Module, def api.FunctionDefinition, _ error) {
	l.stack.pop()
}

// logIndented writes an indentation level and prefix prior to calling log to
// output the log line.
func (l *loggingListener) logIndented(nestLevel int, prefix string, log func()) {
	for i := 0; i < nestLevel; i++ {
		l.w.WriteByte('\t') //nolint
	}
	l.w.WriteString(prefix) //nolint
	log()
	l.w.WriteByte('\n') //nolint

	if f, ok := l.w.(flusher); ok {
		f.Flush() //nolint
	}
}

func (l *loggingListener) logParams(ctx context.Context, mod api.Module, params []uint64) {
	paramLen := len(l.pLoggers)
	l.w.WriteByte('(') //nolint
	if paramLen > 0 {
		l.pLoggers[0](ctx, mod, l.w, params)
		for i := 1; i < paramLen; i++ {
			l.w.WriteByte(',') //nolint
			l.pLoggers[i](ctx, mod, l.w, params)
		}
	}
	l.w.WriteByte(')') //nolint
}

func (l *loggingListener) logResults(ctx context.Context, mod api.Module, params, results []uint64) {
	resultLen := len(l.rLoggers)
	if resultLen == 0 {
		return
	}
	l.w.WriteByte(' ') //nolint
	switch resultLen {
	case 1:
		l.rLoggers[0](ctx, mod, l.w, params, results)
	default:
		l.w.WriteByte('(') //nolint
		l.rLoggers[0](ctx, mod, l.w, params, results)
		for i := 1; i < resultLen; i++ {
			l.w.WriteByte(',') //nolint
			l.rLoggers[i](ctx, mod, l.w, params, results)
		}
		l.w.WriteByte(')') //nolint
	}
}
