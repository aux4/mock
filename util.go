package main

import (
	"io"
	"os"
)

func readAllStdin() ([]byte, error) {
	return io.ReadAll(os.Stdin)
}
