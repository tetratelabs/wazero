package version

import (
	"fmt"
	"runtime/debug"
	"strings"
)

// WazeroVersionKey is the key for holding wazero's version in context.Context.
type WazeroVersionKey struct{}

// GetWazeroVersion returns the current version of wazero in the go.mod.
// This assumes that users of wazero imports wazero as "github.com/tetratelabs/wazero".
// To be precise, the returned string matches the require statement there.
// For example, if the go.mod has "require github.com/tetratelabs/wazero 0.1.2-12314124-abcd",
// then this returns "0.1.2-12314124-abcd".
//
// Note: this is tested in ./test/main_test.go with a separate go.mod to pretend as the wazero user.
func GetWazeroVersion() (ret string) {
	ret = "dev"
	info, ok := debug.ReadBuildInfo()
	if ok {
		for _, dep := range info.Deps {
			// Note: here's the assumption that wazero is imported as github.com/tetratelabs/wazero.
			if strings.Contains(dep.Path, "github.com/tetratelabs/wazero") {
				fmt.Println(dep.Sum)
				ret = dep.Version
			}
		}
	}
	return
}
