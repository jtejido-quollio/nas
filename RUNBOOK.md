# RUNBOOK (Mac + Docker/Podman + k3s on Linux VM)

This runbook assumes k3s is already running on a **Linux VM** because ZFS
operations must run on Linux.

## 0) Prereqs on Mac
- Docker Desktop or Podman Desktop installed (optional if you build inside the VM)
- `kubectl` and `kustomize` on your Mac (optional if you run everything in the VM)
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

### Optional: bootstrap script
This repo includes a bootstrap helper for Phase 0 setup (ZFS + k3s + core pods):
```bash
./scripts/bootstrap.sh --build-images
```
Use `--skip-deploy` if you only want dependencies installed, or `--skip-nfs`
if you don't want the kernel NFS server installed.

### Optional: systemd service wrapper
To run the bootstrap at boot, use the unit in `scripts/nas-bootstrap.service`:
```bash
sudo cp scripts/nas-bootstrap.service /etc/systemd/system/nas-bootstrap.service
sudo tee /etc/default/nas-bootstrap >/dev/null <<'EOF'
REPO_ROOT=/opt/nas
BOOTSTRAP_OPTS=--build-images
EOF
sudo systemctl daemon-reload
sudo systemctl enable --now nas-bootstrap.service
```
Adjust `REPO_ROOT` to where the repo lives.

If `kubectl` is not installed on the VM, k3s bundles it:
```bash
sudo k3s kubectl get nodes
```

To keep using `kubectl` in commands below, either install `kubectl` or run:
```bash
export KUBECTL="sudo k3s kubectl"
alias kubectl="sudo k3s kubectl"
```

### Samples smoke (optional)
```bash
make deploy-samples
make samples-smoke NODE_AGENT_URL=http://<node-ip>:9808
```

## 2) Verify k3s is running
```bash
kubectl get nodes
kubectl get pods -A | head
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
make K3S_CTR="sudo k3s ctr" load-images
```

`make load-images` saves the images and imports them into k3s containerd.
If you already have another local registry/runtime, load images there instead.

If your VM is ARM64 (Apple Silicon/UTM), build with:
```bash
make PLATFORM=linux/arm64 images
```

Samples validation (node-agent API only) is documented in `config/samples/README.md`.

## 5) Update sample node/device values
Before deploying, update `nodeName` and device paths to match your VM:
```bash
kubectl get nodes -o wide
```

Files to edit (anything with `nodeName`):
- `config/samples/10-pool/zpool.yaml`
- `config/samples/20-dataset/zdataset-home.yaml`
- `config/samples/20-dataset/zdataset-nfs.yaml`
- `config/samples/40-snapshots/zsnapshotschedule-home.yaml`
- `config/samples/50-restore/zsnapshotrestore-clone.yaml`

## 6) Deploy samples (Storage + shares)
The samples include:
* nas-node-agent + nas-operator
* OpenEBS ZFS LocalPV (CSI) for dynamic PVC provisioning
* CSI VolumeSnapshot support (ZSnapshot + ZSnapshotRestore mode=csi)
* NASShare resources (SMB + NFS) backed by ZFS datasets / PVCs

```bash
make deploy-samples
```

### Ensure pools/datasets mount after reboot (optional)
To make the "dataset survives reboot" criterion explicit, enable the ZFS boot
unit so pools are imported and datasets mounted on startup:
```bash
sudo cp scripts/nas-zfs-boot.service /etc/systemd/system/nas-zfs-boot.service
sudo tee /etc/default/nas-zfs-boot >/dev/null <<'EOF'
REPO_ROOT=/opt/nas
EOF
sudo systemctl daemon-reload
sudo systemctl enable --now nas-zfs-boot.service
```

### Verify core resources
```bash
kubectl -n nas-system get pods -o wide
kubectl -n nas-system get zpool,zdataset,zsnapshot,zsnapshotrestore
kubectl -n nas-system get pvc,pv,volumesnapshot
```

### Samples health (optional)
```bash
make samples-smoke
```

Wait until:
* the PVC is Bound
* the writer pod is Running (or Succeeded)
* the VolumeSnapshot is ReadyToUse
* the restore PVC is Bound

## 7) NASShare notes (SMB + NFS)
The samples use NASShare resources; the Time Machine share mounts a CSI-backed PVC.
The home share uses the ZFS dataset directly so snapshot schedules remain aligned.
The sample Time Machine PVC size is small for labs; adjust `config/samples/25-pvc/pvc-timemachine.yaml`
if your pool has more capacity.
The default samples assume a local directory (`NASDirectory` named `local`) and use `NASUser`/`NASGroup`.
Optional AD/LDAP samples are in `config/samples/00-directory/` but are not part of the default kustomization.
to define local identities for SMB/NFS.
Samples expect host packages/services when using NFS or AD/LDAP:
- `nfs-kernel-server` (kernel NFS exports)
- `sssd` + `ldap-utils` (AD/LDAP identity lookups)
The health script will fail if these are missing when relevant.

## 8) Verify resources
```bash
kubectl -n nas-system get pods -o wide
kubectl -n nas-system get svc -o wide
kubectl -n nas-system get zpool,zdataset,nasshare,nasdirectory,nasuser,nasgroup,zsnapshotschedule
```

### Samples health script (optional)
```bash
./scripts/samples-health.sh
```
Optional AD/LDAP smoke tests (these patch `nfs-share` temporarily and then restore it):
```bash
make samples-ad-smoke
make samples-ldap-smoke
```
You can override the share or user being checked:
```bash
NASSHARE_NAME=nfs-share SMOKE_USER=alice make samples-ad-smoke
```

## 9) Deploy nas-api + dashboard (optional)
Build and load the `nas-api` image:
```bash
make build
make images
make K3S_CTR="sudo k3s ctr" load-images
make deploy-api
```

Access the UI:
```bash
http://<VM-IP>:30080
```

Or use port-forwarding:
```bash
kubectl -n nas-system port-forward svc/nas-api 8080:8080
```

Local UI dev (optional):
```bash
cd web
npm install
VITE_API_BASE=http://<VM-IP>:30080 npm run dev
```

## 10) Connect from your Mac (SMB)
Find the VM's IP address:
```bash
ip a
```

SMB NodePort for home share defaults to `30445`.
On macOS Finder:
- Go → Connect to Server
- `smb://<VM-IP>:30445/home`

Username/password are from `config/samples/00-secrets/smb-user-alice.yaml` and
the NASUser in `config/samples/00-users/nasuser-alice.yaml`.
Samples set `options.autoPermissions.mode: "0777"` to chmod the dataset mountpoint
for SMB writes. Remove it if you want to manage permissions manually.

If `.zfs/snapshot` is missing on the share, ensure the dataset is mounted and reconnect:
```bash
sudo zfs get mounted,snapdir tank/home
sudo zfs mount tank/home
```
Then disconnect/reconnect the share and check in Terminal:
```bash
ls /Volumes/home/.zfs/snapshot
```

### Directory services (optional)
For AD/LDAP, you can inject raw Samba globals via `options.globalOptions` in a
NASShare (manual join still required). Example:
```yaml
options:
  globalOptions:
    security: ads
    realm: EXAMPLE.COM
    workgroup: EXAMPLE
```
To test the AD sample (`config/samples/00-directory/nasdirectory-ad.yaml`),
create the bind and CA secrets, then apply the sample:
```bash
kubectl -n nas-system create secret generic ad-bind \
  --from-literal=password='AdminPass123!' \
  --dry-run=client -o yaml | kubectl apply -f -

sudo cp /var/lib/samba/private/tls/ca.pem /tmp/ad-ca.pem
kubectl -n nas-system create secret generic ad-ca \
  --from-file=ca.crt=/tmp/ad-ca.pem \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl -n nas-system apply -f config/samples/00-directory/nasdirectory-ad.yaml
```
The AD sample uses LDAPS on `ldaps://<VM-IP>:636`.
Samba AD join state is stored in `/var/lib/nas/samba/<share-name>` on the node by default.
Override with `options.adJoinStatePath` in the NASShare if you want a different location.

To test the LDAP sample (`config/samples/00-directory/nasdirectory-ldap.yaml`),
create the bind and CA secrets, then apply the sample:
```bash
kubectl -n nas-system create secret generic ldap-bind \
  --from-literal=password='LdapPass123!' \
  --dry-run=client -o yaml | kubectl apply -f -

sudo cp /path/to/ldap/ca.pem /tmp/ldap-ca.pem
kubectl -n nas-system create secret generic ldap-ca \
  --from-file=ca.crt=/tmp/ldap-ca.pem \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl -n nas-system apply -f config/samples/00-directory/nasdirectory-ldap.yaml
```
The LDAP sample defaults to LDAPS on `ldaps://<VM-IP>:636`.

To switch an existing NASShare to use AD or LDAP after applying a directory:
```bash
kubectl -n nas-system patch nasshare nfs-share --type merge -p '{"spec":{"directoryRef":"ad"}}'
kubectl -n nas-system patch nasshare nfs-share --type merge -p '{"spec":{"directoryRef":"ldap"}}'
```

If AD/LDAP users are not showing up via `getent`, force a refresh:
```bash
kubectl -n nas-system annotate nasdirectory ad nas.io/force="$(date +%s)" --overwrite
kubectl -n nas-system annotate nasshare nfs-share nas.io/force="$(date +%s)" --overwrite
sudo systemctl restart sssd
sudo journalctl -u sssd -n 50 --no-pager
```

Quick sanity checks:
```bash
kubectl -n nas-system get secret nasdirectory-ad-nfs-sssd \
  -o jsonpath='{.data.sssd\.conf}' | base64 -d
sudo cat /etc/sssd/sssd.conf
id alice
```

## 11) Connect via NFS (kernel)
Ensure `nfs-kernel-server` is installed on the VM (bootstrap does this).
On macOS:
```bash
sudo mkdir -p /Volumes/nfs
sudo mount -t nfs <VM-IP>:/mnt/tank/nfs /Volumes/nfs
```
Kernel NFS is host-based in Phase 2 (exports are managed by the node-agent).
A dedicated NFS gateway pod and CSI-mounted NFS exports are planned for a later phase.

## 12) Validate snapshots and Previous Versions
- Create a file in the SMB share
- Wait 2–4 minutes (sample schedule is every 2 minutes)
- Modify/delete the file
- On Windows: right-click → Properties → Previous Versions
 - On macOS: list `.zfs/snapshot` and copy a file out of a snapshot to confirm content

## 13) Restore by clone
1. List snapshots from ZFS (inside node):
```bash
sudo zfs list -t snapshot -o name -r tank/home | head
```
2. Edit `config/samples/50-restore/zsnapshotrestore-clone.yaml` to point to a real snapshot name.
3. Apply:
```bash
kubectl apply -k config/samples
kubectl -n nas-system describe zsnapshotrestore home-restore-clone
```

## 14) Cleanup
```bash
make cleanup-samples
```

## Notes / gotchas
### Why you typically need Linux (even if you run kubectl from macOS)
ZFS, SMART, and raw block device management require Linux kernel capabilities and privileged device access.
You can run kubectl from macOS, but the ZFS work still happens in the Linux VM.

### OpenEBS manifest fetch
`config/storage/openebs-zfs` references an upstream manifest via URL. Your cluster machine needs outbound internet (or you can vendor the YAML later).

### Privileged node-agent
node-agent runs privileged and uses host mounts. This is expected.
- NetworkPolicy enforcement depends on your CNI.
- NodePort is for lab testing; production would use a different ingress/exposure model.
