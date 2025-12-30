#!/usr/bin/env bash
set -euo pipefail

KUBECTL="${KUBECTL:-kubectl}"
NAMESPACE="${NAMESPACE:-nas-system}"
NASSHARE_NAME="${NASSHARE_NAME:-nfs-share}"
DIRECTORY_NAME="${DIRECTORY_NAME:-ad}"
SMOKE_USER="${SMOKE_USER:-alice}"

echo "== Namespace =="
${KUBECTL} -n "${NAMESPACE}" get ns "${NAMESPACE}" >/dev/null

echo "== NASDirectory (${DIRECTORY_NAME}) =="
${KUBECTL} -n "${NAMESPACE}" get nasdirectory "${DIRECTORY_NAME}" >/dev/null

secret_name="nasdirectory-${DIRECTORY_NAME}-nfs-sssd"
echo "== SSSD secret (${secret_name}) =="
${KUBECTL} -n "${NAMESPACE}" get secret "${secret_name}" >/dev/null

echo "== Switching NASShare (${NASSHARE_NAME}) directoryRef =="
original_dir_ref="$(${KUBECTL} -n "${NAMESPACE}" get nasshare "${NASSHARE_NAME}" -o jsonpath='{.spec.directoryRef}' 2>/dev/null || true)"
if [[ -z "${original_dir_ref}" ]]; then
  echo "Missing NASShare ${NASSHARE_NAME}" >&2
  exit 1
fi

restore_dir_ref() {
  if [[ -n "${original_dir_ref}" && "${original_dir_ref}" != "${DIRECTORY_NAME}" ]]; then
    ${KUBECTL} -n "${NAMESPACE}" patch nasshare "${NASSHARE_NAME}" --type merge \
      -p "{\"spec\":{\"directoryRef\":\"${original_dir_ref}\"}}" >/dev/null || true
  fi
}
trap restore_dir_ref EXIT

if [[ "${original_dir_ref}" != "${DIRECTORY_NAME}" ]]; then
  ${KUBECTL} -n "${NAMESPACE}" patch nasshare "${NASSHARE_NAME}" --type merge \
    -p "{\"spec\":{\"directoryRef\":\"${DIRECTORY_NAME}\"}}" >/dev/null
fi

echo "== NASShare status =="
for _ in {1..10}; do
  phase="$(${KUBECTL} -n "${NAMESPACE}" get nasshare "${NASSHARE_NAME}" -o jsonpath='{.status.phase}' 2>/dev/null || true)"
  msg="$(${KUBECTL} -n "${NAMESPACE}" get nasshare "${NASSHARE_NAME}" -o jsonpath='{.status.message}' 2>/dev/null || true)"
  if [[ "${phase}" == "Ready" ]]; then
    echo "Ready OK"
    break
  fi
  echo "Waiting (${phase:-unknown}): ${msg}"
  sleep 5
done

if ! systemctl is-active --quiet sssd; then
  echo "SSSD is not active; starting..."
  sudo systemctl restart sssd
fi

if command -v sss_cache >/dev/null 2>&1; then
  sudo sss_cache -E || true
fi

echo "== getent checks =="
sudo getent passwd "${SMOKE_USER}" || true
sudo getent group nasusers || true
echo "== id ${SMOKE_USER} =="
sudo id "${SMOKE_USER}" || true
