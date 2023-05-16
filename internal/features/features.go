// Package features implements a feature flagging mechanism for wazero.
//
// Wazero feature flags are passed in a WAZEROFEATURES environment variable as a
// comma-seprated list of features to enable on a run of wazero.
package features

import (
	"os"
	"strings"
)

const (
	// EnvVarName is the name of the environment variable which contains the
	// list of feature flags.
	EnvVarName = "WAZEROFEATURES"
)

// List returns the current list of features enabled on wazero.
func List() []string {
	features := os.Getenv(EnvVarName)
	return strings.Split(features, ",")
}

// Enabled returns true if the given feature is enabled.
func Enabled(feature string) bool {
	features := os.Getenv(EnvVarName)

	for features != "" {
		var f string
		f, features, _ = strings.Cut(features, ",")
		if feature == f {
			return true
		}
	}

	return false
}
