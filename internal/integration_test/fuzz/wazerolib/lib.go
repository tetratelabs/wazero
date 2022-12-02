package main

import "C"
import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path"
)

func main() {}

const failedCasesDir = "wazerolib/testdata"

// saveFailedBinary writes binary and wat into failedCasesDir so that it is easy to reproduce the error.
func saveFailedBinary(bin []byte, wat string, reproduceTestName string) {
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

	var watPath string
	if len(wat) != 0 {
		watPath = path.Join(testDataDir, fmt.Sprintf("%s.wat", checkSumStr))
		watF, err := os.Create(watPath)
		if err != nil {
			panic(err)
		}

		defer watF.Close()

		_, err = watF.Write([]byte(wat))
		if err != nil {
			panic(err)
		}
		wat = "N/A"
		watPath = "N/A"
	}
	fmt.Printf(`
Failed WebAssembly Text:
%s

Failed Wasm binary has been written to %s
Failed Wasm Text has been written to %s
To reproduce the failure, execute: WASM_BINARY_PATH=%s go test -run=%s ./wazerolib/...


`, wat, binaryPath, watPath, reproduceTestName, binaryPath)
}
