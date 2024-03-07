# Stdlibs benchmarks

This directory contains a Makefile to build (a subset of) the stdlibs for Zig, TinyGo and Go (wasip1)
and test them against the compiler.

## Requirements

- Zig 0.11.0 in PATH, source code to zig 0.11.0
- TinyGo in PATH
- Go in PATH

## Usage

First, build the test suite (the Zig source root has to be set explicitly):

    make all zigroot=/path/to/zig/source

Then you can run the test suite against the compiler; e.g.:

    go test -bench=.

## Caveats

* The standard binary zig distribution does not ship some testdata.
  You should override with the zig source code path, otherwise some tests will fail.

* Some tests might fail if Go has been installed with homebrew because
  the file system layout is different than what the tests expect.
  Easiest fix is to install Go without using brew.

