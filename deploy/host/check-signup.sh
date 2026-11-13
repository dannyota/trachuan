#!/usr/bin/env bash
# Read the same non-secret env file as Podman without executing its contents.
set -euo pipefail
file=${1:?usage: check-signup.sh server.env}
fail() { echo 'signup-check: configure SERVER_SIGNUP_EMAILS with valid email entries on the host' >&2; exit 1; }
[[ -f $file && ! -L $file ]] || fail
found=false
while IFS= read -r line || [[ -n $line ]]; do
 case $line in
  SERVER_SIGNUP_EMAILS=*)
   [[ $found == false ]] || fail
   found=true
   emails=${line#*=}
   [[ -n $emails && $emails != ,* && $emails != *, && $emails != *,,* ]] || fail
   IFS=',' read -r -a entries <<< "$emails"
   for email in "${entries[@]}"; do
    [[ $email =~ ^[^[:space:]@,]+@[^[:space:]@,.]+(\.[^[:space:]@,.]+)+$ ]] || fail
   done
   ;;
 esac
done <"$file"
[[ $found == true ]] || fail
