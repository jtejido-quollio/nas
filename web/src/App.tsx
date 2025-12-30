import type { ReactNode } from "react";
import { useCallback, useEffect, useMemo, useState } from "react";
import {
  fetchOverview,
  listDisks,
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
  DiskInventory,
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

type PoolWizardState = {
  step: number;
  name: string;
  nodeName: string;
  encryption: boolean;
  layout: "stripe" | "mirror" | "raidz1" | "raidz2" | "raidz3" | "draid1" | "draid2" | "draid3";
  dataDevices: string;
  logDevices: string;
  cacheDevices: string;
  spareDevices: string;
};

type Column<T> = {
  label: string;
  render: (item: T) => ReactNode;
};

const navItems: Array<{ id: ViewId; label: string }> = [
  { id: "dashboard", label: "Dashboard" },
  { id: "pools", label: "Storage" },
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
    title: "Storage",
    subtitle: "Provision pools, review topology, and manage disk capacity."
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

const poolWizardSteps = [
  "General Info",
  "Data",
  "Log (Optional)",
  "Cache (Optional)",
  "Spare (Optional)",
  "Review"
];

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

const dataLayouts = new Set(["stripe", "mirror", "raidz1", "raidz2", "raidz3", "draid1", "draid2", "draid3"]);

function isDraidLayout(layout: PoolWizardState["layout"]) {
  return layout.startsWith("draid");
}

function layoutLabel(layout?: string) {
  const normalized = layout?.toLowerCase() ?? "stripe";
  switch (normalized) {
    case "mirror":
      return "Mirror";
    case "raidz1":
      return "RAIDZ1";
    case "raidz2":
      return "RAIDZ2";
    case "raidz3":
      return "RAIDZ3";
    case "draid1":
      return "dRAID1";
    case "draid2":
      return "dRAID2";
    case "draid3":
      return "dRAID3";
    case "stripe":
    default:
      return "Stripe";
  }
}

function parseDeviceList(value: string) {
  return value
    .split(/[\s,]+/)
    .map((device) => device.trim())
    .filter((device) => device.length > 0);
}

function minDevicesForLayout(layout: PoolWizardState["layout"]) {
  switch (layout) {
    case "mirror":
      return 2;
    case "raidz1":
      return 3;
    case "raidz2":
      return 4;
    case "raidz3":
      return 5;
    case "stripe":
    default:
      return 1;
  }
}

function validatePoolWizardStep(state: PoolWizardState, step: number) {
  if (step === 0) {
    const name = state.name.trim();
    if (name.length === 0) return "Pool name is required.";
    if (!/^[a-z0-9][a-z0-9-]{0,49}$/.test(name)) {
      return "Pool name must be lowercase, up to 50 chars, and may include dashes.";
    }
    if (state.nodeName.trim() === "") {
      return "Target node is required for pool creation.";
    }
  }
  if (step === 1) {
    if (isDraidLayout(state.layout)) {
      return "dRAID layouts are not yet available. Select Stripe, Mirror, or RAIDZ.";
    }
    if (!state.layout) return "Select a data layout to continue.";
    const dataDevices = parseDeviceList(state.dataDevices);
    const minDevices = minDevicesForLayout(state.layout);
    if (dataDevices.length < minDevices) {
      return `Layout ${layoutLabel(state.layout)} needs at least ${minDevices} disk(s).`;
    }
  }
  return null;
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
  const [diskInventory, setDiskInventory] = useState<DiskInventory | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [diskError, setDiskError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [snapshotsLoading, setSnapshotsLoading] = useState(true);
  const [diskLoading, setDiskLoading] = useState(true);
  const [activeView, setActiveView] = useState<ViewId>("dashboard");
  const [modal, setModal] = useState<ModalState | null>(null);
  const [modalError, setModalError] = useState<string | null>(null);
  const [poolWizard, setPoolWizard] = useState<PoolWizardState | null>(null);
  const [poolWizardError, setPoolWizardError] = useState<string | null>(null);
  const [disksModalOpen, setDisksModalOpen] = useState(false);
  const [actionBusy, setActionBusy] = useState(false);

  const refreshAll = useCallback(async () => {
    setLoading(true);
    setSnapshotsLoading(true);
    setDiskLoading(true);
    setError(null);
    setDiskError(null);
    try {
      const [overviewResult, snapshotResult, disksResult] = await Promise.allSettled([
        fetchOverview(),
        listZSnapshots(),
        listDisks()
      ]);
      if (overviewResult.status === "fulfilled") {
        setOverview(overviewResult.value);
      } else {
        setError(overviewResult.reason instanceof Error ? overviewResult.reason.message : "Failed to load overview");
      }
      if (snapshotResult.status === "fulfilled") {
        setSnapshots(snapshotResult.value);
      } else {
        setError((prev) => prev ?? "Failed to load snapshots");
      }
      if (disksResult.status === "fulfilled") {
        setDiskInventory(disksResult.value);
      } else {
        setDiskInventory(null);
        setDiskError(disksResult.reason instanceof Error ? disksResult.reason.message : "Failed to load disks");
      }
    } finally {
      setLoading(false);
      setSnapshotsLoading(false);
      setDiskLoading(false);
    }
  }, []);

  useEffect(() => {
    refreshAll();
  }, [refreshAll]);

  const pools = overview?.pools ?? [];
  const datasets = overview?.datasets ?? [];
  const shares = overview?.shares ?? [];
  const directories = overview?.directories ?? [];
  const suggestedNodeName = pools[0]?.spec.nodeName ?? "";
  const diskCount = diskInventory?.count ?? diskInventory?.disks?.length ?? 0;
  const diskUpdated = diskInventory?.updated ?? "";
  const diskSelectionEnabled = !diskLoading && !diskError && diskInventory !== null;

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

  const openPoolWizard = () => {
    setPoolWizardError(null);
    setPoolWizard({
      step: 0,
      name: "",
      nodeName: suggestedNodeName,
      encryption: false,
      layout: "mirror",
      dataDevices: "",
      logDevices: "",
      cacheDevices: "",
      spareDevices: ""
    });
  };

  const closePoolWizard = () => {
    setPoolWizard(null);
    setPoolWizardError(null);
  };

  const updatePoolWizard = (changes: Partial<PoolWizardState>) => {
    setPoolWizard((current) => (current ? { ...current, ...changes } : current));
    if (poolWizardError) {
      setPoolWizardError(null);
    }
  };

  const handlePoolWizardStep = (direction: "next" | "back") => {
    if (!poolWizard) return;
    if (direction === "next") {
      const message = validatePoolWizardStep(poolWizard, poolWizard.step);
      if (message) {
        setPoolWizardError(message);
        return;
      }
    }
    const step = direction === "next" ? poolWizard.step + 1 : poolWizard.step - 1;
    updatePoolWizard({ step: Math.min(Math.max(step, 0), poolWizardSteps.length - 1) });
  };

  const handlePoolWizardCreate = async () => {
    if (!poolWizard) return;
    if (isDraidLayout(poolWizard.layout)) {
      setPoolWizardError("dRAID layouts are not yet available in this release.");
      return;
    }
    const name = poolWizard.name.trim();
    if (name.length === 0) {
      setPoolWizardError("Pool name is required.");
      return;
    }
    if (!/^[a-z0-9][a-z0-9-]{0,49}$/.test(name)) {
      setPoolWizardError("Pool name must be lowercase, up to 50 chars, and may include dashes.");
      return;
    }
    if (poolWizard.nodeName.trim() === "") {
      setPoolWizardError("Node name is required for single-node pools.");
      return;
    }
    const dataDevices = parseDeviceList(poolWizard.dataDevices);
    const minDevices = minDevicesForLayout(poolWizard.layout);
    if (dataDevices.length < minDevices) {
      setPoolWizardError(`Layout ${layoutLabel(poolWizard.layout)} needs at least ${minDevices} disk(s).`);
      return;
    }
    const vdevs: ZPool["spec"]["vdevs"] = [{ type: poolWizard.layout, devices: dataDevices }];
    const logDevices = parseDeviceList(poolWizard.logDevices);
    const cacheDevices = parseDeviceList(poolWizard.cacheDevices);
    const spareDevices = parseDeviceList(poolWizard.spareDevices);
    if (logDevices.length) vdevs.push({ type: "log", devices: logDevices });
    if (cacheDevices.length) vdevs.push({ type: "cache", devices: cacheDevices });
    if (spareDevices.length) vdevs.push({ type: "spare", devices: spareDevices });

    setActionBusy(true);
    try {
      await upsertZPool(name, {
        nodeName: poolWizard.nodeName.trim(),
        poolName: name,
        vdevs
      });
      closePoolWizard();
      await refreshAll();
    } catch (err) {
      const message = err instanceof Error ? err.message : "Pool creation failed";
      setPoolWizardError(message);
    } finally {
      setActionBusy(false);
    }
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
  const poolWizardSpec = useMemo(() => {
    if (!poolWizard) return null;
    const dataDevices = parseDeviceList(poolWizard.dataDevices);
    const logDevices = parseDeviceList(poolWizard.logDevices);
    const cacheDevices = parseDeviceList(poolWizard.cacheDevices);
    const spareDevices = parseDeviceList(poolWizard.spareDevices);
    const vdevs: ZPool["spec"]["vdevs"] = [];
    if (dataDevices.length) {
      vdevs.push({ type: poolWizard.layout, devices: dataDevices });
    }
    if (logDevices.length) vdevs.push({ type: "log", devices: logDevices });
    if (cacheDevices.length) vdevs.push({ type: "cache", devices: cacheDevices });
    if (spareDevices.length) vdevs.push({ type: "spare", devices: spareDevices });
    return {
      nodeName: poolWizard.nodeName.trim(),
      poolName: poolWizard.name.trim(),
      vdevs
    };
  }, [poolWizard]);

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
          <section className="storage-page">
            <div className="storage-toolbar">
              <div>
                <h2>Storage Dashboard</h2>
                <p className="page-sub">Build ZFS pools from disks and monitor topology health.</p>
              </div>
              <div className="storage-actions">
                <button className="ghost" disabled>
                  Import Pool
                </button>
                <button className="ghost" onClick={() => setDisksModalOpen(true)} disabled={busy}>
                  Disks
                </button>
                <button className="primary" onClick={openPoolWizard} disabled={busy}>
                  Create Pool
                </button>
              </div>
            </div>

            <div className="storage-grid">
              <div className="storage-card">
                <div className="storage-card-title">Unassigned Disks</div>
                <div className="storage-card-value">
                  {diskSelectionEnabled ? (diskLoading ? "Loading" : diskError ? "N/A" : diskCount) : "N/A"}
                </div>
                <div className="storage-card-sub">
                  {diskSelectionEnabled && diskLoading && "Syncing disk inventory from node-agent."}
                  {diskSelectionEnabled && !diskLoading && diskError && "Disk inventory unavailable. Check node-agent connectivity."}
                  {diskSelectionEnabled &&
                    !diskLoading &&
                    !diskError &&
                    (diskCount === 0
                      ? "No disks discovered yet. Attach disks to the node to create a pool."
                      : `Discovered ${diskCount} disks${diskUpdated ? ` · Updated ${diskUpdated}` : ""}.`)}
                  {!diskSelectionEnabled &&
                    "Disk inventory is not synced yet. Connect node-agent disk discovery to enable automated selection."}
                </div>
                <button className="table-button" onClick={openPoolWizard} disabled={busy}>
                  Add to Pool
                </button>
              </div>
              <div className="storage-card storage-card-wide">
                <div className="storage-card-title">Storage Guidance</div>
                <div className="storage-card-sub">
                  Use mirror or RAIDZ layouts for redundancy. Configure log, cache, and spare vdevs based on workload.
                </div>
                <div className="storage-pill-row">
                  <span className="storage-pill">Mirror</span>
                  <span className="storage-pill">RAIDZ1</span>
                  <span className="storage-pill">RAIDZ2</span>
                  <span className="storage-pill">RAIDZ3</span>
                  <span className="storage-pill muted">dRAID (not yet available)</span>
                </div>
              </div>
            </div>

            <div className="storage-pools">
              {loading && <div className="panel-empty">Loading pools...</div>}
              {!loading && pools.length === 0 && (
                <div className="panel-empty">
                  No pools yet. Create your first pool to unlock datasets and shares.
                </div>
              )}
              {!loading &&
                pools.map((pool) => {
                  const vdevs = pool.spec.vdevs ?? [];
                  const dataVdevs = vdevs.filter((vdev) => dataLayouts.has((vdev.type || "").toLowerCase()));
                  const logVdevs = vdevs.filter((vdev) => vdev.type === "log");
                  const cacheVdevs = vdevs.filter((vdev) => vdev.type === "cache");
                  const spareVdevs = vdevs.filter((vdev) => vdev.type === "spare");
                  const dataLayout = dataVdevs[0]?.type ?? "stripe";
                  const dataWidth = dataVdevs[0]?.devices?.length ?? 0;
                  const dataSummary =
                    dataVdevs.length > 0
                      ? `${dataVdevs.length} x ${layoutLabel(dataLayout)} | ${dataWidth} wide`
                      : "None";
                  const logSummary =
                    logVdevs.length > 0 ? `${logVdevs.length} vdev${logVdevs.length > 1 ? "s" : ""}` : "None";
                  const cacheSummary =
                    cacheVdevs.length > 0 ? `${cacheVdevs.length} vdev${cacheVdevs.length > 1 ? "s" : ""}` : "None";
                  const spareSummary =
                    spareVdevs.length > 0 ? `${spareVdevs.length} vdev${spareVdevs.length > 1 ? "s" : ""}` : "None";

                  return (
                    <div key={pool.metadata.name} className="pool-card">
                      <div className="pool-card-header">
                        <div>
                          <div className="pool-title">{pool.spec.poolName || pool.metadata.name}</div>
                          <div className="pool-sub">{pool.metadata.name}</div>
                        </div>
                        <div className="pool-actions">
                          <button className="table-button" onClick={() => handleEdit("pools", pool)}>
                            Manage Devices
                          </button>
                          <button className="table-button" onClick={() => setActiveView("datasets")}>
                            Manage Datasets
                          </button>
                          <button className="table-button danger" onClick={() => handleDelete("pools", pool.metadata.name)}>
                            Export / Delete
                          </button>
                        </div>
                      </div>

                      <div className="pool-grid">
                        <div className="pool-panel">
                          <div className="pool-panel-title">Topology</div>
                          <div className="pool-row">
                            <span>Data VDEVs</span>
                            <strong>{dataSummary}</strong>
                          </div>
                          <div className="pool-row">
                            <span>Log VDEVs</span>
                            <strong>{logSummary}</strong>
                          </div>
                          <div className="pool-row">
                            <span>Cache VDEVs</span>
                            <strong>{cacheSummary}</strong>
                          </div>
                          <div className="pool-row">
                            <span>Spare VDEVs</span>
                            <strong>{spareSummary}</strong>
                          </div>
                        </div>

                        <div className="pool-panel">
                          <div className="pool-panel-title">Usage</div>
                          <div className="usage-meter">
                            <div className="usage-value">N/A</div>
                            <div className="usage-sub">Usage telemetry pending</div>
                          </div>
                          <div className="usage-list">
                            <div>
                              <span>Usable capacity</span>
                              <strong>N/A</strong>
                            </div>
                            <div>
                              <span>Used</span>
                              <strong>N/A</strong>
                            </div>
                            <div>
                              <span>Available</span>
                              <strong>N/A</strong>
                            </div>
                          </div>
                        </div>

                        <div className="pool-panel">
                          <div className="pool-panel-title">ZFS Health</div>
                          <div className="pool-row">
                            <span>Pool status</span>
                            <span className={`status ${statusTone(pool.status?.phase)}`}>
                              {pool.status?.phase ?? "Unknown"}
                            </span>
                          </div>
                          <div className="pool-row">
                            <span>Scrub task</span>
                            <strong>Not scheduled</strong>
                          </div>
                          <button className="table-button" disabled>
                            Run Scrub
                          </button>
                        </div>

                        <div className="pool-panel">
                          <div className="pool-panel-title">Disk Health</div>
                          <div className="pool-row">
                            <span>SMART status</span>
                            <strong>Not connected</strong>
                          </div>
                          <div className="pool-row">
                            <span>Temperature</span>
                            <strong>N/A</strong>
                          </div>
                          <button className="table-button" disabled>
                            View SMART Reports
                          </button>
                        </div>
                      </div>
                    </div>
                  );
                })}
            </div>
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

      {disksModalOpen && (
        <div className="modal-backdrop" role="dialog" aria-modal="true">
          <div className="modal modal-wide">
            <div className="modal-header">
              <div>
                <div className="modal-eyebrow">Storage</div>
                <h3>Disk Inventory</h3>
              </div>
              <button className="icon-button" onClick={() => setDisksModalOpen(false)} aria-label="Close">
                ✕
              </button>
            </div>
            <div className="modal-body">
              {diskLoading && (
                <div className="disk-empty">
                  <strong>Loading disk inventory...</strong>
                  <p>Waiting for node-agent discovery.</p>
                </div>
              )}
              {!diskLoading && diskError && (
                <div className="disk-empty">
                  <strong>Disk inventory unavailable.</strong>
                  <p>{diskError}</p>
                </div>
              )}
              {!diskLoading && !diskError && diskInventory && diskInventory.disks.length === 0 && (
                <div className="disk-empty">
                  <strong>No disks discovered.</strong>
                  <p>Attach disks to the node-agent host to populate inventory.</p>
                </div>
              )}
              {!diskLoading && !diskError && diskInventory && diskInventory.disks.length > 0 && (
                <div className="disk-list">
                  <div className="disk-meta">
                    <span>{diskInventory.disks.length} disks discovered</span>
                    {diskInventory.updated && <span>Updated {diskInventory.updated}</span>}
                  </div>
                  <table className="table">
                    <thead>
                      <tr>
                        <th>ID</th>
                        <th>Path</th>
                      </tr>
                    </thead>
                    <tbody>
                      {diskInventory.disks.map((disk) => (
                        <tr key={disk.id}>
                          <td>{disk.id}</td>
                          <td>{disk.path}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              )}
            </div>
            <div className="modal-actions">
              <button className="ghost" onClick={() => setDisksModalOpen(false)}>
                Close
              </button>
            </div>
          </div>
        </div>
      )}

      {poolWizard && (
        <div className="modal-backdrop" role="dialog" aria-modal="true">
          <div className="modal modal-wide">
            <div className="modal-header">
              <div>
                <div className="modal-eyebrow">Storage</div>
                <h3>Pool Creation Wizard</h3>
              </div>
              <button className="icon-button" onClick={closePoolWizard} aria-label="Close">
                ✕
              </button>
            </div>
            <div className="wizard">
              <aside className="wizard-steps">
                {poolWizardSteps.map((label, index) => (
                  <div
                    key={label}
                    className={`wizard-step${poolWizard.step === index ? " active" : ""}${
                      poolWizard.step > index ? " done" : ""
                    }`}
                  >
                    <span className="wizard-step-index">{index + 1}</span>
                    <span>{label}</span>
                  </div>
                ))}
              </aside>
              <div className="wizard-panel">
                {poolWizard.step === 0 && (
                  <div className="wizard-section">
                    <h4>General Info</h4>
                    <div className="wizard-grid">
                      <label className="form-field">
                        <span className="field-label">
                          Pool name *
                          <span
                            className="field-help"
                            title="Unique, lowercase ZFS pool identifier (max 50 chars)."
                          >
                            ?
                          </span>
                        </span>
                        <input
                          type="text"
                          value={poolWizard.name}
                          placeholder="tank"
                          onChange={(event) => updatePoolWizard({ name: event.target.value })}
                        />
                      </label>
                      <label className="form-field">
                        <span className="field-label">
                          Target node *
                          <span
                            className="field-help"
                            title="Kubernetes node name where the pool will be created (single-node appliances typically have one)."
                          >
                            ?
                          </span>
                        </span>
                        <input
                          type="text"
                          value={poolWizard.nodeName}
                          placeholder="worker-1"
                          onChange={(event) => updatePoolWizard({ nodeName: event.target.value })}
                        />
                      </label>
                    </div>
                    <label className="checkbox-field">
                      <input
                        type="checkbox"
                        checked={poolWizard.encryption}
                        onChange={(event) => updatePoolWizard({ encryption: event.target.checked })}
                        disabled
                      />
                      <span className="field-label">
                        Encryption (planned)
                        <span className="field-help" title="Encrypt pool data at rest using ZFS native encryption.">
                          ?
                        </span>
                      </span>
                    </label>
                    <p className="wizard-hint">
                      Pool names must be lowercase and are permanent. Encryption will be supported in a future release.
                    </p>
                  </div>
                )}

                {poolWizard.step === 1 && (
                  <div className="wizard-section">
                    <h4>Data VDEV</h4>
                    <label className="form-field">
                      <span className="field-label">
                        Layout *
                        <span
                          className="field-help"
                          title="VDEV redundancy layout. Mirror and RAIDZ provide fault tolerance."
                        >
                          ?
                        </span>
                      </span>
                      <select
                        value={poolWizard.layout}
                        onChange={(event) =>
                          updatePoolWizard({ layout: event.target.value as PoolWizardState["layout"] })
                        }
                      >
                        <option value="stripe">Stripe</option>
                        <option value="mirror">Mirror</option>
                        <option value="raidz1">RAIDZ1</option>
                        <option value="raidz2">RAIDZ2</option>
                        <option value="raidz3">RAIDZ3</option>
                        <option value="draid1">dRAID1</option>
                        <option value="draid2">dRAID2</option>
                        <option value="draid3">dRAID3</option>
                      </select>
                    </label>

                    <div className="wizard-grid">
                      <div className="wizard-card muted">
                        <div className="wizard-card-title">Automated Disk Selection</div>
                        <p className="wizard-card-sub">
                          Not yet available. Disk inventory sync is required to enable automated selection.
                        </p>
                        <label className="form-field">
                          <span className="field-label">
                            Disk size
                            <span className="field-help" title="Filter disks by size for automated selection.">
                              ?
                            </span>
                          </span>
                          <input type="text" value="Not yet available" disabled />
                        </label>
                        <label className="form-field">
                          <span className="field-label">
                            Width
                            <span className="field-help" title="Number of disks per data VDEV.">
                              ?
                            </span>
                          </span>
                          <input type="text" value="Not yet available" disabled />
                        </label>
                        <label className="form-field">
                          <span className="field-label">
                            Number of VDEVs
                            <span className="field-help" title="How many data VDEVs to create in this pool.">
                              ?
                            </span>
                          </span>
                          <input type="text" value="Not yet available" disabled />
                        </label>
                      </div>
                      {!isDraidLayout(poolWizard.layout) && (
                        <div className="wizard-card">
                          <div className="wizard-card-title">Manual Disk Selection</div>
                          <p className="wizard-card-sub">
                            Provide device paths separated by spaces or new lines.
                          </p>
                          <label className="form-field">
                            <span className="field-label">
                              Device paths *
                              <span className="field-help" title="Absolute device paths used to form the data VDEV.">
                                ?
                              </span>
                            </span>
                            <textarea
                              rows={6}
                              value={poolWizard.dataDevices}
                              placeholder="/dev/sdb /dev/sdc"
                              onChange={(event) => updatePoolWizard({ dataDevices: event.target.value })}
                            />
                          </label>
                        </div>
                      )}
                      {isDraidLayout(poolWizard.layout) && (
                        <div className="wizard-card muted">
                          <div className="wizard-card-title">dRAID Settings</div>
                          <p className="wizard-card-sub">
                            Not yet available. dRAID will be enabled once node-agent supports inventory + layout helpers.
                          </p>
                          <label className="form-field">
                            <span className="field-label">
                              Data devices
                              <span className="field-help" title="Number of data disks per dRAID stripe.">
                                ?
                              </span>
                            </span>
                            <input type="text" value="Not yet available" disabled />
                          </label>
                          <label className="form-field">
                            <span className="field-label">
                              Distributed hot spares
                              <span className="field-help" title="Spare capacity reserved across the dRAID vdev.">
                                ?
                              </span>
                            </span>
                            <input type="text" value="Not yet available" disabled />
                          </label>
                          <label className="form-field">
                            <span className="field-label">
                              Children
                              <span className="field-help" title="Total disks allocated to the dRAID vdev.">
                                ?
                              </span>
                            </span>
                            <input type="text" value="Not yet available" disabled />
                          </label>
                          <label className="form-field">
                            <span className="field-label">
                              Number of VDEVs
                              <span className="field-help" title="Number of dRAID vdevs in the pool.">
                                ?
                              </span>
                            </span>
                            <input type="text" value="Not yet available" disabled />
                          </label>
                        </div>
                      )}
                    </div>
                  </div>
                )}

                {poolWizard.step === 2 && (
                  <div className="wizard-section">
                    <h4>Log VDEV (Optional)</h4>
                    <p className="wizard-hint">Add high-speed devices to accelerate synchronous writes.</p>
                    <label className="form-field">
                      <span className="field-label">
                        Log device paths
                        <span className="field-help" title="Dedicated log devices (SLOG) for sync write acceleration.">
                          ?
                        </span>
                      </span>
                      <textarea
                        rows={5}
                        value={poolWizard.logDevices}
                        placeholder="/dev/nvme0n1"
                        onChange={(event) => updatePoolWizard({ logDevices: event.target.value })}
                      />
                    </label>
                  </div>
                )}

                {poolWizard.step === 3 && (
                  <div className="wizard-section">
                    <h4>Cache VDEV (Optional)</h4>
                    <p className="wizard-hint">Add L2ARC cache devices for read-heavy workloads.</p>
                    <label className="form-field">
                      <span className="field-label">
                        Cache device paths
                        <span className="field-help" title="L2ARC cache devices for read-heavy workloads.">
                          ?
                        </span>
                      </span>
                      <textarea
                        rows={5}
                        value={poolWizard.cacheDevices}
                        placeholder="/dev/nvme1n1"
                        onChange={(event) => updatePoolWizard({ cacheDevices: event.target.value })}
                      />
                    </label>
                  </div>
                )}

                {poolWizard.step === 4 && (
                  <div className="wizard-section">
                    <h4>Spare VDEV (Optional)</h4>
                    <p className="wizard-hint">Add hot spare devices to automatically replace failed disks.</p>
                    <label className="form-field">
                      <span className="field-label">
                        Spare device paths
                        <span className="field-help" title="Hot spare devices that can replace failed disks.">
                          ?
                        </span>
                      </span>
                      <textarea
                        rows={5}
                        value={poolWizard.spareDevices}
                        placeholder="/dev/sdd"
                        onChange={(event) => updatePoolWizard({ spareDevices: event.target.value })}
                      />
                    </label>
                  </div>
                )}

                {poolWizard.step === poolWizardSteps.length - 1 && (
                  <div className="wizard-section">
                    <h4>Review</h4>
                    <div className="review-grid">
                      <div>
                        <div className="review-label">Pool name</div>
                        <div className="review-value">{poolWizard.name || "N/A"}</div>
                      </div>
                      <div>
                        <div className="review-label">Target node</div>
                        <div className="review-value">{poolWizard.nodeName || "N/A"}</div>
                      </div>
                      <div>
                        <div className="review-label">Layout</div>
                        <div className="review-value">{layoutLabel(poolWizard.layout)}</div>
                      </div>
                      <div>
                        <div className="review-label">Data disks</div>
                        <div className="review-value">{parseDeviceList(poolWizard.dataDevices).length}</div>
                      </div>
                    </div>
                    <div className="review-code">
                      <div className="review-label">Spec preview</div>
                      <pre>{JSON.stringify(poolWizardSpec, null, 2)}</pre>
                    </div>
                  </div>
                )}
                {poolWizardError && <div className="modal-error">{poolWizardError}</div>}
              </div>
            </div>
            <div className="modal-actions">
              <button className="ghost" onClick={closePoolWizard} disabled={actionBusy}>
                Cancel
              </button>
              <button
                className="ghost"
                onClick={() => handlePoolWizardStep("back")}
                disabled={actionBusy || poolWizard.step === 0}
              >
                Back
              </button>
              {poolWizard.step < poolWizardSteps.length - 1 ? (
                <button className="primary" onClick={() => handlePoolWizardStep("next")} disabled={actionBusy}>
                  Next
                </button>
              ) : (
                <button className="primary" onClick={handlePoolWizardCreate} disabled={actionBusy}>
                  Create Pool
                </button>
              )}
            </div>
          </div>
        </div>
      )}

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
