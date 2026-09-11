package main

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/repository"
)

var errInvalidRepository = errors.New("invalid repository argument")

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

func parse(s string) (*parsed, error) {
	if s == "" {
		return nil, errInvalidRepository
	}

	if strings.Contains(s, "://") {
		remote, err := url.ParseRequestURI(s)

		if err != nil {
			return nil, fmt.Errorf("%w: %v", errInvalidRepository, err)
		}

		if _, accepted := acceptedSchemes[remote.Scheme]; !accepted {
			return nil, errInvalidRepository
		}

		parsed := normalize(remote)
		if parsed == nil {
			return nil, errInvalidRepository
		}
		return parsed, nil
	} else if host, path, found := strings.Cut(s, ":"); found {
		if len(host) == 0 || len(path) == 0 {
			return nil, errInvalidRepository
		}

		remote := &url.URL{Scheme: "ssh", Host: host, Path: path}
		if user, host, found := strings.Cut(host, "@"); found {
			if len(user) == 0 || len(host) == 0 {
				return nil, errInvalidRepository
			}

			remote.User = url.User(user)
			remote.Host = host
		}

		parsed := normalize(remote)
		if parsed == nil {
			return nil, errInvalidRepository
		}
		return parsed, nil
	} else {
		if !strings.Contains(s, "/") {
			owner, err := login()
			if err != nil {
				return nil, fmt.Errorf("failed to determine repository owner: %w", err)
			}
			s = owner + "/" + s
		}

		repo, err := repository.Parse(s)

		if err != nil {
			return nil, fmt.Errorf("%w: %v", errInvalidRepository, err)
		}

		repo.Name = strings.TrimSuffix(repo.Name, ".git")
		path := repo.Owner + "/" + repo.Name
		if strings.Count(s, "/") == 2 {
			path = repo.Host + "/" + path
		}
		remote := &url.URL{Path: path}
		local := []string{repo.Host, repo.Owner, repo.Name}
		return &parsed{local, remote}, nil
	}
}
