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

log "Phase1 CRs"
${KUBECTL_BIN} -n "${NAMESPACE}" get zpool,zdataset,zsnapshot,zsnapshotrestore 2>/dev/null || true
${KUBECTL_BIN} -n "${NAMESPACE}" get pvc,pod,volumesnapshot 2>/dev/null || true

log "Phase1 health OK"
