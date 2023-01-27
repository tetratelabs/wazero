package logging

import (
	"bufio"
	"context"
	"io"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	gologging "github.com/tetratelabs/wazero/internal/gojs/logging"
	"github.com/tetratelabs/wazero/internal/logging"
	"github.com/tetratelabs/wazero/internal/wasi_snapshot_preview1"
	wasilogging "github.com/tetratelabs/wazero/internal/wasi_snapshot_preview1/logging"
)

type Writer interface {
	io.Writer
	io.StringWriter
}

// NewLoggingListenerFactory is an experimental.FunctionListenerFactory that
// logs all functions that have a name to the writer.
//
// Use NewHostLoggingListenerFactory if only interested in host interactions.
func NewLoggingListenerFactory(w Writer) experimental.FunctionListenerFactory {
	return &loggingListenerFactory{w: toInternalWriter(w)}
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
func NewHostLoggingListenerFactory(w Writer) experimental.FunctionListenerFactory {
	return &loggingListenerFactory{w: toInternalWriter(w), hostOnly: true}
}

// NewScopedLoggingListenerFactory is an experimental.FunctionListenerFactory
// that logs exported filesystem functions to the writer.
//
// This is an alternative to NewHostLoggingListenerFactory.
func NewScopedLoggingListenerFactory(w Writer, scopes logging.LogScopes) experimental.FunctionListenerFactory {
	return &loggingListenerFactory{w: toInternalWriter(w), scopes: scopes}
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
}

type flusher interface {
	Flush() error
}

// NewListener implements the same method as documented on
// experimental.FunctionListener.
func (f *loggingListenerFactory) NewListener(fnd api.FunctionDefinition) experimental.FunctionListener {
	exported := len(fnd.ExportNames()) > 0
	if (f.hostOnly || f.scopes.Defined()) && // choose functions defined or callable by the host
		fnd.GoFunction() == nil && // not defined by the host
		!exported { // not callable by the host
		return nil
	}

	var pLoggers []logging.ParamLogger
	var pSampler logging.ParamSampler
	var rLoggers []logging.ResultLogger
	switch fnd.ModuleName() {
	case wasi_snapshot_preview1.InternalModuleName:
		if f.scopes.Defined() && !wasilogging.IsInLogScope(fnd, f.scopes) {
			return nil
		}
		pSampler, pLoggers, rLoggers = wasilogging.Config(fnd)
	case "go":
		if f.scopes.Defined() && !gologging.IsInLogScope(fnd, f.scopes) {
			return nil
		}
		pSampler, pLoggers, rLoggers = gologging.Config(fnd)
	default:
		if f.scopes.Defined() {
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
	}
}

// logState saves a copy of params between calls as the slice underlying them
// is a stack reused for results.
type logState struct {
	w         logging.Writer
	nestLevel int
	params    []uint64
}

// loggingListener implements experimental.FunctionListener to log entrance and after
// of each function call.
type loggingListener struct {
	w                         logging.Writer
	beforePrefix, afterPrefix string
	pLoggers                  []logging.ParamLogger
	pSampler                  logging.ParamSampler
	rLoggers                  []logging.ResultLogger
}

// Before logs to stdout the module and function name, prefixed with '-->' and
// indented based on the call nesting level.
func (l *loggingListener) Before(ctx context.Context, mod api.Module, _ api.FunctionDefinition, params []uint64) context.Context {
	if s := l.pSampler; s != nil && !s(ctx, mod, params) {
		return ctx
	}

	var nestLevel int
	if ls := ctx.Value(logging.LoggerKey{}); ls != nil {
		nestLevel = ls.(*logState).nestLevel
	}
	nestLevel++

	l.logIndented(ctx, mod, nestLevel, true, params, nil, nil)

	ls := &logState{w: l.w, nestLevel: nestLevel}
	if pLen := len(params); pLen > 0 {
		ls.params = make([]uint64, pLen)
		copy(ls.params, params) // safe copy
	} else { // empty
		ls.params = params
	}

	// Increase the next nesting level.
	return context.WithValue(ctx, logging.LoggerKey{}, ls)
}

// After logs to stdout the module and function name, prefixed with '<--' and
// indented based on the call nesting level.
func (l *loggingListener) After(ctx context.Context, mod api.Module, _ api.FunctionDefinition, err error, results []uint64) {
	// Note: We use the nest level directly even though it is the "next" nesting level.
	// This works because our indent of zero nesting is one tab.
	if state, ok := ctx.Value(logging.LoggerKey{}).(*logState); ok {
		l.logIndented(ctx, mod, state.nestLevel, false, state.params, err, results)
	}
}

// logIndented logs an indented l.w like this: "-->\t\t\t$nestLevel$funcName\n"
func (l *loggingListener) logIndented(ctx context.Context, mod api.Module, nestLevel int, isBefore bool, params []uint64, err error, results []uint64) {
	for i := 1; i < nestLevel; i++ {
		l.w.WriteByte('\t') //nolint
	}
	if isBefore { // before
		l.w.WriteString(l.beforePrefix) //nolint
		l.logParams(ctx, mod, params)
	} else { // after
		l.w.WriteString(l.afterPrefix) //nolint
		if err != nil {
			l.w.WriteString(" error: ")  //nolint
			l.w.WriteString(err.Error()) //nolint
		} else {
			l.logResults(ctx, mod, params, results)
		}
	}
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
