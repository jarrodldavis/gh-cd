gh() {
  if [[ "${1-}" != "cd" ]]; then
    command gh "$@"
    return
  fi

  local action result dir

  # For the duration of this assignment, fd 4 saves the caller's stdout. Inside
  # the command substitution, fd 3 points to the captured action channel while
  # fd 1 is restored to the caller's stdout so normal output remains live.
  # Scoping fd 4 to the assignment keeps this compatible with Bash 3.2 and
  # restores any descriptor the caller already had open there.
  {
    action="$(GH_CD_SHELL_FD=3 command gh "$@" 3>&1 1>&4)"
    result=$?
  } 4>&1

  (( result == 0 )) || return "$result"
  [[ -z "$action" ]] && return
  if [[ "$action" != cd$'\n'* ]]; then
    printf '%s\n' "gh cd: invalid shell action" >&2
    return 1
  fi
  dir="${action#*$'\n'}"
  if [[ -z "$dir" || "$dir" == *$'\n'* ]]; then
    printf '%s\n' "gh cd: invalid directory action" >&2
    return 1
  fi
  builtin cd -- "$dir"
}
