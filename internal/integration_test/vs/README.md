# VS

This directory contains tests which compare against other runtimes. As all
known alternatives use CGO, this contains its own [go.mod](go.mod), as
otherwise project dependencies are tainted and multi-platform tests more
difficult to manage.

Examples of portability issues besides CGO
* Wasmtime can only be used in amd64
* Wasmer doesn't link on Windows
