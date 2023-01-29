package logging

import (
	"github.com/tetratelabs/wazero/api"
	. "github.com/tetratelabs/wazero/internal/assemblyscript"
	"github.com/tetratelabs/wazero/internal/logging"
)

func isExitFunction(fnd api.FunctionDefinition) bool {
	return fnd.ExportNames()[0] == AbortName
}

func isRandomFunction(fnd api.FunctionDefinition) bool {
	return fnd.ExportNames()[0] == SeedName
}

// IsInLogScope returns true if the current function is in any of the scopes.
func IsInLogScope(fnd api.FunctionDefinition, scopes logging.LogScopes) bool {
	if scopes.IsEnabled(logging.LogScopeExit) {
		if isExitFunction(fnd) {
			return true
		}
	}

	if scopes.IsEnabled(logging.LogScopeRandom) {
		if isRandomFunction(fnd) {
			return true
		}
	}

	return scopes == logging.LogScopeAll
}

func Config(fnd api.FunctionDefinition) (pSampler logging.ParamSampler, pLoggers []logging.ParamLogger, rLoggers []logging.ResultLogger) {
	pLoggers, rLoggers = logging.Config(fnd)
	return
}
