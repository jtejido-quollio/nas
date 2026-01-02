#!/usr/bin/env bash
set -euo pipefail

KUBECTL_BIN="${KUBECTL:-kubectl}"
NAMESPACE="${NAMESPACE:-nas-system}"
NODE_AGENT_URL="${NODE_AGENT_URL:-http://127.0.0.1:9808}"
CURL_BIN="${CURL:-curl}"

log() {
  echo "== $* =="
}

require_cmd() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Missing required command: $cmd" >&2
    exit 1
  fi
}

require_service() {
  local svc="$1"
  if command -v systemctl >/dev/null 2>&1; then
    if ! systemctl is-active --quiet "$svc"; then
      echo "Required service not active: $svc" >&2
      exit 1
    fi
  fi
}

require_items() {
  local kind="$1"
  local names=""
  names="$(${KUBECTL_BIN} -n "${NAMESPACE}" get "$kind" -o jsonpath='{.items[*].metadata.name}' 2>/dev/null || true)"
  if [[ -z "$names" ]]; then
    echo "Missing expected resources for $kind" >&2
    exit 1
  fi
}

check_phase() {
  local kind="$1"
  local expected="$2"
  local start
  local rows
  start=$(date +%s)
  while true; do
    rows="$(${KUBECTL_BIN} -n "${NAMESPACE}" get "$kind" -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.phase}{"\t"}{.status.message}{"\n"}{end}' 2>/dev/null || true)"
    if [[ -z "$rows" ]]; then
      if (( $(date +%s) - start > 120 )); then
        echo "Missing $kind resources" >&2
        exit 1
      fi
      sleep 5
      continue
    fi

    local all_ok="true"
    while IFS=$'\t' read -r name phase message; do
      if [[ -z "$name" ]]; then
        continue
      fi
      if [[ -z "$phase" ]]; then
        all_ok="false"
        continue
      fi
      if [[ "$phase" == "Error" || "$phase" == "Failed" ]]; then
        echo "$kind/$name failed: ${message:-no status message}" >&2
        exit 1
      fi
      if [[ "$phase" != "$expected" ]]; then
        all_ok="false"
      fi
    done <<< "$rows"

    if [[ "$all_ok" == "true" ]]; then
      return 0
    fi
    if (( $(date +%s) - start > 180 )); then
      echo "$kind not ready (expected phase '$expected')" >&2
      echo "$rows" >&2
      exit 1
    fi
    sleep 5
  done
}

check_snapshot_schedule() {
  local start
  local rows
  start=$(date +%s)
  while true; do
    rows="$(${KUBECTL_BIN} -n "${NAMESPACE}" get zsnapshotschedule -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.message}{"\t"}{.status.lastSnapshotName}{"\n"}{end}' 2>/dev/null || true)"
    if [[ -z "$rows" ]]; then
      if (( $(date +%s) - start > 120 )); then
        echo "Missing zsnapshotschedule resources" >&2
        exit 1
      fi
      sleep 5
      continue
    fi

    local all_ok="true"
    while IFS=$'\t' read -r name message last_snap; do
      if [[ -z "$name" ]]; then
        continue
      fi
      if [[ "$message" != "OK" ]]; then
        all_ok="false"
      fi
      if [[ -z "$last_snap" ]]; then
        all_ok="false"
      fi
    done <<< "$rows"

    if [[ "$all_ok" == "true" ]]; then
      return 0
    fi
    if (( $(date +%s) - start > 240 )); then
      echo "zsnapshotschedule not ready (missing OK message or lastSnapshotName)" >&2
      echo "$rows" >&2
      exit 1
    fi
    sleep 5
  done
}

check_zfs_prop() {
  local dataset="$1"
  local prop="$2"
  local expected="$3"
  local actual=""
  local start
  start=$(date +%s)
  while true; do
    actual="$(${KUBECTL_BIN} -n "${NAMESPACE}" exec "$node_agent_pod" -- zfs get -H -o value "$prop" "$dataset" 2>/dev/null || true)"
    if [[ -n "$actual" ]]; then
      break
    fi
    if (( $(date +%s) - start > 120 )); then
      echo "Missing zfs property $prop for $dataset" >&2
      exit 1
    fi
    sleep 2
  done
  if [[ "$actual" != "$expected" ]]; then
    echo "Unexpected $dataset $prop: expected '$expected', got '$actual'" >&2
    exit 1
  fi
}

log "Node-agent health"
require_cmd "$CURL_BIN"
$CURL_BIN -sf "$NODE_AGENT_URL/health" >/dev/null

log "Disks"
$CURL_BIN -sf "$NODE_AGENT_URL/v1/disks" >/dev/null

log "Disk cache status"
$CURL_BIN -sf "$NODE_AGENT_URL/v1/disks/updated" >/dev/null

log "Namespace"
${KUBECTL_BIN} get ns "${NAMESPACE}" >/dev/null 2>&1 || (echo "Namespace missing"; exit 1)

log "Node-agent DaemonSet"
${KUBECTL_BIN} -n "${NAMESPACE}" rollout status ds/nas-node-agent --timeout=180s

log "Operator"
${KUBECTL_BIN} -n "${NAMESPACE}" rollout status deploy/nas-operator --timeout=180s

log "OpenEBS ZFS CSI (kube-system)"
${KUBECTL_BIN} -n kube-system rollout status deploy/openebs-zfs-localpv-controller --timeout=240s
${KUBECTL_BIN} -n kube-system rollout status ds/openebs-zfs-localpv-node --timeout=240s

log "Current pods"
${KUBECTL_BIN} -n "${NAMESPACE}" get pods -o wide

log "Sample CRs"
${KUBECTL_BIN} -n "${NAMESPACE}" get zpool,zdataset,nasshare,nasdirectory,nasuser,nasgroup,zsnapshotschedule,zsnapshotrestore 2>/dev/null || true

log "Sample CR readiness"
require_items zpool
require_items zdataset
require_items nasshare
require_items nasdirectory
require_items nasuser
require_items nasgroup
require_items zsnapshotschedule
require_items zsnapshotrestore

check_phase zpool "Ready"
check_phase zdataset "Ready"
check_phase nasshare "Ready"
check_phase nasdirectory "Ready"
check_phase zsnapshotrestore "Succeeded"
check_snapshot_schedule

node_agent_pod="$(${KUBECTL_BIN} -n "${NAMESPACE}" get pods -l app=nas-node-agent -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
if [[ -z "$node_agent_pod" ]]; then
  echo "(node-agent pod not found)"
  exit 1
fi

log "Dataset preset checks"
check_zfs_prop "tank/home" "acltype" "nfsv4"
check_zfs_prop "tank/home" "aclmode" "restricted"
check_zfs_prop "tank/home" "casesensitivity" "insensitive"
check_zfs_prop "tank/home" "atime" "on"
check_zfs_prop "tank/nfs" "acltype" "posix"
check_zfs_prop "tank/nfs" "casesensitivity" "sensitive"

log "NFS exports (node-agent)"
${KUBECTL_BIN} -n "${NAMESPACE}" exec "$node_agent_pod" -- cat /etc/exports.d/nas.exports 2>/dev/null || echo "(no exports file yet)"

log "Host dependencies"
dir_types="$(${KUBECTL_BIN} -n "${NAMESPACE}" get nasdirectory -o jsonpath='{.items[*].spec.type}' 2>/dev/null || true)"
nfs_protocols="$(${KUBECTL_BIN} -n "${NAMESPACE}" get nasshare -o jsonpath='{.items[*].spec.protocol}' 2>/dev/null || true)"

if [[ "$nfs_protocols" == *"nfs"* ]]; then
  require_cmd exportfs
  require_service nfs-kernel-server
fi

if [[ "$dir_types" == *"activeDirectory"* || "$dir_types" == *"ldap"* ]]; then
  require_cmd sssd
  require_cmd ldapwhoami
  require_service sssd
fi

log "Samples health OK"
