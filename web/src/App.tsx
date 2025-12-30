import { useEffect, useMemo, useState } from "react";
import { fetchOverview, Overview, NASDirectory, NASShare, ZDataset, ZPool } from "./api";

const navItems = [
  { label: "Dashboard", active: true },
  { label: "Pools" },
  { label: "Datasets" },
  { label: "Shares" },
  { label: "Directories" },
  { label: "Snapshots" }
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
  if (!condition) return "unknown";
  return condition.status === "True" ? "connected" : "offline";
}

export default function App() {
  const [overview, setOverview] = useState<Overview | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let active = true;
    setLoading(true);
    fetchOverview()
      .then((data) => {
        if (!active) return;
        setOverview(data);
        setError(null);
      })
      .catch((err: Error) => {
        if (!active) return;
        setError(err.message);
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => {
      active = false;
    };
  }, []);

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
            <button key={item.label} className={item.active ? "nav-item active" : "nav-item"}>
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
            <h1>Storage Overview</h1>
            <p className="subtitle">
              Unified view of pools, datasets, shares, and directories across the NAS control plane.
            </p>
          </div>
          <div className="topbar-actions">
            <div className={`health-pill ${healthLabel === "Healthy" ? "good" : "warn"}`}>
              {healthLabel}
              <span className="health-count">{errorCount}</span>
            </div>
            <button className="ghost">Sync</button>
          </div>
        </header>

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

        {error && (
          <div className="alert">
            <strong>API error:</strong> {error}
          </div>
        )}
      </main>
    </div>
  );
}
