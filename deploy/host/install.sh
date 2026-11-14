#!/usr/bin/env bash
set -euo pipefail
cd /
fail() { echo "install: $*" >&2; exit 1; }
[[ $(id -u) == 0 ]] || fail 'run with sudo from the opc account'
[[ $# == 1 ]] || fail 'usage: install.sh bundle-directory'
bundle=$(realpath "$1")
[[ -d $bundle/quadlet && -f $bundle/images.env ]] || fail 'incomplete host bundle'
mountpoint -q /srv/trachuan/data || fail 'mount /srv/trachuan/data first; follow docs/runbooks/data-volume.md'
# shellcheck source=/dev/null
source /etc/os-release
[[ $ID == ol && $VERSION_ID == 9.8 && $(uname -m) == aarch64 ]] || fail 'requires Oracle Linux 9.8 arm64'
[[ $(stat -fc %T /sys/fs/cgroup) == cgroup2fs ]] || fail 'cgroup v2 is required'

dnf --disablerepo='*' --enablerepo=ol9_baseos_latest --enablerepo=ol9_appstream \
 install -y podman container-selinux firewalld policycoreutils-python-utils shadow-utils python3
version=$(podman --version | awk '{print $3}')
[[ $(printf '%s\n' 4.4 "$version" | sort -V | head -1) == 4.4 ]] || fail 'Podman >= 4.4 is required'
[[ $(getenforce) == Enforcing ]] || fail 'SELinux must be enforcing'

if ! id trachuan >/dev/null 2>&1; then
 useradd --create-home --shell /bin/bash trachuan
fi
[[ $(getent passwd trachuan | cut -d: -f6) == /home/trachuan ]] || fail 'unexpected trachuan home'
for kind in uid gid; do
 file=/etc/sub${kind}
 if ! awk -F: '$1 == "trachuan" && $3 >= 65536 {found=1} END {exit !found}' "$file"; then
  if awk -F: '$1 == "trachuan" {found=1} END {exit !found}' "$file"; then
   fail "existing subordinate $kind range is too small; owner recovery required"
  fi
  start=$(awk -F: 'BEGIN {nextid=100000} NF==3 {end=$2+$3; if (end>nextid) nextid=end} END {print nextid}' "$file")
  usermod "--add-sub${kind}s" "$start-$((start + 65535))" trachuan
 fi
done
install -d -m 0755 /etc/systemd/system/user@.service.d
printf '[Service]\nDelegate=cpu cpuset io memory pids\n' >/etc/systemd/system/user@.service.d/delegate.conf
systemctl daemon-reload
loginctl enable-linger trachuan
uid=$(id -u trachuan)
systemctl start "user@$uid.service"
as_user() {
 runuser -u trachuan -- env XDG_RUNTIME_DIR="/run/user/$uid" \
  DBUS_SESSION_BUS_ADDRESS="unix:path=/run/user/$uid/bus" "$@"
}
[[ $(as_user podman info --format '{{.Host.CgroupsVersion}}') == v2 ]] || fail 'rootless Podman requires cgroup v2'
controllers=$(<"/sys/fs/cgroup/user.slice/user-$uid.slice/user@$uid.service/cgroup.controllers")
for controller in cpu cpuset io memory pids; do
 [[ " $controllers " == *" $controller "* ]] || fail "cgroup controller $controller is not delegated; restart user@$uid.service"
done

printf '%s\n' 'net.ipv4.ip_unprivileged_port_start=80' >/etc/sysctl.d/90-trachuan.conf
sysctl -p /etc/sysctl.d/90-trachuan.conf >/dev/null
systemctl enable --now firewalld
for service in http https; do
 firewall-cmd --permanent --query-service="$service" >/dev/null || firewall-cmd --permanent --add-service="$service"
done
firewall-cmd --permanent --query-port=443/udp >/dev/null || firewall-cmd --permanent --add-port=443/udp
firewall-cmd --reload

install -d -o root -g root -m 0755 /srv/trachuan
for dir in /srv/trachuan/data/postgres /srv/trachuan/data/pgbackrest; do
 [[ -d $dir ]] || install -d -o trachuan -g trachuan -m 0700 "$dir"
done
install -d -o trachuan -g trachuan -m 0755 /srv/trachuan/host /srv/trachuan/host/quadlet
install -d -o trachuan -g trachuan -m 0755 /srv/trachuan/models /srv/trachuan/models/vi_v2-bundle
for script in bootstrap.sh grant-corpus.sh wait-healthy.sh render-units.sh \
 postgres-admin.sh wait-postgres.sh check-postgres-auth.sh repair-postgres-auth.sh check-signup.sh client-address-log.sh; do
 install -o trachuan -g trachuan -m 0755 "$bundle/$script" "/srv/trachuan/host/$script"
done
for config in Caddyfile client-address-check.caddy; do
 install -o trachuan -g trachuan -m 0644 "$bundle/$config" "/srv/trachuan/host/$config"
done
for config in images.env server.env web.env; do
 if [[ ! -e /srv/trachuan/host/$config ]]; then
  install -o trachuan -g trachuan -m 0644 "$bundle/$config" "/srv/trachuan/host/$config"
 fi
done
rm -f /srv/trachuan/host/quadlet/trachuan.network /srv/trachuan/host/quadlet/trachuan-db.network \
 /srv/trachuan/host/quadlet/trachuan-postgres.volume \
 /home/trachuan/.config/containers/systemd/trachuan-postgres.volume
for template in "$bundle"/quadlet/*; do
 install -o trachuan -g trachuan -m 0644 "$template" "/srv/trachuan/host/quadlet/${template##*/}"
done
if ! semanage fcontext -l -C | awk '$1 == "/srv/trachuan(/.*)?" {found=1} END {exit !found}'; then
 semanage fcontext -a -t container_file_t '/srv/trachuan(/.*)?'
fi
restorecon -RF /srv/trachuan
install -d -o trachuan -g trachuan -m 0755 /home/trachuan/.config /home/trachuan/.config/containers /home/trachuan/.config/containers/systemd
as_user bash /srv/trachuan/host/render-units.sh /srv/trachuan/host/images.env /home/trachuan/.config/containers/systemd
as_user systemctl --user daemon-reload
for unit in trachuan-caddy-data-volume trachuan-caddy-config-volume trachuan-postgres \
 trachuan-migrate trachuan-queryembed trachuan-server trachuan-web trachuan-caddy; do
 [[ $(as_user systemctl --user show --property=LoadState --value "$unit.service") == loaded ]] || fail "Quadlet cannot load $unit.service; upgrade Podman from OL9 repositories"
done
echo 'install: bundle installed; create secrets and provision the model before starting services'
