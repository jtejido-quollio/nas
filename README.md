# nas (Kubernetes-style NAS control plane)

This repository is a Kubernetes-native NAS control plane built in Go.

It now includes **Phase 1 (Storage MVP)** end-to-end:
- ZFS pool creation via node-agent (ZPool)
- Dynamic PVC provisioning via **OpenEBS ZFS LocalPV CSI**
- VolumeSnapshots via CSI (ZSnapshot) and restore to a new PVC (ZSnapshotRestore mode=csi)

Phase 2 SMB samples are included, but Phase 1 is the recommended first run.

## What this is
A small “control plane” that lets you declare storage and SMB services using CRDs:

- **ZPool** — create/import a ZFS pool on a node
- **ZDataset** — create a dataset + set properties (mountpoint, compression, snapdir)
- **ZSnapshotSchedule** — periodic snapshots + retention pruning (GMT naming)
- **ZSnapshot** — create a CSI VolumeSnapshot of a PVC
- **ZSnapshotRestore** — restore from a CSI VolumeSnapshot to a new PVC (mode=csi) or clone a ZFS dataset snapshot (mode=clone)
- **NASShare** — SMB or NFS share backed by ZFS datasets or CSI PVCs
- **NASDirectory** — identity source (local, LDAP, Active Directory)
- **NASUser/NASGroup** — local directory users/groups (secrets-backed)
- **nas-api** — REST API that reads/writes CRDs directly (etcd-backed)
- **nas-ui** — React dashboard that talks to `nas-api`

## Why we built it (vs TrueNAS/FreeNAS)
Traditional NAS OSes are **appliance-style**: one box, one UI, imperative configuration.
This system is **GitOps + Kubernetes-style**:

- Declarative desired state (CRDs)
- Automated reconciliation (operators)
- Repeatable “first run” on clean clusters
- Composable with other K8s workloads

## Who this is for
- Homelab / edge labs that want a reproducible NAS stack
- Teams experimenting with “NAS as a Kubernetes workload”
- Engineers who want a minimal, auditable control plane (no monolithic UI)

## Use cases (Phase 2)
- SMB file share for Windows/macOS clients
- Kernel NFS export for Linux clients
- Windows “Previous Versions” via ZFS snapshots + `shadow_copy2`
- macOS Time Machine target over SMB
- Safe recovery using snapshot **clone restore**
- Basic observability via CR status + pod logs
- Optional directory service config via `options.globalOptions` (manual join)

## Non-goals (explicitly out of scope)
- Automated AD/LDAP join
- HA/replication between nodes
- Full multi-tenant UI (a minimal dashboard is provided)
- Multi-tenant isolation

## Architecture and diagrams
See **ARCHITECTURE.md** for component diagrams, sequence flows, and the Phase 2 boundary.

## nas-api + UI
`nas-api` is a thin REST layer that talks directly to Kubernetes. All state is
stored in **etcd via CRDs/Secrets**; there is no separate database.

Local UI dev (runs Vite, proxies to your VM):
```bash
cd web
npm install
VITE_API_BASE=http://<vm-ip>:30080 npm run dev
```

In-cluster (serves the UI from the same pod):
```bash
make build
make images
make K3S_CTR="sudo k3s ctr" load-images
make deploy-api
```
UI is exposed on `http://<vm-ip>:30080` by default.

## How to run
See **RUNBOOK.md** for a step-by-step Mac + Docker/Podman + k3s-on-Linux-VM guide.

Quick start (inside your Linux VM / Linux host):
```bash
make tidy
make build
make images
make K3S_CTR="sudo k3s ctr" load-images
make deploy-samples
```
