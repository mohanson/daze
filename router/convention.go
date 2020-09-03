package router

// A Road represents a host's road mode.
type Road uint32

const (
	Direct Road = iota // Don't need a proxy
	Daze               // This network are accessed through daze
	Fucked             // Pure rubbish
	Puzzle             // ?
)
