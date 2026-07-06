package main

import (
	"bytes"
	"errors"
	"os"
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
		"https://github.com/owner/repo.git",
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
