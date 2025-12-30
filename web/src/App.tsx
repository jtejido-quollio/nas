import type { ReactNode } from "react";
import { useCallback, useEffect, useMemo, useState } from "react";
import {
  fetchOverview,
  listZSnapshots,
  upsertZPool,
  deleteZPool,
  upsertZDataset,
  deleteZDataset,
  upsertNASShare,
  deleteNASShare,
  upsertNASDirectory,
  deleteNASDirectory,
  upsertZSnapshot,
  deleteZSnapshot,
  Overview,
  NASDirectory,
  NASShare,
  ZDataset,
  ZPool,
  ZSnapshot
} from "./api";

type ViewId = "dashboard" | "pools" | "datasets" | "shares" | "directories" | "snapshots";

type ModalKind = "pools" | "datasets" | "shares" | "directories" | "snapshots";

type ModalState = {
  kind: ModalKind;
  mode: "create" | "edit";
  name: string;
  specJson: string;
};

type Column<T> = {
  label: string;
  render: (item: T) => ReactNode;
};

const navItems: Array<{ id: ViewId; label: string }> = [
  { id: "dashboard", label: "Dashboard" },
  { id: "pools", label: "Pools" },
  { id: "datasets", label: "Datasets" },
  { id: "shares", label: "Shares" },
  { id: "directories", label: "Directories" },
  { id: "snapshots", label: "Snapshots" }
];

const viewMeta: Record<ViewId, { title: string; subtitle: string }> = {
  dashboard: {
    title: "Storage Overview",
    subtitle: "Unified view of pools, datasets, shares, and directories across the NAS control plane."
  },
  pools: {
    title: "Pools",
    subtitle: "Create, edit, and remove ZFS pools."
  },
  datasets: {
    title: "Datasets",
    subtitle: "Manage datasets, mountpoints, and properties."
  },
  shares: {
    title: "Shares",
    subtitle: "Control SMB and NFS protocol gateways."
  },
  directories: {
    title: "Directories",
    subtitle: "Configure local, LDAP, and Active Directory sources."
  },
  snapshots: {
    title: "Snapshots",
    subtitle: "Create and manage ZFS snapshots." 
  }
};

const defaultSpecByKind: Record<ModalKind, object> = {
  pools: {
    nodeName: "",
    poolName: "",
    vdevs: [{ type: "mirror", devices: [] }]
  },
  datasets: {
    nodeName: "",
    datasetName: "",
    properties: {}
  },
  shares: {
    protocol: "smb",
    shareName: "",
    datasetName: "",
    mountPath: "",
    directoryRef: "local",
    readOnly: false,
    options: {}
  },
  directories: {
    type: "local",
    servers: [],
    baseDN: ""
  },
  snapshots: {
    pvcName: "",
    snapshotClassName: ""
  }
};

function getStatus(phase?: string) {
  if (!phase) return "unknown";
  return phase.toLowerCase();
}

function statusTone(phase?: string) {
  const value = getStatus(phase);
  if (value === "ready" || value === "ok") return "good";
  if (value === "error" || value === "failed") return "bad";
  return "warn";
}

function directoryConnectivity(directory: NASDirectory) {
  const condition = directory.status?.conditions?.find((c) => c.type === "Connectivity");
  if (!condition) return "defined";
  return condition.status === "True" ? "connected" : "offline";
}

function ResourceTable<T extends { metadata: { name: string } }>(props: {
  items: T[];
  columns: Array<Column<T>>;
  loading: boolean;
  emptyLabel: string;
  onEdit: (item: T) => void;
  onDelete: (item: T) => void;
}) {
  const { items, columns, loading, emptyLabel, onEdit, onDelete } = props;
  if (loading) {
    return <div className="panel-empty">Loading...</div>;
  }
  if (items.length === 0) {
    return <div className="panel-empty">{emptyLabel}</div>;
  }
  return (
    <div className="table-wrap">
      <table className="table">
        <thead>
          <tr>
            {columns.map((column) => (
              <th key={column.label}>{column.label}</th>
            ))}
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          {items.map((item) => (
            <tr key={item.metadata.name}>
              {columns.map((column) => (
                <td key={column.label}>{column.render(item)}</td>
              ))}
              <td>
                <div className="table-actions">
                  <button className="table-button" onClick={() => onEdit(item)}>
                    Edit
                  </button>
                  <button className="table-button danger" onClick={() => onDelete(item)}>
                    Delete
                  </button>
                </div>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

export default function App() {
  const [overview, setOverview] = useState<Overview | null>(null);
  const [snapshots, setSnapshots] = useState<ZSnapshot[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [snapshotsLoading, setSnapshotsLoading] = useState(true);
  const [activeView, setActiveView] = useState<ViewId>("dashboard");
  const [modal, setModal] = useState<ModalState | null>(null);
  const [modalError, setModalError] = useState<string | null>(null);
  const [actionBusy, setActionBusy] = useState(false);

  const refreshAll = useCallback(async () => {
    setLoading(true);
    setSnapshotsLoading(true);
    setError(null);
    try {
      const [overviewData, snapshotData] = await Promise.all([fetchOverview(), listZSnapshots()]);
      setOverview(overviewData);
      setSnapshots(snapshotData);
    } catch (err) {
      const message = err instanceof Error ? err.message : "Failed to load overview";
      setError(message);
    } finally {
      setLoading(false);
      setSnapshotsLoading(false);
    }
  }, []);

  useEffect(() => {
    refreshAll();
  }, [refreshAll]);

  const pools = overview?.pools ?? [];
  const datasets = overview?.datasets ?? [];
  const shares = overview?.shares ?? [];
  const directories = overview?.directories ?? [];

  const { errorCount, healthLabel } = useMemo(() => {
    const badPools = pools.filter((pool) => statusTone(pool.status?.phase) === "bad");
    const badDatasets = datasets.filter((dataset) => statusTone(dataset.status?.phase) === "bad");
    const badShares = shares.filter((share) => statusTone(share.status?.phase) === "bad");
    const badDirs = directories.filter((dir) => directoryConnectivity(dir) === "offline");
    const count = badPools.length + badDatasets.length + badShares.length + badDirs.length;
    return {
      errorCount: count,
      healthLabel: count === 0 ? "Healthy" : "Attention"
    };
  }, [pools, datasets, shares, directories]);

  const handleCreate = (kind: ModalKind) => {
    setModalError(null);
    setModal({
      kind,
      mode: "create",
      name: "",
      specJson: JSON.stringify(defaultSpecByKind[kind], null, 2)
    });
  };

  const handleEdit = (kind: ModalKind, item: { metadata: { name: string }; spec?: object }) => {
    setModalError(null);
    setModal({
      kind,
      mode: "edit",
      name: item.metadata.name,
      specJson: JSON.stringify(item.spec ?? {}, null, 2)
    });
  };

  const handleDelete = async (kind: ModalKind, name: string) => {
    if (!window.confirm(`Delete ${name}? This cannot be undone.`)) return;
    setActionBusy(true);
    try {
      switch (kind) {
        case "pools":
          await deleteZPool(name);
          break;
        case "datasets":
          await deleteZDataset(name);
          break;
        case "shares":
          await deleteNASShare(name);
          break;
        case "directories":
          await deleteNASDirectory(name);
          break;
        case "snapshots":
          await deleteZSnapshot(name);
          break;
      }
      await refreshAll();
    } catch (err) {
      const message = err instanceof Error ? err.message : "Delete failed";
      setError(message);
    } finally {
      setActionBusy(false);
    }
  };

  const handleModalSubmit = async () => {
    if (!modal) return;
    if (modal.name.trim() === "") {
      setModalError("Name is required.");
      return;
    }
    let spec: object;
    try {
      spec = JSON.parse(modal.specJson);
    } catch (err) {
      const message = err instanceof Error ? err.message : "Invalid JSON";
      setModalError(message);
      return;
    }
    setActionBusy(true);
    try {
      switch (modal.kind) {
        case "pools":
          await upsertZPool(modal.name, spec as ZPool["spec"]);
          break;
        case "datasets":
          await upsertZDataset(modal.name, spec as ZDataset["spec"]);
          break;
        case "shares":
          await upsertNASShare(modal.name, spec as NASShare["spec"]);
          break;
        case "directories":
          await upsertNASDirectory(modal.name, spec as NASDirectory["spec"]);
          break;
        case "snapshots":
          await upsertZSnapshot(modal.name, spec as ZSnapshot["spec"]);
          break;
      }
      setModal(null);
      setModalError(null);
      await refreshAll();
    } catch (err) {
      const message = err instanceof Error ? err.message : "Save failed";
      setModalError(message);
    } finally {
      setActionBusy(false);
    }
  };

  const busy = loading || snapshotsLoading || actionBusy;
  const view = viewMeta[activeView];

  return (
    <div className="app">
      <aside className="nav">
        <div className="nav-brand">
          <img className="brand-logo" src="/amphora-logo.png" alt="Amphora logo" />
          <span className="brand-sub">NAS Control Plane</span>
        </div>
        <div className="nav-section">Core</div>
        <nav className="nav-items">
          {navItems.map((item) => (
            <button
              key={item.label}
              className="nav-item"
              onClick={() => setActiveView(item.id)}
              aria-pressed={activeView === item.id}
            >
              {item.label}
            </button>
          ))}
        </nav>
        <div className="nav-divider" />
        <div className="nav-footer">
          <div className="nav-pill">API-backed</div>
          <div className="nav-small">CRDs only · etcd storage</div>
        </div>
      </aside>

      <main className="main">
        <header className="topbar">
          <div>
            <h1>{view.title}</h1>
            <p className="subtitle">{view.subtitle}</p>
          </div>
          <div className="topbar-actions">
            {activeView === "dashboard" && (
              <div className={`health-pill ${healthLabel === "Healthy" ? "good" : "warn"}`}>
                {healthLabel}
                <span className="health-count">{errorCount}</span>
              </div>
            )}
            <button className="ghost" onClick={refreshAll} disabled={busy}>
              Sync
            </button>
          </div>
        </header>

        {activeView === "dashboard" && (
          <>
            <section className="hero">
              <div className="hero-card">
                <div>
                  <div className="eyebrow">Clusters</div>
                  <div className="metric">Single-node</div>
                  <div className="metric-sub">k3s + ZFS LocalPV</div>
                </div>
                <div className="hero-meta">
                  <div>
                    <span>Control plane</span>
                    <strong>nas-api</strong>
                  </div>
                  <div>
                    <span>Namespace</span>
                    <strong>nas-system</strong>
                  </div>
                </div>
              </div>
              <div className="hero-panel">
                <div className="hero-panel-title">System signals</div>
                <div className="signal-row">
                  <span>Pool health</span>
                  <span className={`signal ${pools.length ? "good" : "warn"}`}>
                    {pools.length ? "Ready" : "Missing"}
                  </span>
                </div>
                <div className="signal-row">
                  <span>Directory connectivity</span>
                  <span className={`signal ${directories.length ? "good" : "warn"}`}>
                    {directories.length ? "Defined" : "None"}
                  </span>
                </div>
                <div className="signal-row">
                  <span>Share coverage</span>
                  <span className={`signal ${shares.length ? "good" : "warn"}`}>
                    {shares.length ? "Active" : "None"}
                  </span>
                </div>
              </div>
            </section>

            <section className="stat-grid">
              <div className="stat-card">
                <div className="stat-label">Pools</div>
                <div className="stat-value">{pools.length}</div>
                <div className="stat-meta">ZFS lifecycles & topology</div>
              </div>
              <div className="stat-card">
                <div className="stat-label">Datasets</div>
                <div className="stat-value">{datasets.length}</div>
                <div className="stat-meta">Mountpoints, properties, snapshots</div>
              </div>
              <div className="stat-card">
                <div className="stat-label">Shares</div>
                <div className="stat-value">{shares.length}</div>
                <div className="stat-meta">SMB + NFS protocol gateways</div>
              </div>
              <div className="stat-card">
                <div className="stat-label">Directories</div>
                <div className="stat-value">{directories.length}</div>
                <div className="stat-meta">Local, LDAP, Active Directory</div>
              </div>
            </section>

            <section className="panel-grid">
              <div className="panel">
                <div className="panel-header">
                  <h2>Pools</h2>
                  <span className="panel-chip">ZPool</span>
                </div>
                <div className="panel-body">
                  {loading && <div className="panel-empty">Loading pools...</div>}
                  {!loading && pools.length === 0 && <div className="panel-empty">No pools yet.</div>}
                  {!loading && pools.length > 0 && (
                    <ul className="list">
                      {pools.map((pool) => (
                        <li key={pool.metadata.name} className="list-item">
                          <div>
                            <div className="list-title">{pool.spec.poolName || pool.metadata.name}</div>
                            <div className="list-sub">{pool.metadata.name}</div>
                          </div>
                          <span className={`status ${statusTone(pool.status?.phase)}`}>
                            {pool.status?.phase ?? "Unknown"}
                          </span>
                        </li>
                      ))}
                    </ul>
                  )}
                </div>
              </div>

              <div className="panel">
                <div className="panel-header">
                  <h2>Datasets</h2>
                  <span className="panel-chip">ZDataset</span>
                </div>
                <div className="panel-body">
                  {loading && <div className="panel-empty">Loading datasets...</div>}
                  {!loading && datasets.length === 0 && <div className="panel-empty">No datasets yet.</div>}
                  {!loading && datasets.length > 0 && (
                    <ul className="list">
                      {datasets.map((dataset) => (
                        <li key={dataset.metadata.name} className="list-item">
                          <div>
                            <div className="list-title">{dataset.spec.datasetName || dataset.metadata.name}</div>
                            <div className="list-sub">{dataset.spec.mountpoint || "auto"}</div>
                          </div>
                          <span className={`status ${statusTone(dataset.status?.phase)}`}>
                            {dataset.status?.phase ?? "Unknown"}
                          </span>
                        </li>
                      ))}
                    </ul>
                  )}
                </div>
              </div>

              <div className="panel">
                <div className="panel-header">
                  <h2>Shares</h2>
                  <span className="panel-chip">NASShare</span>
                </div>
                <div className="panel-body">
                  {loading && <div className="panel-empty">Loading shares...</div>}
                  {!loading && shares.length === 0 && <div className="panel-empty">No shares yet.</div>}
                  {!loading && shares.length > 0 && (
                    <ul className="list">
                      {shares.map((share) => (
                        <li key={share.metadata.name} className="list-item">
                          <div>
                            <div className="list-title">{share.spec.shareName || share.metadata.name}</div>
                            <div className="list-sub">
                              {share.spec.protocol?.toUpperCase() ?? "PROTO"} · {share.spec.datasetName || "dataset"}
                            </div>
                          </div>
                          <span className={`status ${statusTone(share.status?.phase)}`}>
                            {share.status?.phase ?? "Pending"}
                          </span>
                        </li>
                      ))}
                    </ul>
                  )}
                </div>
              </div>

              <div className="panel">
                <div className="panel-header">
                  <h2>Directories</h2>
                  <span className="panel-chip">NASDirectory</span>
                </div>
                <div className="panel-body">
                  {loading && <div className="panel-empty">Loading directories...</div>}
                  {!loading && directories.length === 0 && (
                    <div className="panel-empty">No directory sources defined.</div>
                  )}
                  {!loading && directories.length > 0 && (
                    <ul className="list">
                      {directories.map((dir) => (
                        <li key={dir.metadata.name} className="list-item">
                          <div>
                            <div className="list-title">{dir.metadata.name}</div>
                            <div className="list-sub">
                              {(dir.spec.type ?? "local").toUpperCase()} · {dir.spec.baseDN || ""}
                            </div>
                          </div>
                          <span className={`status ${directoryConnectivity(dir) === "connected" ? "good" : "warn"}`}>
                            {directoryConnectivity(dir)}
                          </span>
                        </li>
                      ))}
                    </ul>
                  )}
                </div>
              </div>
            </section>

            <section className="panel wide">
              <div className="panel-header">
                <h2>Data-plane readiness</h2>
                <span className="panel-chip">Policy</span>
              </div>
              <div className="panel-body rows">
                <div className="row">
                  <div>
                    <div className="row-title">SMB gateways</div>
                    <div className="row-sub">NASShare → Samba config → service endpoints</div>
                  </div>
                  <span className={`status ${shares.some((share) => share.spec.protocol === "smb") ? "good" : "warn"}`}>
                    {shares.some((share) => share.spec.protocol === "smb") ? "Defined" : "None"}
                  </span>
                </div>
                <div className="row">
                  <div>
                    <div className="row-title">NFS exports</div>
                    <div className="row-sub">NASShare → exports.d → kernel NFS</div>
                  </div>
                  <span className={`status ${shares.some((share) => share.spec.protocol === "nfs") ? "good" : "warn"}`}>
                    {shares.some((share) => share.spec.protocol === "nfs") ? "Defined" : "None"}
                  </span>
                </div>
                <div className="row">
                  <div>
                    <div className="row-title">Directory bindings</div>
                    <div className="row-sub">Local, LDAP, or AD integrations</div>
                  </div>
                  <span className={`status ${directories.length ? "good" : "warn"}`}>
                    {directories.length ? "Ready" : "Unset"}
                  </span>
                </div>
              </div>
            </section>
          </>
        )}

        {activeView === "pools" && (
          <section className="resource-page">
            <div className="page-header">
              <div>
                <h2>ZFS Pools</h2>
                <p className="page-sub">Create or destroy pools, and adjust vdev layouts.</p>
              </div>
              <button className="primary" onClick={() => handleCreate("pools")} disabled={busy}>
                Create Pool
              </button>
            </div>
            <ResourceTable
              items={pools}
              columns={[
                { label: "Name", render: (pool) => pool.metadata.name },
                { label: "Pool", render: (pool) => pool.spec.poolName || "" },
                { label: "Node", render: (pool) => pool.spec.nodeName || "" },
                {
                  label: "Vdevs",
                  render: (pool) =>
                    pool.spec.vdevs?.map((vdev) => vdev.type).filter(Boolean).join(", ") || ""
                },
                { label: "Status", render: (pool) => <span className={`status ${statusTone(pool.status?.phase)}`}>
                    {pool.status?.phase ?? "Unknown"}
                  </span> }
              ]}
              loading={loading}
              emptyLabel="No pools yet."
              onEdit={(item) => handleEdit("pools", item)}
              onDelete={(item) => handleDelete("pools", item.metadata.name)}
            />
          </section>
        )}

        {activeView === "datasets" && (
          <section className="resource-page">
            <div className="page-header">
              <div>
                <h2>Datasets</h2>
                <p className="page-sub">Manage datasets and their properties.</p>
              </div>
              <button className="primary" onClick={() => handleCreate("datasets")} disabled={busy}>
                Create Dataset
              </button>
            </div>
            <ResourceTable
              items={datasets}
              columns={[
                { label: "Name", render: (dataset) => dataset.metadata.name },
                { label: "Dataset", render: (dataset) => dataset.spec.datasetName || "" },
                { label: "Node", render: (dataset) => dataset.spec.nodeName || "" },
                { label: "Mount", render: (dataset) => dataset.spec.mountpoint || "auto" },
                { label: "Status", render: (dataset) => <span className={`status ${statusTone(dataset.status?.phase)}`}>
                    {dataset.status?.phase ?? "Unknown"}
                  </span> }
              ]}
              loading={loading}
              emptyLabel="No datasets yet."
              onEdit={(item) => handleEdit("datasets", item)}
              onDelete={(item) => handleDelete("datasets", item.metadata.name)}
            />
          </section>
        )}

        {activeView === "shares" && (
          <section className="resource-page">
            <div className="page-header">
              <div>
                <h2>Shares</h2>
                <p className="page-sub">Create SMB and NFS gateway shares.</p>
              </div>
              <button className="primary" onClick={() => handleCreate("shares")} disabled={busy}>
                Create Share
              </button>
            </div>
            <ResourceTable
              items={shares}
              columns={[
                { label: "Name", render: (share) => share.metadata.name },
                { label: "Share", render: (share) => share.spec.shareName || "" },
                { label: "Protocol", render: (share) => share.spec.protocol?.toUpperCase() || "" },
                { label: "Dataset", render: (share) => share.spec.datasetName || "" },
                { label: "Directory", render: (share) => share.spec.directoryRef || "" },
                { label: "Status", render: (share) => <span className={`status ${statusTone(share.status?.phase)}`}>
                    {share.status?.phase ?? "Pending"}
                  </span> }
              ]}
              loading={loading}
              emptyLabel="No shares yet."
              onEdit={(item) => handleEdit("shares", item)}
              onDelete={(item) => handleDelete("shares", item.metadata.name)}
            />
          </section>
        )}

        {activeView === "directories" && (
          <section className="resource-page">
            <div className="page-header">
              <div>
                <h2>Directories</h2>
                <p className="page-sub">Manage local, LDAP, and AD directories.</p>
              </div>
              <button className="primary" onClick={() => handleCreate("directories")} disabled={busy}>
                Create Directory
              </button>
            </div>
            <ResourceTable
              items={directories}
              columns={[
                { label: "Name", render: (dir) => dir.metadata.name },
                { label: "Type", render: (dir) => (dir.spec.type ?? "local").toUpperCase() },
                { label: "Servers", render: (dir) => dir.spec.servers?.join(", ") || "" },
                { label: "Base DN", render: (dir) => dir.spec.baseDN || "" },
                { label: "Status", render: (dir) => <span className={`status ${directoryConnectivity(dir) === "connected" ? "good" : "warn"}`}>
                    {directoryConnectivity(dir)}
                  </span> }
              ]}
              loading={loading}
              emptyLabel="No directory sources yet."
              onEdit={(item) => handleEdit("directories", item)}
              onDelete={(item) => handleDelete("directories", item.metadata.name)}
            />
          </section>
        )}

        {activeView === "snapshots" && (
          <section className="resource-page">
            <div className="page-header">
              <div>
                <h2>Snapshots</h2>
                <p className="page-sub">Create and manage snapshots for PVCs.</p>
              </div>
              <button className="primary" onClick={() => handleCreate("snapshots")} disabled={busy}>
                Create Snapshot
              </button>
            </div>
            <ResourceTable
              items={snapshots}
              columns={[
                { label: "Name", render: (snap) => snap.metadata.name },
                { label: "PVC", render: (snap) => snap.spec.pvcName || "" },
                { label: "Class", render: (snap) => snap.spec.snapshotClassName || "" },
                { label: "VolumeSnapshot", render: (snap) => snap.status?.volumeSnapshotName || "" },
                { label: "Status", render: (snap) => <span className={`status ${statusTone(snap.status?.phase)}`}>
                    {snap.status?.phase ?? "Unknown"}
                  </span> }
              ]}
              loading={snapshotsLoading}
              emptyLabel="No snapshots yet."
              onEdit={(item) => handleEdit("snapshots", item)}
              onDelete={(item) => handleDelete("snapshots", item.metadata.name)}
            />
          </section>
        )}

        {error && (
          <div className="alert">
            <strong>API error:</strong> {error}
          </div>
        )}
      </main>

      {modal && (
        <div className="modal-backdrop" role="dialog" aria-modal="true">
          <div className="modal">
            <div className="modal-header">
              <div>
                <div className="modal-eyebrow">
                  {modal.mode === "create" ? "Create" : "Edit"} {modal.kind}
                </div>
                <h3>{modal.mode === "create" ? "New resource" : modal.name}</h3>
              </div>
              <button className="icon-button" onClick={() => setModal(null)} aria-label="Close">
                ✕
              </button>
            </div>
            <div className="modal-body">
              <label className="form-field">
                <span>Name</span>
                <input
                  type="text"
                  value={modal.name}
                  onChange={(event) =>
                    setModal({ ...modal, name: event.target.value })
                  }
                  disabled={modal.mode === "edit"}
                />
              </label>
              <label className="form-field">
                <span>Spec (JSON)</span>
                <textarea
                  value={modal.specJson}
                  onChange={(event) =>
                    setModal({ ...modal, specJson: event.target.value })
                  }
                  rows={10}
                />
              </label>
              {modalError && <div className="modal-error">{modalError}</div>}
            </div>
            <div className="modal-actions">
              <button className="ghost" onClick={() => setModal(null)} disabled={actionBusy}>
                Cancel
              </button>
              <button className="primary" onClick={handleModalSubmit} disabled={actionBusy}>
                {modal.mode === "create" ? "Create" : "Save"}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
