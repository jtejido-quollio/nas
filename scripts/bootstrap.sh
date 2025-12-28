#!/usr/bin/env bash
set -euo pipefail

INSTALL_ZFS=1
INSTALL_K3S=1
DEPLOY=1
BUILD_IMAGES=0
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
K3S_CTR_ENV="${K3S_CTR:-}"

usage() {
  cat <<'EOF'
Usage: scripts/bootstrap.sh [options]

Options:
  --skip-zfs        Skip installing zfsutils-linux
  --skip-k3s        Skip installing k3s
  --skip-deploy     Skip deploying CRDs/node-agent/operator
  --build-images    Build and load images into k3s if missing
  --repo <path>     Repo root (default: script directory/..)
  -h, --help        Show this help

Environment:
  INSTALL_K3S_EXEC  Passed through to the k3s install script
  K3S_CTR           Override k3s ctr command (default: "sudo k3s ctr")
  PLATFORM          Passed to "make images" when --build-images is set
EOF
}

log() {
  echo "== $*"
}

die() {
  echo "error: $*" >&2
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --skip-zfs) INSTALL_ZFS=0 ;;
    --skip-k3s) INSTALL_K3S=0 ;;
    --skip-deploy) DEPLOY=0 ;;
    --build-images) BUILD_IMAGES=1 ;;
    --repo)
      shift
      [[ $# -gt 0 ]] || die "--repo requires a path"
      REPO_ROOT="$1"
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      die "unknown option: $1"
      ;;
  esac
  shift
done

if [[ ! -f "$REPO_ROOT/Makefile" ]]; then
  die "repo root not found: $REPO_ROOT"
fi

run_root() {
  if [[ ${EUID:-$(id -u)} -ne 0 ]]; then
    command -v sudo >/dev/null 2>&1 || die "sudo not found; run as root"
    sudo "$@"
  else
    "$@"
  fi
}

log "Installing base packages"
run_root apt-get update -y
run_root apt-get install -y curl git make

if [[ "$INSTALL_ZFS" == "1" ]]; then
  log "Installing ZFS utilities"
  run_root apt-get install -y zfsutils-linux
fi

if [[ "$INSTALL_K3S" == "1" ]]; then
  if ! command -v k3s >/dev/null 2>&1; then
    log "Installing k3s"
    if [[ -n "${INSTALL_K3S_EXEC:-}" ]]; then
      run_root sh -c "curl -sfL https://get.k3s.io | INSTALL_K3S_EXEC=\"$INSTALL_K3S_EXEC\" sh -"
    else
      run_root sh -c "curl -sfL https://get.k3s.io | sh -"
    fi
  else
    log "k3s already installed"
  fi
fi

if [[ "$DEPLOY" == "1" ]]; then
  if [[ -n "${KUBECTL:-}" ]]; then
    KUBECTL_CMD=($KUBECTL)
  elif command -v kubectl >/dev/null 2>&1; then
    KUBECTL_CMD=(kubectl)
  elif command -v k3s >/dev/null 2>&1; then
    KUBECTL_CMD=(sudo k3s kubectl)
  else
    die "kubectl not found (install kubectl or k3s)"
  fi

  if [[ -n "$K3S_CTR_ENV" ]]; then
    K3S_CTR_CMD=($K3S_CTR_ENV)
  elif command -v k3s >/dev/null 2>&1; then
    K3S_CTR_CMD=(sudo k3s ctr)
  else
    K3S_CTR_CMD=()
  fi

  if [[ ${#K3S_CTR_CMD[@]} -gt 0 ]]; then
    missing=0
    if ! "${K3S_CTR_CMD[@]}" images ls | grep -q "nas-node-agent:dev"; then
      missing=1
    fi
    if ! "${K3S_CTR_CMD[@]}" images ls | grep -q "nas-operator:dev"; then
      missing=1
    fi
    if [[ "$missing" == "1" ]]; then
      if [[ "$BUILD_IMAGES" == "1" ]]; then
        command -v docker >/dev/null 2>&1 || die "docker not found for --build-images"
        log "Building and loading images"
        (cd "$REPO_ROOT" && make images)
        (cd "$REPO_ROOT" && K3S_CTR="${K3S_CTR_CMD[*]}" make load-images)
      else
        die "missing images in containerd; run with --build-images or preload images"
      fi
    fi
  else
    die "k3s ctr not available; set K3S_CTR or install k3s"
  fi

  log "Deploying CRDs, node-agent, operator"
  "${KUBECTL_CMD[@]}" apply -k "$REPO_ROOT/config/crd"
  "${KUBECTL_CMD[@]}" apply -k "$REPO_ROOT/config/node-agent"
  "${KUBECTL_CMD[@]}" apply -k "$REPO_ROOT/config/operator"
fi

log "Bootstrap complete"
