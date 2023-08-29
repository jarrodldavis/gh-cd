package main

import (
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/MakeNowJust/heredoc"
	"github.com/cli/go-gh/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func cmd() *cobra.Command {
	cmd := &cobra.Command{
		DisableFlagsInUseLine: true,

		Use: "gh cd <repository> [-- <ghflags>...]",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 0 {
				return errors.New("cannot cd: repository argument required")
			} else if len(args) > 1 {
				return errors.New("cannot cd: too many arguments")
			} else {
				return nil
			}
		},
		Short: "Change the current directory to a local clone, creating the clone if necessary",
		Long: heredoc.Docf(`
			Change the current directory to a local clone, creating the clone if necessary.
			Pass additional %[1]sgh repo clone%[1]s flags by listing them after "--".
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

			lseg := make([]string, len(parsed.local)+2)
			lseg = append(lseg, home)
			lseg = append(lseg, "git")
			lseg = append(lseg, parsed.local...)
			local := path.Join(lseg...)

			if _, err := os.Stat(local); errors.Is(err, os.ErrNotExist) {
				remote := parsed.remote.String()
				ghargs := make([]string, len(args)+3)
				ghargs = []string{"repo", "clone", remote, local}
				ghargs = append(ghargs, args[1:]...)
				err := gh.ExecInteractive(cmd.Context(), ghargs...)

				if err != nil {
					return err
				}

				// TODO: change process of shell, not current process
				return os.Chdir(local)
			} else if err == nil {
				// TODO: change process of shell, not current process
				return os.Chdir(local)
			} else {
				return fmt.Errorf("cannot cd: failed to stat: %w", err)
			}
		},
	}

	cmd.Flags().BoolP("help", "h", false, "help for gh cd")

	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		if err == pflag.ErrHelp {
			return err
		}
		return fmt.Errorf("%w\nSeparate gh clone flags with '--'.", err)
	})

	return cmd
}
