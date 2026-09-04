package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func executeTestCmd(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := cmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)

	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func setTestHome(t *testing.T) string {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func installFakeGH(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	logPath := filepath.Join(dir, "args")
	ghPath := filepath.Join(dir, "gh")
	script := `#!/bin/sh
printf '%s\n' "$@" > "$GH_CD_FAKE_ARGS"
printf 'clone stdout\n'
printf 'clone stderr\n' >&2
if [ "${GH_CD_FAKE_EXIT:-0}" -eq 0 ]; then
  git init -q "$4"
  git -C "$4" remote add origin https://github.com/owner/repo.git
fi
exit "${GH_CD_FAKE_EXIT:-0}"
`
	if err := os.WriteFile(ghPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("GH_CD_FAKE_ARGS", logPath)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func TestCmdRequiresRepository(t *testing.T) {
	_, _, err := executeTestCmd(t)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCmdRejectsExtraArgsWithoutDash(t *testing.T) {
	_, _, err := executeTestCmd(t, "owner/repo", "--depth=1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCmdPrintsExistingClone(t *testing.T) {
	home := setTestHome(t)
	wantPath := filepath.Join(home, "git", "github.com", "owner", "repo")
	if err := os.MkdirAll(wantPath, 0o755); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, wantPath, "init", "-q")
	runTestGit(t, wantPath, "remote", "add", "origin", "https://github.com/owner/repo.git")

	stdout, stderr, err := executeTestCmd(t, "owner/repo")
	if err != nil {
		t.Fatal(err)
	}

	if stdout != wantPath+"\n" {
		t.Fatalf("stdout = %q, want %q", stdout, wantPath+"\n")
	}
	if stderr != "using existing clone: "+wantPath+"\n" {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestAddReviewRefspecs(t *testing.T) {
	repo := t.TempDir()
	runTestGit(t, repo, "init", "-q")
	runTestGit(t, repo, "remote", "add", "origin", "git@github.com:owner/repo.git")
	runTestGit(t, repo, "remote", "add", "upstream", "ssh://git@github.com/parent/repo.git")
	runTestGit(t, repo, "remote", "add", "mirror", "https://gitlab.com/owner/repo.git")

	if err := addReviewRefspecs(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	if err := addReviewRefspecs(context.Background(), repo); err != nil {
		t.Fatal(err)
	}

	want := map[string][]string{
		"origin": {
			"+refs/heads/*:refs/remotes/origin/*",
			"+refs/pull/*/head:refs/remotes/origin/pr/*",
		},
		"upstream": {
			"+refs/heads/*:refs/remotes/upstream/*",
			"+refs/pull/*/head:refs/remotes/upstream/pr/*",
		},
		"mirror": {
			"+refs/heads/*:refs/remotes/mirror/*",
			"+refs/merge-requests/*/head:refs/remotes/mirror/mr/*",
		},
	}
	for remote, wantFetches := range want {
		got, err := gitConfigValues(context.Background(), repo, "remote."+remote+".fetch")
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(wantFetches, got); diff != "" {
			t.Errorf("%s fetch refspecs mismatch (-want +got):\n%s", remote, diff)
		}
	}
}

func TestAddReviewRefspecsUsesPrimaryFetchURL(t *testing.T) {
	repo := t.TempDir()
	runTestGit(t, repo, "init", "-q")
	runTestGit(t, repo, "remote", "add", "origin", "https://example.com/owner/repo.git")
	runTestGit(t, repo, "config", "--add", "remote.origin.url", "https://github.com/owner/repo.git")

	if err := addReviewRefspecs(context.Background(), repo); err != nil {
		t.Fatal(err)
	}

	fetches, err := gitConfigValues(context.Background(), repo, "remote.origin.fetch")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"+refs/heads/*:refs/remotes/origin/*"}
	if diff := cmp.Diff(want, fetches); diff != "" {
		t.Fatalf("fetch refspecs mismatch (-want +got):\n%s", diff)
	}
}

func TestAddReviewRefspecsReplacesProviderRefspec(t *testing.T) {
	repo := t.TempDir()
	runTestGit(t, repo, "init", "-q")
	runTestGit(t, repo, "remote", "add", "origin", "https://github.com/owner/repo.git")

	if err := addReviewRefspecs(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, repo, "remote", "set-url", "origin", "https://gitlab.com/owner/repo.git")
	if err := addReviewRefspecs(context.Background(), repo); err != nil {
		t.Fatal(err)
	}

	fetches, err := gitConfigValues(context.Background(), repo, "remote.origin.fetch")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"+refs/heads/*:refs/remotes/origin/*",
		"+refs/merge-requests/*/head:refs/remotes/origin/mr/*",
	}
	if diff := cmp.Diff(want, fetches); diff != "" {
		t.Fatalf("fetch refspecs mismatch (-want +got):\n%s", diff)
	}
}

func TestAddReviewRefspecsIgnoresUnsupportedHosts(t *testing.T) {
	repo := t.TempDir()
	runTestGit(t, repo, "init", "-q")
	runTestGit(t, repo, "remote", "add", "origin", "https://github.com/owner/repo.git")

	if err := addReviewRefspecs(context.Background(), repo); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, repo, "remote", "set-url", "origin", "https://bitbucket.org/owner/repo.git")
	if err := addReviewRefspecs(context.Background(), repo); err != nil {
		t.Fatal(err)
	}

	fetches, err := gitConfigValues(context.Background(), repo, "remote.origin.fetch")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"+refs/heads/*:refs/remotes/origin/*"}
	if diff := cmp.Diff(want, fetches); diff != "" {
		t.Fatalf("fetch refspecs mismatch (-want +got):\n%s", diff)
	}
}

func TestAddReviewRefspecsIgnoresGlobalRemotes(t *testing.T) {
	repo := t.TempDir()
	globalConfig := filepath.Join(t.TempDir(), "gitconfig")
	t.Setenv("GIT_CONFIG_GLOBAL", globalConfig)
	runTestGit(t, repo, "init", "-q")
	runTestGit(t, repo, "config", "--global", "remote.phantom.url", "https://github.com/owner/repo.git")
	runTestGit(t, repo, "config", "--global", "remote.phantom.fetch", "+refs/heads/*:refs/remotes/phantom/*")

	if err := addReviewRefspecs(context.Background(), repo); err != nil {
		t.Fatal(err)
	}

	remotes, err := localRemoteNames(context.Background(), repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(remotes) != 0 {
		t.Fatalf("local remotes = %q, want none", remotes)
	}
	fetches, err := gitConfigValues(context.Background(), repo, "remote.phantom.fetch")
	if err != nil {
		t.Fatal(err)
	}
	if len(fetches) != 0 {
		t.Fatalf("local phantom fetch refspecs = %q, want none", fetches)
	}
}

func TestAddReviewRefspecsReportsGitError(t *testing.T) {
	repo := t.TempDir()

	err := addReviewRefspecs(context.Background(), repo)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "git config --local") {
		t.Fatalf("err = %q, want command context", err)
	}
	if !strings.Contains(err.Error(), "fatal:") {
		t.Fatalf("err = %q, want git stderr", err)
	}
}

func runTestGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}

func TestCmdRejectsExistingNonDirectory(t *testing.T) {
	home := setTestHome(t)
	local := filepath.Join(home, "git", "github.com", "owner", "repo")
	if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(local, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := executeTestCmd(t, "owner/repo")
	if err == nil {
		t.Fatal("expected error")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
}

func TestCmdClonesMissingRepository(t *testing.T) {
	home := setTestHome(t)
	logPath := installFakeGH(t)

	stdout, stderr, err := executeTestCmd(t, "owner/repo", "--no-upstream", "--upstream-remote-name", "parent", "--", "--depth=1")
	if err != nil {
		t.Fatal(err)
	}

	local := filepath.Join(home, "git", "github.com", "owner", "repo")
	wantCloneArgs := []string{
		"repo",
		"clone",
		"owner/repo",
		local,
		"--no-upstream",
		"--upstream-remote-name",
		"parent",
		"--",
		"--depth=1",
	}
	gotBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	gotCloneArgs := strings.Split(strings.TrimSuffix(string(gotBytes), "\n"), "\n")
	if diff := cmp.Diff(wantCloneArgs, gotCloneArgs); diff != "" {
		t.Fatalf("clone args mismatch (-want +got):\n%s", diff)
	}
	if stdout != local+"\n" {
		t.Fatalf("stdout = %q, want %q", stdout, local+"\n")
	}
	if stderr != "clone stdout\nclone stderr\n" {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestCmdCloneFailureDoesNotPrintDirectory(t *testing.T) {
	setTestHome(t)
	installFakeGH(t)
	t.Setenv("GH_CD_FAKE_EXIT", "7")

	stdout, stderr, err := executeTestCmd(t, "owner/repo")
	if err == nil {
		t.Fatal("expected error")
	}
	var exitErr interface{ ExitCode() int }
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
		t.Fatalf("err = %v, want exit code 7", err)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if stderr != "clone stdout\nclone stderr\n" {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestCmdInitZsh(t *testing.T) {
	stdout, stderr, err := executeTestCmd(t, "init", "zsh")
	if err != nil {
		t.Fatal(err)
	}
	if stdout != zshInit {
		t.Fatalf("stdout = %q, want %q", stdout, zshInit)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestCmdInitZshWrapGH(t *testing.T) {
	stdout, stderr, err := executeTestCmd(t, "init", "zsh", "--wrap-gh")
	if err != nil {
		t.Fatal(err)
	}
	if stdout != zshWrapGHInit {
		t.Fatalf("stdout = %q, want %q", stdout, zshWrapGHInit)
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}
