# gh-cd

`gh-cd` is a GitHub CLI extension that changes to a repository's local clone,
cloning it first when necessary.

## Install

```sh
gh extension install jarrodldavis/gh-cd
```

## Zsh Setup

Add this to `.zshrc` to define `gh cd`:

```zsh
eval "$(gh cd init zsh)"
```

This defines a `gh()` function that forwards every invocation to the real GitHub
CLI executable. The extension sends a private action to the wrapper when a
successful `gh cd <repository>` invocation should change directories. Normal
standard output and standard error remain connected to the terminal, so clone
progress and help output are displayed as usual.

## Usage

```sh
gh cd cli/cli
gh cd jarrodldavis/gh-cd
gh cd https://github.com/cli/cli
gh cd git@github.com:cli/cli.git
```

Repositories are cloned under:

```text
~/git/<host>/<owner>/<repo>
```

Whenever a repository is resolved, `gh-cd` also configures each supported remote
to fetch code-review heads. A GitHub `origin` receives this local, idempotent
fetch refspec:

```text
+refs/pull/*/head:refs/remotes/origin/pr/*
```

GitLab remotes similarly receive:

```text
+refs/merge-requests/*/head:refs/remotes/origin/mr/*
```

This applies to remotes created during cloning (including a GitHub fork's
`upstream`) and supported remotes added later. Other hosts are not changed.
Afterward, ordinary `git fetch` makes review tips available as refs such as
`origin/pr/123` on GitHub or `origin/mr/123` on GitLab.

Pass additional `git clone` flags after `--`:

```sh
gh cd cli/cli -- --depth=1
```

Clone options supported by `gh repo clone` can be passed before `--`:

```sh
gh cd cli/cli --no-upstream
gh cd cli/cli --upstream-remote-name parent
```

To initialize an empty local Git repository without checking or cloning the
requested remote, pass `--mkdir`:

```sh
gh cd owner/new-repository --mkdir
```

If the local directory already exists, it is used as usual.
