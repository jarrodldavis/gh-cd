package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/spf13/cobra"
	"gopkg.in/h2non/gock.v1"
)

func executeTestCmd(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	return executeCommand(t, cmd(), args...)
}

func executeCommand(t *testing.T, cmd *cobra.Command, args ...string) (string, string, error) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
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
	_, _, err := executeTestCmd(t, "path")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCmdWithoutShellIntegrationErrors(t *testing.T) {
	stdout, _, err := executeTestCmd(t, "owner/repo")
	if err == nil {
		t.Fatal("expected error")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(err.Error(), "requires shell integration") {
		t.Fatalf("error = %q, want shell integration guidance", err)
	}
}

func TestCmdWritesShellAction(t *testing.T) {
	home := setTestHome(t)
	wantPath := filepath.Join(home, "git", "github.com", "owner", "repo")
	if err := os.MkdirAll(wantPath, 0o755); err != nil {
		t.Fatal(err)
	}
	runTestGit(t, wantPath, "init", "-q")

	var action bytes.Buffer
	stdout, _, err := executeCommand(t, cmdWithAction(&action), "owner/repo")
	if err != nil {
		t.Fatal(err)
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if want := "cd\n" + wantPath + "\n"; action.String() != want {
		t.Fatalf("action = %q, want %q", action.String(), want)
	}
}

func TestCmdDisambiguatesSubcommandNamesAsRepositories(t *testing.T) {
	for _, name := range []string{"init", "path"} {
		t.Run(name, func(t *testing.T) {
			home := setTestHome(t)
			t.Setenv("GH_TOKEN", "test-token")
			defer gock.Off()
			gock.New("https://api.github.com/").
				Get("/user").
				Reply(200).
				JSON(map[string]string{"login": "owner"})

			wantPath := filepath.Join(home, "git", "github.com", "owner", name)
			if err := os.MkdirAll(wantPath, 0o755); err != nil {
				t.Fatal(err)
			}
			runTestGit(t, wantPath, "init", "-q")

			var action bytes.Buffer
			_, _, err := executeCommand(t, cmdWithAction(&action), "--", name)
			if err != nil {
				t.Fatal(err)
			}
			if want := "cd\n" + wantPath + "\n"; action.String() != want {
				t.Fatalf("action = %q, want %q", action.String(), want)
			}
		})
	}
}

func TestCmdDisambiguatedRepositoryForwardsCloneOptions(t *testing.T) {
	home := setTestHome(t)
	logPath := installFakeGH(t)
	t.Setenv("GH_TOKEN", "test-token")
	defer gock.Off()
	gock.New("https://api.github.com/").
		Get("/user").
		Reply(200).
		JSON(map[string]string{"login": "owner"})

	var action bytes.Buffer
	_, _, err := executeCommand(t, cmdWithAction(&action), "--no-upstream", "--", "path", "--", "--depth=1")
	if err != nil {
		t.Fatal(err)
	}

	local := filepath.Join(home, "git", "github.com", "owner", "path")
	wantCloneArgs := []string{
		"repo",
		"clone",
		"owner/path",
		local,
		"--no-upstream",
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
	if want := "cd\n" + local + "\n"; action.String() != want {
		t.Fatalf("action = %q, want %q", action.String(), want)
	}
}

func TestCmdRejectsNewlineInLocalPath(t *testing.T) {
	home := setTestHome(t)
	stdout, _, err := executeTestCmd(t, "path", "https://github.com/owner/repo%0A", "--mkdir")
	if err == nil {
		t.Fatal("expected error")
	}
	if stdout != "" {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(err.Error(), "local path contains a newline") {
		t.Fatalf("error = %q, want newline error", err)
	}
	local := filepath.Join(home, "git", "github.com", "owner", "repo\n")
	if _, err := os.Stat(local); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("newline path was created: %v", err)
	}
}

func TestCmdHelpCombinesRepositoryUsageAndSubcommands(t *testing.T) {
	stdout, stderr, err := executeTestCmd(t, "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"gh cd [--] <repository> [-- <gitflags>...]",
		"--mkdir",
		"--no-upstream",
		"--upstream-remote-name",
		"init",
		"path",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help does not contain %q:\n%s", want, stdout)
		}
	}
	if stderr != "" {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
}

func TestCmdRejectsCloneArgumentsWithoutDash(t *testing.T) {
	for _, args := range [][]string{
		{"owner/repo", "--depth=1"},
		{"path", "owner/repo", "--depth=1"},
		{"owner/repo", "extra"},
		{"path", "owner/repo", "extra"},
	} {
		_, _, err := executeTestCmd(t, args...)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "pass git clone flags after '--'") {
			t.Fatalf("error = %q, want clone flag separator guidance", err)
		}
	}
}

func TestCmdPreservesAuthenticationError(t *testing.T) {
	t.Setenv("GH_TOKEN", "invalid-token")
	defer gock.Off()

	gock.New("https://api.github.com/").
		Get("/user").
		Reply(401).
		JSON(map[string]string{"message": "Bad credentials"})

	_, _, err := executeTestCmd(t, "path", "features")
	if err == nil {
		t.Fatal("expected error")
	}
	want := "cannot cd: failed to determine repository owner"
	if !strings.Contains(err.Error(), want) || !strings.Contains(err.Error(), "Bad credentials") {
		t.Fatalf("error = %q, want repository owner and authentication details", err)
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

	stdout, stderr, err := executeTestCmd(t, "path", "owner/repo")
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

	stdout, _, err := executeTestCmd(t, "path", "owner/repo")
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

	stdout, stderr, err := executeTestCmd(t, "path", "owner/repo", "--no-upstream", "--upstream-remote-name", "parent", "--", "--depth=1")
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

func TestPathCmdUsesRepositoryFlagsBeforeSubcommand(t *testing.T) {
	home := setTestHome(t)
	logPath := installFakeGH(t)

	stdout, _, err := executeTestCmd(t,
		"--no-upstream",
		"--upstream-remote-name", "parent",
		"path", "owner/repo",
		"--", "--depth=1",
	)
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
}

func TestCmdCloneFailureDoesNotPrintDirectory(t *testing.T) {
	setTestHome(t)
	installFakeGH(t)
	t.Setenv("GH_CD_FAKE_EXIT", "7")

	stdout, stderr, err := executeTestCmd(t, "path", "owner/repo")
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

func TestCmdMkdirInitializesRepository(t *testing.T) {
	home := setTestHome(t)
	logPath := installFakeGH(t)

	stdout, stderr, err := executeTestCmd(t, "path", "owner/repo", "--mkdir")
	if err != nil {
		t.Fatalf("err = %v, stderr = %q", err, stderr)
	}

	local := filepath.Join(home, "git", "github.com", "owner", "repo")
	if stdout != local+"\n" {
		t.Fatalf("stdout = %q, want %q", stdout, local+"\n")
	}
	if !strings.Contains(stderr, "initialized empty repository: "+local+"\n") {
		t.Fatalf("stderr = %q, want initialization message", stderr)
	}
	gitDir := filepath.Join(local, ".git")
	if info, err := os.Stat(gitDir); err != nil || !info.IsDir() {
		t.Fatalf("git directory %q: info=%v err=%v", gitDir, info, err)
	}
	if _, err := os.Stat(logPath); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			t.Fatalf("gh was invoked; clone arguments were written to %s", logPath)
		}
		t.Fatalf("stat clone argument log: %v", err)
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

func TestZshInitDispatchesExtensionCommandsAndRepositories(t *testing.T) {
	if _, err := exec.LookPath("zsh"); err != nil {
		t.Skip("zsh is not installed")
	}

	dir := t.TempDir()
	destination := filepath.Join(dir, "repository")
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "calls")
	ghPath := filepath.Join(dir, "gh")
	fakeGH, err := os.ReadFile("testdata/fake-gh.sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ghPath, fakeGH, 0o755); err != nil {
		t.Fatal(err)
	}

	integration, err := os.ReadFile("testdata/integration.zsh")
	if err != nil {
		t.Fatal(err)
	}
	script := zshInit + "\n" + string(integration)
	command := exec.Command("zsh", "-c", script)
	command.Env = append(os.Environ(),
		"PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"GH_CD_DISPATCH_LOG="+logPath,
		"GH_CD_DISPATCH_DESTINATION="+destination,
	)
	var integrationStdout bytes.Buffer
	var integrationStderr bytes.Buffer
	command.Stdout = &integrationStdout
	command.Stderr = &integrationStderr
	if err := command.Run(); err != nil {
		t.Fatalf("zsh integration failed: %v\nstdout:\n%s\nstderr:\n%s", err, integrationStdout.String(), integrationStderr.String())
	}
	wantStdout := "combined piped help\nshell init\n" + destination + "\npath_unchanged=yes\nlive stdout\n" + destination + "\nfailure=7 unchanged=yes"
	if got := strings.TrimSpace(integrationStdout.String()); got != wantStdout {
		t.Fatalf("stdout = %q, want %q", got, wantStdout)
	}
	wantStderr := "live stderr\nclone failed"
	if got := strings.TrimSpace(integrationStderr.String()); got != wantStderr {
		t.Fatalf("stderr = %q, want %q", got, wantStderr)
	}

	calls, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	wantCalls := "cd --help\ncd init zsh\ncd path owner/repo\ncd owner/repo\ncd broken\n"
	if string(calls) != wantCalls {
		t.Fatalf("gh calls = %q, want %q", calls, wantCalls)
	}

	liveOutput, err := os.ReadFile("testdata/live-output.zsh")
	if err != nil {
		t.Fatal(err)
	}
	liveCommand := exec.Command("zsh", "-c", zshInit+"\n"+string(liveOutput))
	liveCommand.Env = command.Env
	stdout, err := liveCommand.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := liveCommand.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := liveCommand.Start(); err != nil {
		t.Fatal(err)
	}
	if line, err := bufio.NewReader(stdout).ReadString('\n'); err != nil || line != "clone stdout progress\n" {
		t.Fatalf("live stdout = %q, err = %v", line, err)
	}
	if line, err := bufio.NewReader(stderr).ReadString('\n'); err != nil || line != "clone stderr progress\n" {
		t.Fatalf("live stderr = %q, err = %v", line, err)
	}
	done := make(chan error, 1)
	go func() { done <- liveCommand.Wait() }()
	select {
	case err := <-done:
		t.Fatalf("command completed before progress could be observed: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestGHLauncherPreservesActionDescriptor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell action descriptors are supported only on Unix")
	}
	ghPath, err := exec.LookPath("gh")
	if err != nil {
		t.Skip("gh is not installed")
	}

	dataDir := t.TempDir()
	extensionDir := filepath.Join(dataDir, "gh", "extensions", "gh-fd-probe")
	if err := os.MkdirAll(extensionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	probe, err := os.ReadFile("testdata/fd-probe.sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extensionDir, "gh-fd-probe"), probe, 0o755); err != nil {
		t.Fatal(err)
	}

	actionReader, actionWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer actionReader.Close()

	command := exec.Command(ghPath, "fd-probe")
	command.Env = append(os.Environ(),
		"XDG_DATA_HOME="+dataDir,
		"GH_CONFIG_DIR="+filepath.Join(dataDir, "config"),
		"GH_NO_EXTENSION_UPDATE_NOTIFIER=1",
	)
	command.ExtraFiles = []*os.File{actionWriter}
	if output, err := command.CombinedOutput(); err != nil {
		actionWriter.Close()
		t.Fatalf("gh extension invocation failed: %v\n%s", err, output)
	}
	if err := actionWriter.Close(); err != nil {
		t.Fatal(err)
	}

	action, err := io.ReadAll(actionReader)
	if err != nil {
		t.Fatal(err)
	}
	if string(action) != "inherited\n" {
		t.Fatalf("action = %q, want descriptor payload", action)
	}
}
