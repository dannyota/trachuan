#!/usr/bin/env bash
set -euo pipefail
packages=(podman container-selinux firewalld policycoreutils-python-utils shadow-utils python3 acl tar gzip logrotate)
if ! rpm -q "${packages[@]}" >/dev/null 2>&1; then
 dnf --disablerepo='*' --enablerepo=ol9_baseos_latest --enablerepo=ol9_appstream install -y "${packages[@]}"
fi
if ! rpm -q python39-oci-sdk >/dev/null 2>&1; then
 dnf --disablerepo='*' --enablerepo=ol9_oci_included install -y python39-oci-sdk
fi
