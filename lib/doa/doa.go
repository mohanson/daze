// Package doa stands for "dead or alive". It provides simple utilities to intentionally crash the program with a panic.
package doa

// Doa checks a boolean condition and triggers a panic if it’s false.
func Doa(b bool) {
	if !b {
		panic("unreachable")
	}
}

// Err returns the error passed to it, ignoring the first argument.
func Err[T any](a T, err error) error {
	return err
}

// Nil checks if an error is non-nil and panics if it is.
func Nil(err error) {
	if err != nil {
		panic(err)
	}
}

// Try returns a value if there’s no error, otherwise it panics.
func Try[T any](a T, err error) T {
	if err != nil {
		panic(err)
	}
	return a
}

// Val returns the first argument, ignoring the error.
func Val[T any](a T, err error) T {
	return a
}
