package main

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/go-gh/v2"
	"github.com/spf13/cobra"
)

func cmd() *cobra.Command {
	if os.Getenv("GH_CD_SHELL_FD") != "3" {
		return cmdWithAction(nil)
	}
	// The Zsh wrapper opens fd 3 without FD_CLOEXEC. On Unix, GitHub CLI's
	// os/exec-based extension launcher leaves inherited descriptors >= 3
	// open, so this channel survives the intermediate `gh` process. Keep
	// TestGHLauncherPreservesActionDescriptor as coverage for that contract.
	return cmdWithAction(os.NewFile(3, "gh-cd-shell-action"))
}

func cmdWithAction(shellAction io.Writer) *cobra.Command {
	options := &cdOptions{}
	cmd := &cobra.Command{
		DisableFlagsInUseLine: true,
		Use:                   "cd [--] <repository> [-- <gitflags>...]",
		Args:                  repositoryArgs,
		Short:                 "Change to a local clone of a repository",
		Long: heredoc.Docf(`
			Change to a local clone of a repository, cloning it first when necessary.
			Shell integration from %[1]sgh cd init zsh%[1]s is required because an external
			command cannot change its parent shell's working directory.

			Use %[1]sgh cd path%[1]s to print the local path without changing directories.
			Pass additional %[1]sgit clone%[1]s flags by listing them after "--".
		`, "`"),
		Annotations: map[string]string{
			cobra.CommandDisplayNameAnnotation: "gh cd",
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if shellAction == nil {
				return errors.New("gh cd requires shell integration; run 'eval \"$(gh cd init zsh)\"'")
			}
			local, err := resolveRepository(cmd, args, options)
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintf(shellAction, "cd\n%s\n", local); err != nil {
				return fmt.Errorf("cannot cd: failed to send shell action: %w", err)
			}
			return nil
		},
	}
	configureRepositoryFlags(cmd, options, "help for gh cd")
	cmd.AddCommand(pathCmd(options), initCmd())
	return cmd
}

type cdOptions struct {
	mkdir              bool
	noUpstream         bool
	upstreamRemoteName string
}

func pathCmd(options *cdOptions) *cobra.Command {
	cmd := &cobra.Command{
		DisableFlagsInUseLine: true,

		Use:   "path <repository> [-- <gitflags>...]",
		Args:  repositoryArgs,
		Short: "Print the path to a local clone, creating the clone if necessary",
		Long: heredoc.Docf(`
			Print the path to a local clone, creating the clone if necessary.
			Use %[1]sgh cd init zsh%[1]s to define %[1]sgh cd%[1]s as a Zsh function that changes directories.
			Pass additional %[1]sgit clone%[1]s flags by listing them after "--".
		`, "`"),
		RunE: func(cmd *cobra.Command, args []string) error {
			local, err := resolveRepository(cmd, args, options)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), local)
			return nil
		},
	}
	configureRepositoryFlags(cmd, options, "help for gh cd path")
	return cmd
}

func repositoryArgs(cmd *cobra.Command, args []string) error {
	dash := cmd.Flags().ArgsLenAtDash()
	if len(args) == 0 {
		return errors.New("cannot cd: repository argument required")
	}
	if dash == 0 {
		if len(args) == 1 {
			return nil
		}
		if args[1] != "--" {
			return errors.New("cannot cd: too many arguments")
		}
		return nil
	}
	if dash >= 0 && dash != 1 {
		return errors.New("cannot cd: too many arguments")
	}
	if dash < 0 && len(args) > 1 {
		return errors.New("cannot cd: too many arguments")
	}
	return nil
}

func configureRepositoryFlags(cmd *cobra.Command, options *cdOptions, help string) {
	cmd.Flags().BoolP("help", "h", false, help)
	cmd.Flags().BoolVar(&options.mkdir, "mkdir", false, "initialize an empty repository instead of cloning")
	cmd.Flags().BoolVar(&options.noUpstream, "no-upstream", false, "do not add an upstream remote when cloning a fork")
	cmd.Flags().StringVarP(&options.upstreamRemoteName, "upstream-remote-name", "u", "", "upstream remote name when cloning a fork")
}

func resolveRepository(cmd *cobra.Command, args []string, options *cdOptions) (string, error) {
	parsed, err := parse(args[0])
	if err != nil {
		return "", fmt.Errorf("cannot cd: %w", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot cd: failed to find home directory: %w", err)
	}

	localSegments := make([]string, 0, len(parsed.local)+2)
	localSegments = append(localSegments, home, "git")
	localSegments = append(localSegments, parsed.local...)
	local := filepath.Join(localSegments...)
	if strings.ContainsAny(local, "\r\n") {
		return "", errors.New("cannot cd: local path contains a newline")
	}

	if info, err := os.Stat(local); errors.Is(err, os.ErrNotExist) && options.mkdir {
		if err := initRepository(cmd.Context(), local); err != nil {
			return "", fmt.Errorf("cannot cd: failed to initialize repository: %w", err)
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "initialized empty repository: %s\n", local)
	} else if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
			return "", fmt.Errorf("cannot cd: failed to create parent directory: %w", err)
		}
		remote := parsed.remote.String()
		ghargs := []string{"repo", "clone", remote, local}
		ghargs = append(ghargs, cloneOptions(cmd, options)...)
		dash := cmd.Flags().ArgsLenAtDash()
		if dash == 1 || (dash == 0 && len(args) > 1) {
			ghargs = append(ghargs, "--")
		}
		if dash == 0 {
			ghargs = append(ghargs, args[2:]...)
		} else {
			ghargs = append(ghargs, args[1:]...)
		}

		if err := runClone(cmd.Context(), cmd.ErrOrStderr(), ghargs...); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", fmt.Errorf("cannot cd: failed to stat: %w", err)
	} else if !info.IsDir() {
		return "", fmt.Errorf("cannot cd: local path exists but is not a directory: %s", local)
	} else {
		fmt.Fprintf(cmd.ErrOrStderr(), "using existing clone: %s\n", local)
	}

	if err := addReviewRefspecs(cmd.Context(), local); err != nil {
		return "", fmt.Errorf("cannot cd: failed to configure code review refspecs: %w", err)
	}
	return local, nil
}

func addReviewRefspecs(ctx context.Context, repo string) error {
	remotes, err := localRemoteNames(ctx, repo)
	if err != nil {
		return err
	}

	for _, remote := range remotes {
		urls, err := gitConfigValues(ctx, repo, fmt.Sprintf("remote.%s.url", remote))
		if err != nil {
			return err
		}

		key := fmt.Sprintf("remote.%s.fetch", remote)
		fetches, err := gitConfigValues(ctx, repo, key)
		if err != nil {
			return err
		}

		var refspec string
		if len(urls) > 0 {
			refspec = reviewRefspec(remote, urls[0])
		}

		for _, managed := range managedReviewRefspecs(remote) {
			if managed == refspec || !contains(fetches, managed) {
				continue
			}
			if _, err := gitOutput(ctx, repo, "config", "--local", "--fixed-value", "--unset-all", key, managed); err != nil {
				return err
			}
		}

		if refspec == "" || contains(fetches, refspec) {
			continue
		}
		if _, err := gitOutput(ctx, repo, "config", "--local", "--add", key, refspec); err != nil {
			return err
		}
	}

	return nil
}

func localRemoteNames(ctx context.Context, repo string) ([]string, error) {
	output, err := gitOutput(ctx, repo, "config", "--local", "--name-only", "--get-regexp", `^remote\..*\.url$`)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, err
	}

	var remotes []string
	seen := make(map[string]struct{})
	for _, key := range outputLines(output) {
		remote := strings.TrimSuffix(strings.TrimPrefix(key, "remote."), ".url")
		if _, ok := seen[remote]; ok {
			continue
		}
		seen[remote] = struct{}{}
		remotes = append(remotes, remote)
	}
	return remotes, nil
}

func managedReviewRefspecs(remote string) []string {
	return []string{
		fmt.Sprintf("+refs/pull/*/head:refs/remotes/%s/pr/*", remote),
		fmt.Sprintf("+refs/merge-requests/*/head:refs/remotes/%s/mr/*", remote),
	}
}

func reviewRefspec(remote, rawURL string) string {
	url := strings.ToLower(rawURL)
	if strings.HasPrefix(url, "git@github.com:") ||
		strings.HasPrefix(url, "ssh://git@github.com/") ||
		strings.HasPrefix(url, "https://github.com/") {
		return managedReviewRefspecs(remote)[0]
	}
	if strings.HasPrefix(url, "git@gitlab.com:") ||
		strings.HasPrefix(url, "ssh://git@gitlab.com/") ||
		strings.HasPrefix(url, "https://gitlab.com/") {
		return managedReviewRefspecs(remote)[1]
	}
	return ""
}

func gitConfigValues(ctx context.Context, repo, key string) ([]string, error) {
	output, err := gitOutput(ctx, repo, "config", "--local", "--get-all", key)
	if err == nil {
		return outputLines(output), nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return nil, nil
	}
	return nil, err
}

func gitOutput(ctx context.Context, repo string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, detail)
	}
	return stdout.String(), nil
}

func outputLines(output string) []string {
	return strings.Fields(strings.TrimSpace(output))
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func cloneOptions(cmd *cobra.Command, options *cdOptions) []string {
	var args []string
	if options.noUpstream {
		args = append(args, "--no-upstream")
	}
	if flagChanged(cmd, "upstream-remote-name") {
		args = append(args, "--upstream-remote-name", options.upstreamRemoteName)
	}
	return args
}

func flagChanged(cmd *cobra.Command, name string) bool {
	for current := cmd; current != nil; current = current.Parent() {
		if current.Flags().Changed(name) {
			return true
		}
	}
	return false
}

func runClone(ctx context.Context, output io.Writer, args ...string) error {
	ghPath, err := gh.Path()
	if err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, ghPath, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = output
	cmd.Stderr = output
	return cmd.Run()
}

func initRepository(ctx context.Context, repo string) error {
	if err := os.MkdirAll(repo, 0o755); err != nil {
		return err
	}
	_, err := gitOutput(ctx, repo, "init", "-q")
	return err
}

func initCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                   "init zsh",
		DisableFlagsInUseLine: true,
		Args:                  cobra.ExactArgs(1),
		Short:                 "Print shell integration for gh-cd",
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] != "zsh" {
				return fmt.Errorf("unsupported shell %q", args[0])
			}
			fmt.Fprint(cmd.OutOrStdout(), zshInit)
			return nil
		},
	}
	return cmd
}

//go:embed shell/init.zsh
var zshInit string
