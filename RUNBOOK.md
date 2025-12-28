# RUNBOOK (Mac + Podman + Minikube on Linux VM)

This runbook assumes you selected **Option A**:
Minikube runs inside a **Linux VM**, because ZFS operations must run on Linux.

## 0) Prereqs on Mac
- Podman Desktop installed (for building images)
- `kubectl`, `minikube`, `kustomize` on your Mac
- A Linux VM (Ubuntu 22.04/24.04 is fine) with:
  - ZFS installed (`zfsutils-linux`)
  - Two extra disks attached for a pool OR a single disk for a test pool
  - Network connectivity to your Mac (same LAN)

Recommended VM options:
- UTM (Apple Silicon) or VMware/VirtualBox (Intel)
- 4 CPU, 8GB RAM, 80GB disk + 2 extra virtual disks

## 1) On the Linux VM: install dependencies
```bash
sudo apt-get update
sudo apt-get install -y curl git make
sudo apt-get install -y zfsutils-linux
```

Install kubectl/minikube (VM side) OR run minikube from Mac targeting the VM. The simplest is to run everything in the VM.

## 2) Start Minikube in the VM (recommended)
```bash
minikube start --cpus=4 --memory=8192 --disk-size=80g
minikube addons enable ingress
```

## 3) Get the project onto the VM
Option A (simple): `git clone` this repo inside VM.
Option B: copy the folder via `scp`.

## 4) Build images (inside the VM)
This repo provides Dockerfiles. If you use Podman, ensure it can build OCI images.

Inside the VM:
```bash
make tidy
make build
make images
make load-images
```

If `minikube image load` is not available in your setup, use your container runtime accordingly.

## 5) Deploy Phase 1 (Storage MVP)
Phase 1 includes:
* nas-node-agent + nas-operator
* OpenEBS ZFS LocalPV (CSI) for dynamic PVC provisioning
* CSI VolumeSnapshot support (ZSnapshot + ZSnapshotRestore mode=csi)

```bash
make deploy-phase1
```

### Phase 1 verify
```bash
kubectl -n nas-system get pods -o wide
kubectl -n nas-system get zpool,zdataset,zsnapshot,zsnapshotrestore
kubectl -n nas-system get pvc,pv,volumesnapshot
```

Wait until:
* the PVC is Bound
* the writer pod is Running (or Succeeded)
* the VolumeSnapshot is ReadyToUse
* the restore PVC is Bound

## 6) Deploy Phase 3 (optional)
```bash
make deploy-phase3
```

## 6) Verify resources
```bash
kubectl -n nas-system get pods -o wide
kubectl -n nas-system get svc -o wide
kubectl -n nas-system get zpool,zdataset,smbshare,zsnapshotschedule
```

## 7) Connect from your Mac (SMB)
Find the VM's IP address:
```bash
ip a
```

SMB NodePort for home share defaults to `30445`.
On macOS Finder:
- Go → Connect to Server
- `smb://<VM-IP>:30445`

Username/password are from `config/samples/phase3/00-secrets/smb-user-alice.yaml`.

## 8) Validate snapshots and Previous Versions
- Create a file in the SMB share
- Wait 2–4 minutes (sample schedule is every 2 minutes)
- Modify/delete the file
- On Windows: right-click → Properties → Previous Versions

## 9) Restore by clone
1. List snapshots from ZFS (inside node):
```bash
sudo zfs list -t snapshot -o name -r tank/home | head
```
2. Edit `config/samples/phase3/50-restore/zsnapshotrestore-clone.yaml` to point to a real snapshot name.
3. Apply:
```bash
kubectl apply -k config/samples/phase3
kubectl -n nas-system describe zsnapshotrestore home-restore-clone
```

## 10) Cleanup
```bash
make cleanup-phase3
```

## Notes / gotchas
### Why you typically need Linux (even if you run Minikube on macOS)
ZFS, SMART, and raw block device management require Linux kernel capabilities and privileged device access.
When you run Minikube on macOS (Docker/Podman driver), Kubernetes nodes are Linux VMs hidden behind the driver.
You can run the control plane on macOS, but the ZFS work still happens in that Linux VM.

### OpenEBS manifest fetch
`config/storage/openebs-zfs` references an upstream manifest via URL. Your cluster machine needs outbound internet (or you can vendor the YAML later).

### Privileged node-agent
node-agent runs privileged and uses host mounts. This is expected.
- NetworkPolicy enforcement depends on your CNI.
- NodePort is for lab testing; production would use a different ingress/exposure model.

