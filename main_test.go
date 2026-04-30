package main

import (
	"io"
	"os"
	"testing"
)

func TestMainReturnsForHelp(t *testing.T) {
	originalArgs := os.Args
	originalStdout := os.Stdout
	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe returned error: %v", err)
	}
	os.Args = []string{"alienshard", "--help"}
	os.Stdout = writeEnd
	t.Cleanup(func() {
		os.Args = originalArgs
		os.Stdout = originalStdout
		_ = readEnd.Close()
	})

	main()
	if err := writeEnd.Close(); err != nil {
		t.Fatalf("stdout pipe close returned error: %v", err)
	}
	_, _ = io.Copy(io.Discard, readEnd)
}
