//go:generate tinygo build -opt=s -o panic.wasm -target wasm panic.go
package main

func main() {}

//export cause_panic
func causePanic() {
	panic("causing panic!!!!!!!!!!")
}
