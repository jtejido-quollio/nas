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

log "NFS exports (node-agent)"
node_agent_pod="$(${KUBECTL_BIN} -n "${NAMESPACE}" get pods -l app=nas-node-agent -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
if [[ -n "$node_agent_pod" ]]; then
  ${KUBECTL_BIN} -n "${NAMESPACE}" exec "$node_agent_pod" -- cat /etc/exports.d/nas.exports 2>/dev/null || echo "(no exports file yet)"
else
  echo "(node-agent pod not found)"
fi

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
