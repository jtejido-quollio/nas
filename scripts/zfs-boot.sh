#!/usr/bin/env bash
set -euo pipefail

log() {
  echo "== $*"
}

if ! command -v zpool >/dev/null 2>&1; then
  log "zpool not found; skipping ZFS import"
  exit 0
fi

log "Importing ZFS pools"
zpool import -a -N || true

if ! command -v zfs >/dev/null 2>&1; then
  log "zfs not found; skipping mount"
  exit 0
fi

log "Mounting ZFS datasets"
zfs mount -a || true
