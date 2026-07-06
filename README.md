# gh-cd

`gh-cd` is a GitHub CLI extension that resolves a repository argument to a
local clone, cloning it first when necessary.

The extension prints the local directory to stdout. It cannot change the parent
shell process by itself, so the recommended Zsh integration is a small function
that runs `cd` after the extension succeeds.

## Install

```sh
gh extension install jarrodldavis/gh-cd
```

## Zsh Setup

Add this to `.zshrc` to define a `ghcd` helper:

```zsh
eval "$(gh cd init zsh)"
```

This defines:

```zsh
ghcd() {
  local dir
  dir="$(gh cd "$@")" || return
  builtin cd -- "$dir"
}
```

If you prefer `gh cd <repo>` syntax, use the opt-in wrapper instead:

```zsh
eval "$(gh cd init zsh --wrap-gh)"
```

That defines a `gh()` function that intercepts only `gh cd` and forwards all
other `gh` commands to the real GitHub CLI executable.

## Usage

```sh
ghcd cli/cli
ghcd jarrodldavis/gh-cd
ghcd https://github.com/cli/cli
ghcd git@github.com:cli/cli.git
```

With `--wrap-gh`, use the same arguments through `gh cd`:

```sh
gh cd cli/cli
```

Repositories are cloned under:

```text
~/git/<host>/<owner>/<repo>
```

Pass additional `git clone` flags after `--`:

```sh
ghcd cli/cli -- --depth=1
```

Clone options supported by `gh repo clone` can be passed before `--`:

```sh
ghcd cli/cli --no-upstream
ghcd cli/cli --upstream-remote-name parent
```
