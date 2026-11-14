#!/usr/bin/env bash
set -euo pipefail
umask 077
[[ $# -ge 2 && $2 =~ ^[0-9a-f]{32}$ ]] || exit 125
action=$1
token=$2
shift 2
root=${BACKUP_JOB_ROOT:-/run/pgbackrest/jobs}
proc=${BACKUP_PROC_ROOT:-/proc}
dir=$root/$token
mkdir -p "$root"
if [[ $action == prepare ]]; then
 [[ $# == 0 ]] || exit 125
 mkdir "$dir"
 exec 9>"$dir/lock"
 flock 9
 [[ ! -e $dir/state ]] || exit 125
 printf 'prepared\n' >"$dir/state"
 exit 0
fi
if [[ $action == cancel ]]; then
 mkdir -p "$dir"
fi
[[ -d $dir && ! -L $dir ]] || exit 125
exec 9>"$dir/lock"
flock 9

group_present() {
 local file stat entry
 local -a fields
 for file in "$proc"/[0-9]*/stat; do
  [[ -r $file ]] || continue
  if ! IFS= read -r stat <"$file" 2>/dev/null; then continue; fi
  read -r -a fields <<<"${stat##*) }"
  [[ ${fields[2]:-} == "$group" && ${fields[3]:-} == "$group" ]] || continue
  if [[ ! -r ${file%/stat}/environ ]]; then
   [[ ! -d ${file%/stat} ]] || return 2
   continue
  fi
  while IFS= read -r -d '' entry; do
   if [[ $entry == "TRACHUAN_BACKUP_JOB=$token" ]]; then return 0; fi
  done <"${file%/stat}/environ"
 done
 return 1
}

group_alive() {
 local status=0
 group_present || status=$?
 if ((status > 1)); then exit 125; fi
 return "$status"
}

stop_group() {
 [[ -f $dir/group ]] || return 0
 group=$(<"$dir/group")
 [[ $group =~ ^[0-9]+$ && $group -gt 1 ]] || return 125
 if group_alive; then kill -TERM -- "-$group" 2>/dev/null || true; fi
 local deadline=$((SECONDS+5))
 while group_alive; do
  if ((SECONDS >= deadline)); then kill -KILL -- "-$group" 2>/dev/null || true; fi
  sleep 0.05
 done
}

if [[ $action == cancel ]]; then
 [[ $# == 0 ]] || exit 125
 printf 'cancelled\n' >"$dir/state"
 stop_group
 printf 'stopped\n'
 exit 0
fi
[[ $action == run && $# -ge 2 && $1 =~ ^[0-9]+([.][0-9]+)?$ && $(<"$dir/state") == prepared ]] || exit 125
seconds=$1
shift
[[ $1 == pgbackrest || $1 == psql ]] || exit 125
TRACHUAN_BACKUP_JOB=$token setsid timeout --signal=TERM --kill-after=5s "$seconds" "$@" 9>&- &
child=$!
printf '%s\n' "$child" >"$dir/group"
printf 'running\n' >"$dir/state"
group=$child
while ! group_alive; do
 kill -0 "$child" 2>/dev/null || break
 sleep 0.01
done
flock -u 9
finish() {
 code=$?
 trap - EXIT
 flock 9
 stop_group
 wait "$child" 2>/dev/null || true
 printf 'done\n' >"$dir/state"
 exit "$code"
}
trap finish EXIT
trap 'exit 1' HUP INT TERM
wait "$child"
