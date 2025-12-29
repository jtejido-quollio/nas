#!/usr/bin/env bash
set -euo pipefail

KUBECTL_BIN="${KUBECTL:-kubectl}"
NAMESPACE="${NAMESPACE:-nas-system}"

log() {
  echo "== $* =="
}

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

log "Phase2 CRs"
${KUBECTL_BIN} -n "${NAMESPACE}" get zpool,zdataset,nasshare,nasuser,nasgroup,zsnapshotschedule,zsnapshotrestore 2>/dev/null || true

log "NFS exports (node-agent)"
node_agent_pod="$(${KUBECTL_BIN} -n "${NAMESPACE}" get pods -l app=nas-node-agent -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || true)"
if [[ -n "$node_agent_pod" ]]; then
  ${KUBECTL_BIN} -n "${NAMESPACE}" exec "$node_agent_pod" -- cat /etc/exports.d/nas-exports 2>/dev/null || echo "(no exports file yet)"
else
  echo "(node-agent pod not found)"
fi

log "Phase2 health OK"
