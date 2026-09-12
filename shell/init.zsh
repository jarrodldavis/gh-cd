gh() {
  if [[ "${1-}" != "cd" ]]; then
    command gh "$@"
    return
  fi

  local saved_stdout action result dir

  # Save the caller's stdout on a dynamically allocated descriptor. Inside the
  # command substitution, fd 3 points to the captured action channel while fd 1
  # is restored to the caller's stdout so normal output remains live.
  exec {saved_stdout}>&1 || return
  action="$(GH_CD_SHELL_FD=3 command gh "$@" 3>&1 1>&$saved_stdout)"
  result=$?
  exec {saved_stdout}>&-

  (( result == 0 )) || return $result
  [[ -z "$action" ]] && return
  if [[ "$action" != cd$'\n'* ]]; then
    print -u2 -- "gh cd: invalid shell action"
    return 1
  fi
  dir="${action#*$'\n'}"
  if [[ -z "$dir" || "$dir" == *$'\n'* ]]; then
    print -u2 -- "gh cd: invalid directory action"
    return 1
  fi
  builtin cd -- "$dir"
}
