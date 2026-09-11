#!/bin/sh

printf '%s\n' "$*" >> "$GH_CD_DISPATCH_LOG"
case "$*" in
  "cd --help") printf 'combined help\n' ;;
  "cd init zsh") printf 'shell init\n' ;;
  "cd path owner/repo") printf '%s\n' "$GH_CD_DISPATCH_DESTINATION" ;;
  "cd owner/repo")
    printf 'live stdout\n'
    printf 'live stderr\n' >&2
    printf 'cd\n%s\n' "$GH_CD_DISPATCH_DESTINATION" >&3
    ;;
  "cd broken")
    printf 'clone failed\n' >&2
    exit 7
    ;;
  "cd slow")
    printf 'clone stdout progress\n'
    printf 'clone stderr progress\n' >&2
    sleep 1
    printf 'cd\n%s\n' "$GH_CD_DISPATCH_DESTINATION" >&3
    ;;
esac
