//go:generate tinygo build -opt=s -o fib.wasm -target wasm fib.go
package main

func main() {}

//export fib
func fib(in uint32) uint32 {
	if in <= 1 {
		return in
	}
	return fib(in-1) + fib(in-2)
}
