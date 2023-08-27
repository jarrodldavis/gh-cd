package main

import (
	"fmt"
	"os"

	"github.com/cli/go-gh/v2/pkg/api"
)

func main() {
	fmt.Println("hi world, this is the gh-cd extension!")
	client, err := api.DefaultRESTClient()
	if err != nil {
		fmt.Println(err)
		return
	}
	response := struct{ Login string }{}
	err = client.Get("user", &response)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Printf("running as %s\n", response.Login)

	parsed := parse(os.Args[1])

	if parsed == nil {
		fmt.Printf("parsed=%v\n", parsed)
	} else {
		fmt.Printf("local=%#v, parsed=%#v\n", parsed.local, parsed.remote)
	}
}

// For more examples of using go-gh, see:
// https://github.com/cli/go-gh/blob/trunk/example_gh_test.go
