#!/usr/bin/env bash
# Build the preconfigured dockup rootfs tarball for WSL.
#
# Usage: sudo ./scripts/image/build.sh <amd64|arm64> [outdir]
#
# Produces <outdir>/dockup-<arch>.tar.gz and its .sha256 checksum.
# Cross-building arm64 on amd64 needs qemu-user-static (installed on demand).
set -euo pipefail

ARCH="${1:-}"
OUTDIR="${2:-dist/image}"

case "$ARCH" in
  amd64|arm64) ;;
  *) echo "usage: $0 <amd64|arm64> [outdir]" >&2; exit 2 ;;
esac

if [[ $EUID -ne 0 ]]; then
  echo "run as root (chroot is required): sudo $0 $*" >&2
  exit 1
fi

for cmd in debootstrap tar gzip sha256sum curl systemctl; do
  command -v "$cmd" >/dev/null || { echo "missing required command: $cmd" >&2; exit 1; }
done

SUITE=bookworm
MIRROR=http://deb.debian.org/debian
DOCKER_REPO=https://download.docker.com/linux/debian
HOST_ARCH=$(dpkg --print-architecture)
ROOT=$(mktemp -d /tmp/dockup-image-XXXXXX)
BUILD_ID="${BUILD_ID:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"
SOURCE_DATE_EPOCH="${SOURCE_DATE_EPOCH:-0}"

export DEBIAN_FRONTEND=noninteractive

cleanup() {
  for d in sys proc dev; do
    umount -Rf "$ROOT/$d" 2>/dev/null || true
  done
  rm -rf "$ROOT"
}
trap cleanup EXIT

FOREIGN=0
QEMU_ARCH=""
if [[ "$ARCH" != "$HOST_ARCH" ]]; then
  FOREIGN=1
  case "$ARCH" in
    amd64) QEMU_ARCH=x86_64 ;;
    arm64) QEMU_ARCH=aarch64 ;;
  esac
  apt-get update -qq
  apt-get install -y -qq qemu-user-static binfmt-support >/dev/null
  update-binfmts --enable "qemu-$QEMU_ARCH" 2>/dev/null || true
fi

echo "==> debootstrap $SUITE ($ARCH)"
if [[ $FOREIGN -eq 1 ]]; then
  debootstrap --arch="$ARCH" --variant=minbase --foreign "$SUITE" "$ROOT" "$MIRROR"
  cp "/usr/bin/qemu-${QEMU_ARCH}-static" "$ROOT/usr/bin/"
  chroot "$ROOT" /debootstrap/debootstrap --second-stage
else
  debootstrap --arch="$ARCH" --variant=minbase "$SUITE" "$ROOT" "$MIRROR"
fi

mount --rbind /dev "$ROOT/dev"
mount -t proc proc "$ROOT/proc"
mount -t sysfs sysfs "$ROOT/sys"
cp /etc/resolv.conf "$ROOT/etc/resolv.conf"

printf '#!/bin/sh\nexit 101\n' > "$ROOT/usr/sbin/policy-rc.d"
chmod +x "$ROOT/usr/sbin/policy-rc.d"

APT=(apt-get)
if [[ $FOREIGN -eq 1 ]]; then
  APT=(apt-get -o APT::Sandbox::User=root)
fi

echo "==> installing base packages"
chroot "$ROOT" "${APT[@]}" update
chroot "$ROOT" "${APT[@]}" install -y --no-install-recommends \
  ca-certificates curl gnupg procps iproute2

echo "==> installing docker"
install -d -m 0755 "$ROOT/etc/apt/keyrings"
curl -fsSL "$DOCKER_REPO/gpg" -o "$ROOT/etc/apt/keyrings/docker.asc"
chmod 0644 "$ROOT/etc/apt/keyrings/docker.asc"
printf 'deb [arch=%s signed-by=/etc/apt/keyrings/docker.asc] %s %s stable\n' \
  "$ARCH" "$DOCKER_REPO" "$SUITE" > "$ROOT/etc/apt/sources.list.d/docker.list"

chroot "$ROOT" "${APT[@]}" update
chroot "$ROOT" "${APT[@]}" install -y --no-install-recommends \
  docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin socat

echo "==> configuring systemd"
printf '[boot]\nsystemd=true\n' > "$ROOT/etc/wsl.conf"
systemctl --root="$ROOT" enable containerd.service docker.service docker.socket

DOCKER_VERSION=$(chroot "$ROOT" docker --version)
CONTAINERD_VERSION=$(chroot "$ROOT" containerd --version)

cat > "$ROOT/etc/dockup-image" <<EOF
build=$BUILD_ID
arch=$ARCH
suite=$SUITE
docker=$DOCKER_VERSION
containerd=$CONTAINERD_VERSION
EOF

echo "==> scrubbing build state"
chroot "$ROOT" apt-get clean
rm -rf "$ROOT/var/lib/apt/lists/"*
rm -rf "$ROOT/var/cache/apt/archives/"*.deb
rm -rf "$ROOT/var/lib/docker/"* "$ROOT/var/lib/containerd/"*
rm -rf "$ROOT/var/log/"* "$ROOT/tmp/"* "$ROOT/var/tmp/"* "$ROOT/run/"*
rm -rf "$ROOT/root/.bash_history" "$ROOT/root/.cache"
rm -f "$ROOT/etc/hosts" "$ROOT/etc/resolv.conf"
rm -f "$ROOT/etc/ssh/ssh_host_"*
rm -f "$ROOT/var/lib/systemd/random-seed"
rm -f "$ROOT"/usr/bin/qemu-*-static
rm -f "$ROOT/usr/sbin/policy-rc.d"
rm -f "$ROOT/etc/machine-id" "$ROOT/var/lib/dbus/machine-id"
: > "$ROOT/etc/machine-id"
mkdir -p "$ROOT/var/lib/dbus"
ln -sf /etc/machine-id "$ROOT/var/lib/dbus/machine-id"

echo "==> packing"
mkdir -p "$OUTDIR"
NAME="dockup-$ARCH.tar.gz"
TAR="$OUTDIR/$NAME"
( cd "$ROOT" && tar \
    --numeric-owner --xattrs --xattrs-include='*' --acls \
    --sort=name --mtime="@$SOURCE_DATE_EPOCH" --owner=0 --group=0 \
    -cf - . ) | gzip -9 -n > "$TAR"
( cd "$OUTDIR" && sha256sum "$NAME" > "$NAME.sha256" )

echo "built $TAR ($(du -h "$TAR" | cut -f1))"
echo "sha256 $(cut -d' ' -f1 "$OUTDIR/$NAME.sha256")"
