package amd64

type instruction struct {
	prev, next *instruction
}

func resetInstruction(i *instruction) {
	i.prev = nil
	i.next = nil
}

func setNext(i *instruction, next *instruction) {
	i.next = next
}

func setPrev(i *instruction, prev *instruction) {
	i.prev = prev
}

func asNop(*instruction) {
}
