# Architecture (Phase 2)

## Scope boundary
Phase 2 is intentionally constrained to a single-node-friendly, end-to-end experience:

- ZFS pool + datasets
- SMB + kernel NFS shares
- Snapshots + retention
- Previous Versions (Windows)
- Time Machine (macOS)
- Restore via clone

No iSCSI/AD/HA/UI.

---

## High-level components (PUML)

Save as `docs/puml/01-context.puml` (already included in repo).

- **nas-operator**: reconciles CRDs, calls node-agent, and deploys SMB services.
- **nas-node-agent**: runs on each node (DaemonSet), performs ZFS operations on the host.
- **NASShare**: abstract share CRD (SMB/NFS).
- **SMB share pods**: per NASShare (SMB), reads config from ConfigMap + creates users from Secrets.
- **Kernel NFS exports**: per NASShare (NFS), managed via node-agent `exportfs`.

---

## Data/control plane split
- **Control plane:** CRDs + operators
- **Data plane:** ZFS on node + SMB pods (PVC or hostPath) + kernel NFS exports

---

## Main flows

### 1) Provision dataset
1. Apply `ZPool`
2. Apply `ZDataset` with mountpoint `/mnt/<pool>/<ds>`
3. Operator asks node-agent to create/ensure pool/dataset and properties.

### 2) Expose SMB share
1. Apply `NASShare` with `protocol: smb`
2. Operator generates `smb.conf` from allowlisted options
3. Operator creates:
   - ConfigMap (smb.conf)
   - Deployment (Samba container)
   - Service (NodePort for lab)
4. Clients connect over SMB.

### 3) Expose NFS export
1. Apply `NASShare` with `protocol: nfs`
2. Operator ensures dataset mounted
3. Node-agent writes `/etc/exports.d/nas-exports` + runs `exportfs -ra`
4. Clients mount via NFS.

### 4) Snapshots + Previous Versions
1. Apply `ZSnapshotSchedule` with `GMT-%Y.%m.%d-%H.%M.%S`
2. Operator creates snapshots via node-agent
3. Samba `shadow_copy2` exposes snapshots as “Previous Versions”.

### 5) Restore (clone)
1. Apply `ZSnapshotRestore` (mode=clone)
2. Operator calls node-agent to `zfs clone` into a new dataset
3. Optionally create a new NASShare pointing to the clone dataset for validation.

---

## Operational model
- Errors surface as **Conditions** on CR status.
- Reconciliation is idempotent:
  - Reapplying manifests should converge to same state.
- The system is designed to be easy to reset and redeploy.
