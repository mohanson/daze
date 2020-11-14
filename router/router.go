package router

// A Road represents a host's road mode.
type Road uint32

const (
	Direct Road = iota // Don't need a proxy
	Daze               // This network are accessed through daze
	Fucked             // Pure rubbish
	Puzzle             // ?
)

// Router is a selector that will judge the host address.
type Router interface {
	// The host must be a literal IP address, or a host name that can be resolved to IP addresses.
	// Examples:
	//	 Choose("golang.org")
	//	 Choose("192.0.2.1")
	Choose(host string) Road
}
