export type ZPool = {
  metadata: { name: string };
  spec: {
    nodeName?: string;
    poolName?: string;
    vdevs?: Array<{ type?: string; devices?: string[] }>;
  };
  status?: { phase?: string; message?: string };
};

export type ZDataset = {
  metadata: { name: string };
  spec: { nodeName?: string; datasetName?: string; mountpoint?: string; properties?: Record<string, string> };
  status?: { phase?: string; message?: string };
};

export type NASShare = {
  metadata: { name: string };
  spec: {
    protocol?: string;
    shareName?: string;
    datasetName?: string;
    mountPath?: string;
    directoryRef?: string;
    readOnly?: boolean;
    nfs?: { clients?: string[]; options?: string };
  };
  status?: { phase?: string; message?: string };
};

export type NASDirectory = {
  metadata: { name: string };
  spec: {
    type?: string;
    servers?: string[];
    baseDN?: string;
  };
  status?: { conditions?: Array<{ type: string; status: string; reason?: string }> };
};

export type ZSnapshot = {
  metadata: { name: string };
  spec: { pvcName?: string; snapshotClassName?: string };
  status?: { phase?: string; message?: string; volumeSnapshotName?: string };
};

export type DiskInfo = {
  id: string;
  path: string;
  sizeBytes?: number;
  model?: string;
  rotational?: boolean;
};

export type DiskInventory = {
  disks: DiskInfo[];
  updated?: string;
  count?: number;
};

export type Overview = {
  pools: ZPool[];
  datasets: ZDataset[];
  shares: NASShare[];
  directories: NASDirectory[];
};

const apiBase = (import.meta.env.VITE_API_BASE as string | undefined) ?? "";

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${apiBase}${path}`, {
    headers: { "Content-Type": "application/json" },
    ...init
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || `Request failed: ${res.status}`);
  }
  if (res.status === 204) {
    return undefined as T;
  }
  const text = await res.text();
  if (!text) {
    return undefined as T;
  }
  return JSON.parse(text) as T;
}

export function fetchOverview(): Promise<Overview> {
  return request<Overview>("/v1/overview");
}

export function listDisks(): Promise<DiskInventory> {
  return request<DiskInventory>("/v1/disks");
}

type CreateRequest<T> = {
  name: string;
  namespace?: string;
  spec: T;
};

export function listZPools(): Promise<ZPool[]> {
  return request<ZPool[]>("/v1/zpools");
}

export function upsertZPool(name: string, spec: ZPool["spec"]): Promise<ZPool> {
  return request<ZPool>("/v1/zpools", {
    method: "POST",
    body: JSON.stringify({ name, spec } satisfies CreateRequest<ZPool["spec"]>)
  });
}

export function deleteZPool(name: string): Promise<void> {
  return request<void>(`/v1/zpools/${name}`, { method: "DELETE" });
}

export function listZDatasets(): Promise<ZDataset[]> {
  return request<ZDataset[]>("/v1/zdatasets");
}

export function upsertZDataset(name: string, spec: ZDataset["spec"]): Promise<ZDataset> {
  return request<ZDataset>("/v1/zdatasets", {
    method: "POST",
    body: JSON.stringify({ name, spec } satisfies CreateRequest<ZDataset["spec"]>)
  });
}

export function deleteZDataset(name: string): Promise<void> {
  return request<void>(`/v1/zdatasets/${name}`, { method: "DELETE" });
}

export function listNASShares(): Promise<NASShare[]> {
  return request<NASShare[]>("/v1/nasshares");
}

export function upsertNASShare(name: string, spec: NASShare["spec"]): Promise<NASShare> {
  return request<NASShare>("/v1/nasshares", {
    method: "POST",
    body: JSON.stringify({ name, spec } satisfies CreateRequest<NASShare["spec"]>)
  });
}

export function deleteNASShare(name: string): Promise<void> {
  return request<void>(`/v1/nasshares/${name}`, { method: "DELETE" });
}

export function listNASDirectories(): Promise<NASDirectory[]> {
  return request<NASDirectory[]>("/v1/nasdirectories");
}

export function upsertNASDirectory(name: string, spec: NASDirectory["spec"]): Promise<NASDirectory> {
  return request<NASDirectory>("/v1/nasdirectories", {
    method: "POST",
    body: JSON.stringify({ name, spec } satisfies CreateRequest<NASDirectory["spec"]>)
  });
}

export function deleteNASDirectory(name: string): Promise<void> {
  return request<void>(`/v1/nasdirectories/${name}`, { method: "DELETE" });
}

export function listZSnapshots(): Promise<ZSnapshot[]> {
  return request<ZSnapshot[]>("/v1/zsnapshots");
}

export function upsertZSnapshot(name: string, spec: ZSnapshot["spec"]): Promise<ZSnapshot> {
  return request<ZSnapshot>("/v1/zsnapshots", {
    method: "POST",
    body: JSON.stringify({ name, spec } satisfies CreateRequest<ZSnapshot["spec"]>)
  });
}

export function deleteZSnapshot(name: string): Promise<void> {
  return request<void>(`/v1/zsnapshots/${name}`, { method: "DELETE" });
}
