export type ZPool = {
  metadata: { name: string };
  spec: { poolName?: string };
  status?: { phase?: string; message?: string };
};

export type ZDataset = {
  metadata: { name: string };
  spec: { datasetName?: string; mountpoint?: string };
  status?: { phase?: string; message?: string };
};

export type NASShare = {
  metadata: { name: string };
  spec: {
    protocol?: string;
    shareName?: string;
    datasetName?: string;
    mountPath?: string;
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
  return (await res.json()) as T;
}

export function fetchOverview(): Promise<Overview> {
  return request<Overview>("/v1/overview");
}
