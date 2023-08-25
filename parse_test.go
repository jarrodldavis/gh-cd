package main

import (
	"net/url"
	"testing"

	"github.com/google/go-cmp/cmp"
)

var diffOpts = cmp.AllowUnexported(parsed{}, url.Userinfo{})

func assertParse(input string, want parsed) func(t *testing.T) {
	return func(t *testing.T) {
		t.Helper()
		got := parse(input)

		if diff := cmp.Diff(&want, got, diffOpts); diff != "" {
			t.Errorf("parse(%#v) = %s, want %s:\n%s", input, got, want, diff)
		}
	}
}

func TestParseGitHubSyntax(t *testing.T) {
	t.Run("Name", assertParse("name", parsed{
		local:  []string{"github.com", "owner", "name"},
		remote: &url.URL{Scheme: "https", Host: "github.com", Path: "/owner/name.git"},
	}))

	t.Run("OwnerName", assertParse("owner/name", parsed{
		local:  []string{"github.com", "owner", "name"},
		remote: &url.URL{Scheme: "https", Host: "github.com", Path: "/owner/name.git"},
	}))

	t.Run("HostOwnerName", assertParse("host.xz/owner/name", parsed{
		local:  []string{"host.xz", "owner", "name"},
		remote: &url.URL{Scheme: "https", Host: "host.xz", Path: "/owner/name.git"},
	}))
}

func TestParseSSHSyntax(t *testing.T) {
	t.Run("NoUserNoPort", assertParse("ssh://host.xz/path/to/repo.git/", parsed{
		local:  []string{"host.xz", "path", "to", "repo"},
		remote: &url.URL{Scheme: "ssh", Host: "host.xz", Path: "/path/to/repo.git"},
	}))

	t.Run("WithUserNoPort", assertParse("ssh://user@host.xz/path/to/repo.git/", parsed{
		local:  []string{"host.xz", "path", "to", "repo"},
		remote: &url.URL{Scheme: "ssh", User: url.User("user"), Host: "host.xz", Path: "/path/to/repo.git"},
	}))

	t.Run("NoUserWithPort", assertParse("ssh://host.xz:22/path/to/repo.git/", parsed{
		local:  []string{"host.xz", "path", "to", "repo"},
		remote: &url.URL{Scheme: "ssh", Host: "host.xz:22", Path: "/path/to/repo.git"},
	}))

	t.Run("WithUserWithPort", assertParse("ssh://user@host.xz:22/path/to/repo.git/", parsed{
		local:  []string{"host.xz", "path", "to", "repo"},
		remote: &url.URL{Scheme: "ssh", User: url.User("user"), Host: "host.xz:22", Path: "/path/to/repo.git"},
	}))
}

func TestParseGitSyntax(t *testing.T) {
	t.Run("NoPort", assertParse("git://host.xz/path/to/repo.git/", parsed{
		local:  []string{"host.xz", "path", "to", "repo"},
		remote: &url.URL{Scheme: "git", Host: "host.xz", Path: "/path/to/repo.git"},
	}))

	t.Run("WithPort", assertParse("git://host.xz:9418/path/to/repo.git/", parsed{
		local:  []string{"host.xz", "path", "to", "repo"},
		remote: &url.URL{Scheme: "git", Host: "host.xz:9418", Path: "/path/to/repo.git"},
	}))
}

func TestParseHTTPSyntax(t *testing.T) {
	t.Run("InsecureNoPort", assertParse("http://host.xz/path/to/repo.git/", parsed{
		local:  []string{"host.xz", "path", "to", "repo"},
		remote: &url.URL{Scheme: "http", Host: "host.xz", Path: "/path/to/repo.git"},
	}))

	t.Run("SecureNoPort", assertParse("https://host.xz/path/to/repo.git/", parsed{
		local:  []string{"host.xz", "path", "to", "repo"},
		remote: &url.URL{Scheme: "https", Host: "host.xz", Path: "/path/to/repo.git"},
	}))

	t.Run("InsecureWithPort", assertParse("http://host.xz:80/path/to/repo.git/", parsed{
		local:  []string{"host.xz", "path", "to", "repo"},
		remote: &url.URL{Scheme: "http", Host: "host.xz:80", Path: "/path/to/repo.git"},
	}))

	t.Run("SecureWithPort", assertParse("https://host.xz:443/path/to/repo.git/", parsed{
		local:  []string{"host.xz", "path", "to", "repo"},
		remote: &url.URL{Scheme: "https", Host: "host.xz:443", Path: "/path/to/repo.git"},
	}))
}

func TestParseFTPSyntax(t *testing.T) {
	t.Run("InsecureNoPort", assertParse("ftp://host.xz/path/to/repo.git/", parsed{
		local:  []string{"host.xz", "path", "to", "repo"},
		remote: &url.URL{Scheme: "ftp", Host: "host.xz", Path: "/path/to/repo.git"},
	}))

	t.Run("SecureNoPort", assertParse("ftps://host.xz/path/to/repo.git/", parsed{
		local:  []string{"host.xz", "path", "to", "repo"},
		remote: &url.URL{Scheme: "ftps", Host: "host.xz", Path: "/path/to/repo.git"},
	}))

	t.Run("InsecureWithPort", assertParse("ftp://host.xz:21/path/to/repo.git/", parsed{
		local:  []string{"host.xz", "path", "to", "repo"},
		remote: &url.URL{Scheme: "ftp", Host: "host.xz:21", Path: "/path/to/repo.git"},
	}))

	t.Run("SecureWithPort", assertParse("ftps://host.xz:990/path/to/repo.git/", parsed{
		local:  []string{"host.xz", "path", "to", "repo"},
		remote: &url.URL{Scheme: "ftps", Host: "host.xz:990", Path: "/path/to/repo.git"},
	}))
}

func TestParseSCPSyntax(t *testing.T) {
	t.Run("NoUser", assertParse("host.xz:path/to/repo.git/", parsed{
		local:  []string{"host.xz", "path", "to", "repo"},
		remote: &url.URL{Scheme: "ssh", Host: "host.xz", Path: "/path/to/repo.git"},
	}))

	t.Run("WithUser", assertParse("user@host.xz:path/to/repo.git/", parsed{
		local:  []string{"host.xz", "path", "to", "repo"},
		remote: &url.URL{Scheme: "ssh", User: url.User("user"), Host: "host.xz", Path: "/path/to/repo.git"},
	}))
}
