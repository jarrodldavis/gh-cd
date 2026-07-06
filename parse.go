package main

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"
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

func login() (string, error) {
	client, err := api.DefaultRESTClient()
	if err != nil {
		return "", err
	}
	response := struct{ Login string }{}
	err = client.Get("user", &response)
	if err != nil {
		return "", err
	}
	return response.Login, nil
}

func normalize(remote *url.URL) *parsed {
	path := remote.Path
	path = strings.Trim(path, "/")
	path = strings.TrimSuffix(path, ".git")
	if remote.Hostname() == "" || path == "" {
		return nil
	}

	remote.Path = "/" + path + ".git"
	segments := strings.Split(path, "/")
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return nil
		}
	}

	local := make([]string, 0, 1+len(segments))
	local = append(local, remote.Hostname())
	local = append(local, segments...)

	return &parsed{local, remote}
}

func parse(s string) *parsed {
	if s == "" {
		return nil
	}

	if strings.Contains(s, "://") {
		remote, err := url.ParseRequestURI(s)

		if err != nil {
			return nil
		}

		if _, accepted := acceptedSchemes[remote.Scheme]; !accepted {
			return nil
		}

		return normalize(remote)
	} else if host, path, found := strings.Cut(s, ":"); found {
		if len(host) == 0 || len(path) == 0 {
			return nil
		}

		remote := &url.URL{Scheme: "ssh", Host: host, Path: path}
		if user, host, found := strings.Cut(host, "@"); found {
			if len(user) == 0 || len(host) == 0 {
				return nil
			}

			remote.User = url.User(user)
			remote.Host = host
		}

		return normalize(remote)
	} else {
		if !strings.Contains(s, "/") {
			owner, err := login()
			if err != nil {
				return nil
			}
			s = owner + "/" + s
		}

		repo, err := repository.Parse(s)

		if err != nil {
			return nil
		}

		repo.Name = strings.TrimSuffix(repo.Name, ".git")
		path := "/" + repo.Owner + "/" + repo.Name + ".git"
		remote := &url.URL{Scheme: "https", Host: repo.Host, Path: path}
		local := []string{repo.Host, repo.Owner, repo.Name}
		return &parsed{local, remote}
	}
}
