package main

import "os"

func main() {
	if err := cmd().Execute(); err != nil {
		os.Exit(1)
	}
}

// For more examples of using go-gh, see:
// https://github.com/cli/go-gh/blob/trunk/example_gh_test.go
