package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/go-gh/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func cmd() *cobra.Command {
	cmd := &cobra.Command{
		DisableFlagsInUseLine: true,

		Use: "gh cd <repository> [-- <gitflags>...]",
		Args: func(cmd *cobra.Command, args []string) error {
			dash := cmd.Flags().ArgsLenAtDash()
			if len(args) == 0 || dash == 0 {
				return errors.New("cannot cd: repository argument required")
			}
			if dash >= 0 && dash != 1 {
				return errors.New("cannot cd: too many arguments")
			}
			if dash < 0 && len(args) > 1 {
				return errors.New("cannot cd: too many arguments\nSeparate git clone flags with '--'.")
			}
			return nil
		},
		Short: "Print the path to a local clone, creating the clone if necessary",
		Long: heredoc.Docf(`
			Print the path to a local clone, creating the clone if necessary.
			Use %[1]sgh cd init zsh%[1]s to define a Zsh function that changes directories.
			Pass additional %[1]sgit clone%[1]s flags by listing them after "--".
		`, "`"),
		RunE: func(cmd *cobra.Command, args []string) error {
			parsed := parse(args[0])

			if parsed == nil {
				return errors.New("cannot cd: failed to parse repository argument")
			}

			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("cannot cd: failed to find home directory: %w", err)
			}

			localSegments := make([]string, 0, len(parsed.local)+2)
			localSegments = append(localSegments, home, "git")
			localSegments = append(localSegments, parsed.local...)
			local := filepath.Join(localSegments...)

			if info, err := os.Stat(local); errors.Is(err, os.ErrNotExist) {
				if err := os.MkdirAll(filepath.Dir(local), 0o755); err != nil {
					return fmt.Errorf("cannot cd: failed to create parent directory: %w", err)
				}
				remote := parsed.remote.String()
				ghargs := []string{"repo", "clone", remote, local}
				ghargs = append(ghargs, cloneOptions(cmd)...)
				if cmd.Flags().ArgsLenAtDash() == 1 {
					ghargs = append(ghargs, "--")
				}
				ghargs = append(ghargs, args[1:]...)

				if err := runClone(cmd.Context(), cmd.ErrOrStderr(), ghargs...); err != nil {
					return err
				}
			} else if err != nil {
				return fmt.Errorf("cannot cd: failed to stat: %w", err)
			} else if !info.IsDir() {
				return fmt.Errorf("cannot cd: local path exists but is not a directory: %s", local)
			} else {
				fmt.Fprintf(cmd.ErrOrStderr(), "using existing clone: %s\n", local)
			}

			fmt.Fprintln(cmd.OutOrStdout(), local)
			return nil
		},
	}

	cmd.Flags().BoolP("help", "h", false, "help for gh cd")
	cmd.Flags().Bool("no-upstream", false, "do not add an upstream remote when cloning a fork")
	cmd.Flags().StringP("upstream-remote-name", "u", "", "upstream remote name when cloning a fork")

	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		if err == pflag.ErrHelp {
			return err
		}
		return fmt.Errorf("%w\nSeparate git clone flags with '--'.", err)
	})
	cmd.AddCommand(initCmd())

	return cmd
}

func cloneOptions(cmd *cobra.Command) []string {
	var args []string
	flags := cmd.Flags()

	if flags.Changed("no-upstream") {
		value, _ := flags.GetBool("no-upstream")
		if value {
			args = append(args, "--no-upstream")
		}
	}

	if flags.Changed("upstream-remote-name") {
		value, _ := flags.GetString("upstream-remote-name")
		args = append(args, "--upstream-remote-name", value)
	}

	return args
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

func initCmd() *cobra.Command {
	var wrapGH bool
	cmd := &cobra.Command{
		Use:                   "init zsh",
		DisableFlagsInUseLine: true,
		Args:                  cobra.ExactArgs(1),
		Short:                 "Print shell integration for gh-cd",
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] != "zsh" {
				return fmt.Errorf("unsupported shell %q", args[0])
			}
			if wrapGH {
				fmt.Fprint(cmd.OutOrStdout(), zshWrapGHInit)
				return nil
			}
			fmt.Fprint(cmd.OutOrStdout(), zshInit)
			return nil
		},
	}
	cmd.Flags().BoolVar(&wrapGH, "wrap-gh", false, "define a gh function that handles gh cd")
	return cmd
}

const zshInit = `ghcd() {
  local dir
  dir="$(gh cd "$@")" || return
  builtin cd -- "$dir"
}
`

const zshWrapGHInit = `gh() {
  if [[ "$1" == "cd" ]]; then
    shift
    local dir
    dir="$(command gh cd "$@")" || return
    builtin cd -- "$dir"
  else
    command gh "$@"
  fi
}
`
