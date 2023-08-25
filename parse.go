package main

import (
	"fmt"
	"net/url"
	"strings"
)

var acceptedSchemes = map[string]struct{}{
	"ssh":   {},
	"git":   {},
	"http":  {},
	"https": {},
	"ftp":   {},
	"ftps":  {},
}

type parsed struct {
	local  []string
	remote *url.URL
}

func (p parsed) String() string {
	local := strings.Join(p.local, "/")

	if p.remote == nil {
		return fmt.Sprintf("`%s` -> ``", local)
	} else {
		return fmt.Sprintf("`%s` -> `%s`", local, p.remote)
	}
}

func parse(s string) *parsed {
	return &parsed{}
}
