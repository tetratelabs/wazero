package main

import (
	"fmt"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_zig_os(t *testing.T) {
	args := []string{
		"run", "-mount=:/",
		"--hostlogging=filesystem",
		"zig_os_test.wasm",
	}
	exitCode, stdOut, stdErr := runMain(t, args)
	fmt.Println(stdErr)
	fmt.Println(stdOut)
	require.Equal(t, 0, exitCode)
}

func Test_zig_fs(t *testing.T) {
	args := []string{
		"run", "-mount=:/",
		"--hostlogging=filesystem",
		"zig_fs_test.wasm",
	}
	exitCode, stdOut, stdErr := runMain(t, args)
	fmt.Println(stdErr)
	fmt.Println(stdOut)
	require.Equal(t, 0, exitCode)
}
