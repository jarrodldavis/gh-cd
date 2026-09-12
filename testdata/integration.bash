set -u
gh
gh cd --help | sed 's/combined/combined piped/'
gh cd init bash
before="$PWD"
gh cd path owner/repo
printf '%s\n' "path_unchanged=$([[ "$PWD" == "$before" ]] && printf yes || printf no)"
gh cd owner/repo
pwd
before="$PWD"
gh cd broken
result=$?
printf '%s\n' "failure=$result unchanged=$([[ "$PWD" == "$before" ]] && printf yes || printf no)"
