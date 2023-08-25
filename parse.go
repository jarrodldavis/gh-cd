package main

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/cli/go-gh/v2/pkg/repository"
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
	if strings.Contains(s, "://") {
		remote, err := url.ParseRequestURI(s)

		if err != nil {
			return nil
		}

		if _, accepted := acceptedSchemes[remote.Scheme]; !accepted {
			return nil
		}

		path := remote.Path
		path = strings.Trim(path, "/")
		path = strings.TrimSuffix(path, ".git")

		remote.Path = "/" + path + ".git"
		segments := strings.Split(path, "/")

		local := make([]string, 0, 1+len(segments))
		local = append(local, remote.Hostname())
		local = append(local, segments...)

		return &parsed{local, remote}
	} else if strings.Contains(s, ":") {
		// TODO: SCP syntax
		return nil
	} else {
		repo, err := repository.Parse(s)

		if err != nil {
			return nil
		}

		path := "/" + repo.Owner + "/" + repo.Name + ".git"
		remote := &url.URL{Scheme: "https", Host: repo.Host, Path: path}
		local := []string{repo.Host, repo.Owner, repo.Name}
		return &parsed{local, remote}
	}
}
