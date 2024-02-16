package main

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"
	"path"
)

const failedCasesDir = "wazerolib/testdata"

// saveFailedBinary writes binary and wat into failedCasesDir so that it is easy to reproduce the error.
func saveFailedBinary(bin []byte, reproduceTestName string) {
	checksum := sha256.Sum256(bin)
	checkSumStr := hex.EncodeToString(checksum[:])

	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	testDataDir := path.Join(dir, failedCasesDir)
	binaryPath := path.Join(testDataDir, fmt.Sprintf("%s.wasm", checkSumStr))
	f, err := os.Create(binaryPath)
	if err != nil {
		panic(err)
	}

	defer f.Close()

	_, err = f.Write(bin)
	if err != nil {
		panic(err)
	}

	fmt.Printf(`
Failed WebAssembly Binary in hex: %s
Failed Wasm binary has been written to %s
To reproduce the failure, execute: WASM_BINARY_PATH=%s go test -run=%s ./wazerolib/...
`, hex.EncodeToString(bin), binaryPath, binaryPath, reproduceTestName)
}
