package main

func main() {
	// Go compiles into Wasm with a 16MB heap.
	// As there will be some used already, allocating 16MB should force growth.
	_ = make([]byte, 16*1024*1024)
}
