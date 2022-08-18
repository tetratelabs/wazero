package wazero

import (
	"runtime/debug"
	"strings"
)

// wazeroVersion holds the current version of wazero.
var wazeroVersion = "dev"

func init() {
	info, ok := debug.ReadBuildInfo()
	if ok {
		for _, dep := range info.Deps {
			// Note: here's the assumption that wazero is imported as github.com/tetratelabs/wazero.
			if strings.Contains(dep.Path, "github.com/tetratelabs/wazero") {
				wazeroVersion = dep.Version
			}
		}
	}
}
