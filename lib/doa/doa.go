package doa

// Doa lets you test if a condition in your code returns true, if not, the program will panic.
func Doa(b bool) {
	if !b {
		panic("unreachable")
	}
}

// Nil lets you test if an error in your code is nil, if not, the program will panic.
func Nil(err error) {
	if err != nil {
		panic(err)
	}
}

// Try will give you the embedded value if there is no error returns. If instead error then it will panic.
func Try[T any](a T, err error) T {
	if err != nil {
		panic(err)
	}
	return a
}

// Err just returns error.
func Err[T any](a T, err error) error {
	return err
}
