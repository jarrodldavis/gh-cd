set -u
gh
gh cd --help | sed 's/combined/combined piped/'
gh cd init zsh
before="$PWD"
gh cd path owner/repo
print -r -- "path_unchanged=$([[ "$PWD" == "$before" ]] && print yes || print no)"
gh cd owner/repo
pwd
before="$PWD"
gh cd broken
result=$?
print -r -- "failure=$result unchanged=$([[ "$PWD" == "$before" ]] && print yes || print no)"
