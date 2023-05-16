// Package features implements a feature flagging mechanism for wazero.
//
// Features are intended to control properties of the code that can only be
// enabled globally.
package features

import (
	"os"
	"strings"
	"sync"
)

const (
	// EnvVarName is the name of the environment variable which contains the
	// list of feature flags.
	EnvVarName = "WAZEROFEATURES"
)

var (
	lock sync.RWMutex
	list []string
)

// EnableFromEnvironment extracts the list of wazero features enabled from the
// WAZEROFEATURES environment variable.
func EnableFromEnvironment() {
	features := os.Getenv(EnvVarName)
	Enable(strings.Split(features, ",")...)
}

// Enable the list of features passed as arguments.
//
// The function is idempotent and atomic, features that are already present are
// skipped.
//
// Unrecognized features are ignored.
func Enable(features ...string) {
	lock.Lock()
	defer lock.Unlock()

	enabled := list

	for _, f := range features {
		if supported(f) && !have(enabled, f) {
			enabled = append(enabled, f)
		}
	}

	list = enabled
}

// List returns the current list of features enabled on wazero.
//
// The program must treat the returned slice as read-only.
func List() []string {
	lock.RLock()
	defer lock.RUnlock()
	return list
}

// Have returns true if the given feature is enabled.
func Have(feature string) bool {
	lock.RLock()
	features := list
	lock.RUnlock()
	return have(features, feature)
}

func have(list []string, feature string) bool {
	for _, f := range list {
		if f == feature {
			return true
		}
	}
	return false
}

func supported(feature string) bool {
	switch feature {
	case "hugepages":
		return true
	default:
		return false
	}
}
