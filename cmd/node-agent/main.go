package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// -----------------
// API payloads
// -----------------

type Disk struct {
	ID         string `json:"id"`
	Path       string `json:"path"`
	SizeBytes  int64  `json:"sizeBytes,omitempty"`
	Model      string `json:"model,omitempty"`
	Rotational *bool  `json:"rotational,omitempty"`
}

type DiskList struct {
	Disks []Disk `json:"disks"`
}

type DiskCacheStatus struct {
	Updated string `json:"updated,omitempty"`
	Count   int    `json:"count"`
}

type SmartResponse struct {
	OK     bool           `json:"ok"`
	Device string         `json:"device,omitempty"`
	Output string         `json:"output,omitempty"`
	JSON   map[string]any `json:"json,omitempty"`
	Error  string         `json:"error,omitempty"`
}

type SmartAllResponse struct {
	OK    bool            `json:"ok"`
	Items []SmartResponse `json:"items,omitempty"`
	Error string          `json:"error,omitempty"`
}

type NFSExportRequest struct {
	Path    string   `json:"path"`
	Clients []string `json:"clients,omitempty"`
	Options string   `json:"options,omitempty"`
}

type NFSExportResponse struct {
	OK     bool   `json:"ok"`
	Path   string `json:"path,omitempty"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

type NFSExportListResponse struct {
	OK    bool     `json:"ok"`
	Items []string `json:"items,omitempty"`
	Error string   `json:"error,omitempty"`
}

type NFSSSSDApplyRequest struct {
	Config   string `json:"config"`
	CABundle string `json:"caBundle,omitempty"`
}

type NFSSSSDApplyResponse struct {
	OK     bool   `json:"ok"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

var diskCache struct {
	mu      sync.RWMutex
	disks   []Disk
	updated time.Time
}

var diskRefreshCh = make(chan struct{}, 1)

const nfsExportsPath = "/etc/exports.d/nas.exports"

// Legacy pool create (kept for backward compatibility)
type ZPoolCreateRequest struct {
	PoolName string      `json:"poolName"`
	VdevType string      `json:"vdevType"`
	Vdevs    []ZPoolVdev `json:"vdevs"`
}

type ZPoolVdev struct {
	Type    string   `json:"type"`
	Devices []string `json:"devices"`
}

// New pool create API (mirrors the newer scaffold)
type ZPoolCreateRequestV2 struct {
	Name       string            `json:"name"`
	Layout     string            `json:"layout"`
	Devices    []string          `json:"devices"`
	Properties map[string]string `json:"properties,omitempty"`
}

type ZPoolOpResponse struct {
	OK     bool   `json:"ok"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

type ZPoolListResponse struct {
	OK    bool     `json:"ok"`
	Error string   `json:"error,omitempty"`
	Items []string `json:"items,omitempty"`
}

type ZPoolStatusResponse struct {
	OK     bool         `json:"ok"`
	Pool   *PoolStatus  `json:"pool,omitempty"`
	Pools  []PoolStatus `json:"pools,omitempty"`
	Output string       `json:"output,omitempty"`
	Error  string       `json:"error,omitempty"`
}

type PoolStatus struct {
	Name   string     `json:"name"`
	State  string     `json:"state,omitempty"`
	Status string     `json:"status,omitempty"`
	Action string     `json:"action,omitempty"`
	Scan   string     `json:"scan,omitempty"`
	Errors string     `json:"errors,omitempty"`
	Vdevs  []PoolVdev `json:"vdevs,omitempty"`
}

type PoolVdev struct {
	Name  string `json:"name"`
	State string `json:"state,omitempty"`
	Read  uint64 `json:"read,omitempty"`
	Write uint64 `json:"write,omitempty"`
	Cksum uint64 `json:"cksum,omitempty"`
}

type ZDatasetEnsureRequest struct {
	Dataset    string            `json:"dataset"`              // e.g. "tank/data" (legacy)
	Mountpoint string            `json:"mountpoint,omitempty"` // optional
	Properties map[string]string `json:"properties,omitempty"`
}

type ZDatasetEnsureRequestV2 struct {
	Pool       string            `json:"pool"`
	Name       string            `json:"name"`       // e.g. "data"
	Mountpoint string            `json:"mountpoint"` // optional
	Properties map[string]string `json:"properties,omitempty"`
}

type ZDatasetMountRequest struct {
	Dataset    string `json:"dataset"`
	Mountpoint string `json:"mountpoint,omitempty"`
	Mode       string `json:"mode,omitempty"`
	Recursive  bool   `json:"recursive,omitempty"`
}

type ZDatasetStatusResponse struct {
	OK     bool   `json:"ok"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

type ZPoolDestroyRequest struct {
	PoolName string `json:"poolName"`
}

type ZSnapshotCreateRequest struct {
	Dataset   string `json:"dataset"`
	Name      string `json:"name"`
	Recursive bool   `json:"recursive,omitempty"`
}

type ZSnapshotDestroyRequest struct {
	Snapshot string `json:"snapshot"`
}

type ZSnapshotListResponse struct {
	OK    bool     `json:"ok"`
	Error string   `json:"error,omitempty"`
	Items []string `json:"items,omitempty"`
}

// -----------------
// Server
// -----------------

func main() {
	var addr string
	flag.StringVar(&addr, "addr", ":9808", "listen address")
	flag.Parse()

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Disk discovery (used by UI / operator to pick stable paths)
	mux.HandleFunc("/v1/disks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		refresh := strings.TrimSpace(r.URL.Query().Get("refresh"))
		if refresh == "1" || strings.EqualFold(refresh, "true") {
			refreshDiskCache()
		} else if len(getDiskCache()) == 0 {
			refreshDiskCache()
		}
		out := DiskList{Disks: getDiskCache()}
		writeJSON(w, http.StatusOK, out)
	})

	mux.HandleFunc("/v1/disks/updated", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		status := getDiskCacheStatus()
		writeJSON(w, http.StatusOK, status)
	})

	mux.HandleFunc("/v1/disks/smart", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if _, err := exec.LookPath("smartctl"); err != nil {
			writeJSON(w, http.StatusInternalServerError, SmartResponse{OK: false, Error: "smartctl not found"})
			return
		}
		timeout := parseSmartTimeout(r.URL.Query().Get("timeout"))
		useJSON := !isFalse(r.URL.Query().Get("json"))
		device := strings.TrimSpace(r.URL.Query().Get("device"))
		if device == "" {
			device = strings.TrimSpace(r.URL.Query().Get("id"))
		}
		if device == "" {
			all := strings.TrimSpace(r.URL.Query().Get("all"))
			if all != "" && all != "0" && !strings.EqualFold(all, "false") {
				refresh := strings.TrimSpace(r.URL.Query().Get("refresh"))
				if refresh == "1" || strings.EqualFold(refresh, "true") {
					refreshDiskCache()
				} else if len(getDiskCache()) == 0 {
					refreshDiskCache()
				}
				var items []SmartResponse
				for _, d := range getDiskCache() {
					resp := probeSmart(d.Path, timeout, useJSON)
					items = append(items, resp)
				}
				writeJSON(w, http.StatusOK, SmartAllResponse{OK: true, Items: items})
				return
			}
			writeJSON(w, http.StatusBadRequest, SmartResponse{OK: false, Error: "device or all=1 required"})
			return
		}
		path := resolveDiskPath(device)
		if path == "" {
			writeJSON(w, http.StatusBadRequest, SmartResponse{OK: false, Error: "device not found"})
			return
		}
		resp := probeSmart(path, timeout, useJSON)
		writeJSON(w, http.StatusOK, resp)
	})

	// ----- NFS exports -----
	mux.HandleFunc("/v1/nfs/exports", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		items, err := listNFSExports()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, NFSExportListResponse{OK: false, Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, NFSExportListResponse{OK: true, Items: items})
	})

	mux.HandleFunc("/v1/nfs/sssd/apply", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req NFSSSSDApplyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, NFSSSSDApplyResponse{OK: false, Error: "invalid json"})
			return
		}
		out, err := applyNFSSSSDConfig(req.Config, req.CABundle)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, NFSSSSDApplyResponse{OK: false, Output: out, Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, NFSSSSDApplyResponse{OK: true, Output: out})
	})

	mux.HandleFunc("/v1/nfs/export/ensure", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req NFSExportRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, NFSExportResponse{OK: false, Error: "invalid json"})
			return
		}
		if strings.TrimSpace(req.Path) == "" {
			writeJSON(w, http.StatusBadRequest, NFSExportResponse{OK: false, Error: "path required"})
			return
		}
		out, err := ensureNFSExport(req.Path, req.Clients, req.Options)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, NFSExportResponse{OK: false, Path: req.Path, Output: out, Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, NFSExportResponse{OK: true, Path: req.Path, Output: out})
	})

	mux.HandleFunc("/v1/nfs/export/delete", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req NFSExportRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, NFSExportResponse{OK: false, Error: "invalid json"})
			return
		}
		if strings.TrimSpace(req.Path) == "" {
			writeJSON(w, http.StatusBadRequest, NFSExportResponse{OK: false, Error: "path required"})
			return
		}
		out, err := deleteNFSExport(req.Path)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, NFSExportResponse{OK: false, Path: req.Path, Output: out, Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, NFSExportResponse{OK: true, Path: req.Path, Output: out})
	})

	// ----- Pools -----
	// legacy list
	mux.HandleFunc("/v1/zfs/pool/list", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		items, raw, err := listZPoolNames()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ZPoolListResponse{OK: false, Error: err.Error()})
			return
		}
		_ = raw
		writeJSON(w, http.StatusOK, ZPoolListResponse{OK: true, Items: items})
	})

	// legacy create
	mux.HandleFunc("/v1/zfs/pool/create", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req ZPoolCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ZPoolOpResponse{OK: false, Error: "invalid json"})
			return
		}
		if strings.TrimSpace(req.PoolName) == "" {
			writeJSON(w, http.StatusBadRequest, ZPoolOpResponse{OK: false, Error: "poolName required"})
			return
		}
		layout := strings.TrimSpace(strings.ToLower(req.VdevType))
		var devices []string
		for _, v := range req.Vdevs {
			for _, d := range v.Devices {
				devices = append(devices, d)
			}
		}
		if len(devices) == 0 {
			writeJSON(w, http.StatusBadRequest, ZPoolOpResponse{OK: false, Error: "no devices provided"})
			return
		}
		out, err := createPoolV2(ZPoolCreateRequestV2{Name: req.PoolName, Layout: layout, Devices: devices, Properties: map[string]string{"ashift": "12"}})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ZPoolOpResponse{OK: false, Output: out, Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, ZPoolOpResponse{OK: true, Output: out})
	})

	// legacy destroy
	mux.HandleFunc("/v1/zfs/pool/destroy", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req ZPoolDestroyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ZPoolOpResponse{OK: false, Error: "invalid json"})
			return
		}
		if strings.TrimSpace(req.PoolName) == "" {
			writeJSON(w, http.StatusBadRequest, ZPoolOpResponse{OK: false, Error: "poolName required"})
			return
		}
		out, err := runCmdCombined(r.Context(), 120*time.Second, "zpool", "destroy", "-f", req.PoolName)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ZPoolOpResponse{OK: false, Output: out, Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, ZPoolOpResponse{OK: true, Output: out})
	})

	// V2 create
	mux.HandleFunc("/v1/zfs/zpools/create", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req ZPoolCreateRequestV2
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ZPoolOpResponse{OK: false, Error: "invalid json"})
			return
		}
		if err := validateZpoolCreateV2(req); err != nil {
			writeJSON(w, http.StatusBadRequest, ZPoolOpResponse{OK: false, Error: err.Error()})
			return
		}

		out, err := createPoolV2(req)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ZPoolOpResponse{OK: false, Output: out, Error: err.Error()})
			return
		}
		st, raw, stErr := getZPoolStatus(req.Name)
		if stErr != nil {
			writeJSON(w, http.StatusOK, ZPoolStatusResponse{OK: true, Output: out + "\n" + raw})
			return
		}
		writeJSON(w, http.StatusOK, ZPoolStatusResponse{OK: true, Pool: &st, Output: out + "\n" + raw})
	})

	// V2 status (single or all)
	mux.HandleFunc("/v1/zfs/zpools/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := strings.TrimSpace(r.URL.Query().Get("name"))
		if name != "" {
			st, raw, err := getZPoolStatus(name)
			if err != nil {
				writeJSON(w, http.StatusNotFound, ZPoolStatusResponse{OK: false, Output: raw, Error: err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, ZPoolStatusResponse{OK: true, Pool: &st, Output: raw})
			return
		}
		items, raw, err := listZPoolNames()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ZPoolStatusResponse{OK: false, Output: raw, Error: err.Error()})
			return
		}
		var pools []PoolStatus
		var rawAll strings.Builder
		for _, p := range items {
			st, out, e := getZPoolStatus(p)
			rawAll.WriteString("=== " + p + " ===\n" + out + "\n")
			if e != nil {
				pools = append(pools, PoolStatus{Name: p, State: "UNKNOWN", Status: e.Error()})
				continue
			}
			pools = append(pools, st)
		}
		writeJSON(w, http.StatusOK, ZPoolStatusResponse{OK: true, Pools: pools, Output: rawAll.String()})
	})

	// ----- Datasets -----
	// legacy ensure
	mux.HandleFunc("/v1/zfs/dataset/ensure", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req ZDatasetEnsureRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ZDatasetStatusResponse{OK: false, Error: "invalid json"})
			return
		}
		if strings.TrimSpace(req.Dataset) == "" {
			writeJSON(w, http.StatusBadRequest, ZDatasetStatusResponse{OK: false, Error: "dataset required"})
			return
		}
		out, err := ensureDataset(req.Dataset, req.Mountpoint, req.Properties)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ZDatasetStatusResponse{OK: false, Output: out, Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, ZDatasetStatusResponse{OK: true, Output: out})
	})

	// v2 ensure
	mux.HandleFunc("/v1/zfs/zdatasets/ensure", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req ZDatasetEnsureRequestV2
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ZDatasetStatusResponse{OK: false, Error: "invalid json"})
			return
		}
		if strings.TrimSpace(req.Pool) == "" || strings.TrimSpace(req.Name) == "" {
			writeJSON(w, http.StatusBadRequest, ZDatasetStatusResponse{OK: false, Error: "pool and name are required"})
			return
		}
		full := strings.TrimSpace(req.Pool) + "/" + strings.TrimSpace(req.Name)
		out, err := ensureDataset(full, req.Mountpoint, req.Properties)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ZDatasetStatusResponse{OK: false, Output: out, Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, ZDatasetStatusResponse{OK: true, Output: out})
	})

	mux.HandleFunc("/v1/zfs/dataset/mount", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req ZDatasetMountRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ZDatasetStatusResponse{OK: false, Error: "invalid json"})
			return
		}
		if strings.TrimSpace(req.Dataset) == "" {
			writeJSON(w, http.StatusBadRequest, ZDatasetStatusResponse{OK: false, Error: "dataset required"})
			return
		}
		mode := strings.TrimSpace(req.Mode)
		if mode != "" && !isOctalMode(mode) {
			writeJSON(w, http.StatusBadRequest, ZDatasetStatusResponse{OK: false, Error: "mode must be octal (e.g. 0777)"})
			return
		}
		out, err := ensureDatasetMounted(req.Dataset, req.Mountpoint, mode, req.Recursive)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ZDatasetStatusResponse{OK: false, Output: out, Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, ZDatasetStatusResponse{OK: true, Output: out})
	})

	// ----- Snapshots -----
	mux.HandleFunc("/v1/zfs/snapshot/list", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		ds := strings.TrimSpace(r.URL.Query().Get("dataset"))
		items, out, err := listSnapshotNames(ds)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ZSnapshotListResponse{OK: false, Error: err.Error()})
			return
		}
		_ = out
		writeJSON(w, http.StatusOK, ZSnapshotListResponse{OK: true, Items: items})
	})

	mux.HandleFunc("/v1/zfs/snapshot/create", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req ZSnapshotCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ZPoolOpResponse{OK: false, Error: "invalid json"})
			return
		}
		if strings.TrimSpace(req.Dataset) == "" || strings.TrimSpace(req.Name) == "" {
			writeJSON(w, http.StatusBadRequest, ZPoolOpResponse{OK: false, Error: "dataset and name required"})
			return
		}
		snap := strings.TrimSpace(req.Dataset) + "@" + strings.TrimSpace(req.Name)
		args := []string{"snapshot"}
		if req.Recursive {
			args = append(args, "-r")
		}
		args = append(args, snap)
		out, err := runCmdCombined(r.Context(), 120*time.Second, "zfs", args...)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ZPoolOpResponse{OK: false, Output: out, Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, ZPoolOpResponse{OK: true, Output: out})
	})

	mux.HandleFunc("/v1/zfs/snapshot/destroy", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req ZSnapshotDestroyRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, ZPoolOpResponse{OK: false, Error: "invalid json"})
			return
		}
		if strings.TrimSpace(req.Snapshot) == "" {
			writeJSON(w, http.StatusBadRequest, ZPoolOpResponse{OK: false, Error: "snapshot required"})
			return
		}
		out, err := runCmdCombined(r.Context(), 120*time.Second, "zfs", "destroy", strings.TrimSpace(req.Snapshot))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ZPoolOpResponse{OK: false, Output: out, Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, ZPoolOpResponse{OK: true, Output: out})
	})

	refreshDiskCache()
	go startDiskRefreshLoop(context.Background())
	go startUdevMonitor(context.Background())

	server := &http.Server{Addr: addr, Handler: mux}
	log.Printf("node-agent listening on %s", addr)
	log.Fatal(server.ListenAndServe())
}

// -----------------
// Helpers
// -----------------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func validateZpoolCreateV2(req ZPoolCreateRequestV2) error {
	if strings.TrimSpace(req.Name) == "" || strings.ContainsAny(req.Name, " \t/") {
		return errors.New("invalid pool name")
	}
	if len(req.Devices) == 0 {
		return errors.New("devices required")
	}
	return nil
}

func runCmdCombined(ctx context.Context, timeout time.Duration, name string, args ...string) (string, error) {
	c, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(c, name, args...)
	b, err := cmd.CombinedOutput()
	out := string(b)
	if c.Err() == context.DeadlineExceeded {
		return out, fmt.Errorf("command timed out: %s %s", name, strings.Join(args, " "))
	}
	return out, err
}

func udevSettle() {
	_ = exec.Command("udevadm", "settle", "--timeout=5").Run()
}

func pick(m map[string]string, k, def string) string {
	if m == nil {
		return def
	}
	if v, ok := m[k]; ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// -----------------
// Pool operations
// -----------------

func normalizeDevicePath(d string) string {
	d = strings.TrimSpace(d)
	if d == "" {
		return d
	}
	if strings.HasPrefix(d, "/dev/") {
		return d
	}
	byID := "/dev/disk/by-id/" + d
	byPath := "/dev/disk/by-path/" + d
	if fileExists(byID) {
		return byID
	}
	if fileExists(byPath) {
		return byPath
	}
	return "/dev/" + d
}

func createPoolV2(req ZPoolCreateRequestV2) (string, error) {
	props := req.Properties
	ashift := pick(props, "ashift", "12")

	devs := make([]string, 0, len(req.Devices))
	for _, d := range req.Devices {
		devs = append(devs, normalizeDevicePath(d))
	}

	prepared, err := prepareVdevs(devs)
	if err != nil {
		return "", err
	}

	args := []string{"create", "-f", "-o", "ashift=" + ashift, "-m", "none"}
	switch strings.ToLower(strings.TrimSpace(req.Layout)) {
	case "mirror":
		args = append(args, req.Name, "mirror")
	case "raidz1":
		args = append(args, req.Name, "raidz1")
	case "raidz2":
		args = append(args, req.Name, "raidz2")
	case "stripe", "":
		args = append(args, req.Name)
	default:
		return "", fmt.Errorf("unsupported layout: %s", req.Layout)
	}
	args = append(args, prepared...)

	udevSettle()
	log.Printf("zpool cmd: zpool %s", strings.Join(args, " "))
	log.Printf("zpool vdevs: %v", prepared)

	out, err := runCmdCombined(context.Background(), 180*time.Second, "zpool", args...)
	if err != nil {
		// One retry after settle; device events can be racy in VMs
		udevSettle()
		out2, err2 := runCmdCombined(context.Background(), 180*time.Second, "zpool", args...)
		if err2 != nil {
			return out + "\n" + out2, err2
		}
		out = out2
	}

	if pick(props, "autoexpand", "") == "on" {
		_, _ = runCmdCombined(context.Background(), 30*time.Second, "zpool", "set", "autoexpand=on", req.Name)
	}

	// Normalize host ownership and boot import determinism.
	normOut, normErr := normalizePoolAfterCreate(req.Name)
	combined := strings.TrimSpace(out + "\n" + normOut)
	if normErr != nil {
		return combined, normErr
	}

	return combined, nil
}

func normalizePoolAfterCreate(pool string) (string, error) {
	var b strings.Builder

	out, err := runCmdCombined(context.Background(), 60*time.Second, "zpool", "export", pool)
	b.WriteString("zpool export:\n" + out + "\n")
	if err != nil {
		return b.String(), fmt.Errorf("zpool export failed: %w", err)
	}

	udevSettle()
	out, err = runCmdCombined(context.Background(), 60*time.Second, "zpool", "import", "-d", "/dev/disk/by-id", "-d", "/dev/disk/by-path", pool)
	b.WriteString("zpool import:\n" + out + "\n")
	if err != nil {
		return b.String(), fmt.Errorf("zpool import failed: %w", err)
	}

	out, err = runCmdCombined(context.Background(), 30*time.Second, "zpool", "set", "cachefile=/etc/zfs/zpool.cache", pool)
	b.WriteString("zpool set cachefile:\n" + out + "\n")
	if err != nil {
		return b.String(), fmt.Errorf("zpool set cachefile failed: %w", err)
	}

	return b.String(), nil
}

func prepareVdevs(devs []string) ([]string, error) {
	out := make([]string, 0, len(devs))
	for _, d := range devs {
		if strings.TrimSpace(d) == "" {
			continue
		}
		rd, err := filepath.EvalSymlinks(d)
		if err != nil {
			rd = d
		}
		if isWholeDisk(rd) {
			if err := ensureSingleZfsPartition(rd); err != nil {
				return nil, fmt.Errorf("prepare %s: %w", rd, err)
			}
			out = append(out, rd+"1")
			continue
		}
		out = append(out, rd)
	}
	return out, nil
}

func isWholeDisk(p string) bool {
	if !strings.HasPrefix(p, "/dev/sd") {
		return false
	}
	last := p[len(p)-1]
	return last >= 'a' && last <= 'z'
}

func ensureSingleZfsPartition(disk string) error {
	// If partition already exists, do not re-zap.
	if fileExists(disk + "1") {
		return nil
	}

	// Data destructive. Only safe for dedicated data disks.
	_, _ = runCmdCombined(context.Background(), 30*time.Second, "wipefs", "-a", disk)
	_, _ = runCmdCombined(context.Background(), 30*time.Second, "sgdisk", "--zap-all", disk)

	if _, err := runCmdCombined(context.Background(), 60*time.Second, "sgdisk", "-n", "1:1MiB:0", "-t", "1:BF01", "-c", "1:mnemosyne-zfs", disk); err != nil {
		return err
	}

	_, _ = runCmdCombined(context.Background(), 15*time.Second, "partprobe", disk)
	udevSettle()
	if !fileExists(disk + "1") {
		return fmt.Errorf("partition %s1 not found after partitioning", disk)
	}
	return nil
}

func listZPoolNames() ([]string, string, error) {
	out, err := runCmdCombined(context.Background(), 30*time.Second, "zpool", "list", "-H", "-o", "name")
	if err != nil {
		return nil, out, fmt.Errorf("zpool list failed: %w", err)
	}
	return splitLines(out), out, nil
}

func listSnapshotNames(dataset string) ([]string, string, error) {
	args := []string{"list", "-H", "-t", "snapshot", "-o", "name"}
	if dataset != "" {
		args = append(args, "-r", dataset)
	}
	out, err := runCmdCombined(context.Background(), 30*time.Second, "zfs", args...)
	if err != nil {
		return nil, out, fmt.Errorf("zfs snapshot list failed: %w", err)
	}
	return splitLines(out), out, nil
}

func getZPoolStatus(pool string) (PoolStatus, string, error) {
	raw, err := runCmdCombined(context.Background(), 30*time.Second, "zpool", "status", pool)
	if err != nil {
		return PoolStatus{Name: pool}, raw, fmt.Errorf("zpool status failed: %w", err)
	}
	st := parseZPoolStatus(raw)
	st.Name = pool
	return st, raw, nil
}

func parseZPoolStatus(raw string) PoolStatus {
	var st PoolStatus
	lines := strings.Split(raw, "\n")
	inConfig := false
	for _, ln := range lines {
		l := strings.TrimRight(ln, " \t")
		s := strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(s, "state:"):
			st.State = strings.TrimSpace(strings.TrimPrefix(s, "state:"))
		case strings.HasPrefix(s, "status:"):
			st.Status = strings.TrimSpace(strings.TrimPrefix(s, "status:"))
		case strings.HasPrefix(s, "action:"):
			st.Action = strings.TrimSpace(strings.TrimPrefix(s, "action:"))
		case strings.HasPrefix(s, "scan:"):
			st.Scan = strings.TrimSpace(strings.TrimPrefix(s, "scan:"))
		case strings.HasPrefix(s, "errors:"):
			st.Errors = strings.TrimSpace(strings.TrimPrefix(s, "errors:"))
		case strings.HasPrefix(s, "config:"):
			inConfig = true
		default:
			if !inConfig {
				continue
			}
			fields := strings.Fields(l)
			if len(fields) < 2 {
				continue
			}
			if fields[0] == "NAME" && fields[1] == "STATE" {
				continue
			}
			v := PoolVdev{Name: fields[0], State: fields[1]}
			if len(fields) >= 5 {
				v.Read = parseUint(fields[2])
				v.Write = parseUint(fields[3])
				v.Cksum = parseUint(fields[4])
			}
			st.Vdevs = append(st.Vdevs, v)
		}
	}
	return st
}

func parseUint(s string) uint64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
	}
	var n uint64
	for _, r := range s {
		n = n*10 + uint64(r-'0')
	}
	return n
}

func isOctalMode(s string) bool {
	if len(s) < 3 || len(s) > 4 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '7' {
			return false
		}
	}
	return true
}

// -----------------
// Dataset operations
// -----------------

func ensureDataset(full string, mountpoint string, props map[string]string) (string, error) {
	full = strings.TrimSpace(full)
	if full == "" {
		return "", errors.New("dataset empty")
	}

	// Attempt create (idempotent)
	args := []string{"create"}
	if mp := strings.TrimSpace(mountpoint); mp != "" {
		args = append(args, "-o", "mountpoint="+mp)
	}
	for k, v := range props {
		k = strings.TrimSpace(strings.ToLower(k))
		v = strings.TrimSpace(v)
		if k == "" || v == "" {
			continue
		}
		args = append(args, "-o", k+"="+v)
	}
	args = append(args, full)

	out, err := runCmdCombined(context.Background(), 60*time.Second, "zfs", args...)
	if err != nil {
		lo := strings.ToLower(out)
		if !(strings.Contains(lo, "already exists") || strings.Contains(lo, "dataset already exists")) {
			return out, err
		}
	}

	// Enforce properties even if existed.
	for k, v := range props {
		k = strings.TrimSpace(strings.ToLower(k))
		v = strings.TrimSpace(v)
		if k == "" || v == "" {
			continue
		}
		_, _ = runCmdCombined(context.Background(), 30*time.Second, "zfs", "set", k+"="+v, full)
	}
	if mp := strings.TrimSpace(mountpoint); mp != "" {
		_, _ = runCmdCombined(context.Background(), 30*time.Second, "zfs", "set", "mountpoint="+mp, full)
	}
	return out, nil
}

func ensureDatasetMounted(full string, mountpoint string, mode string, recursive bool) (string, error) {
	full = strings.TrimSpace(full)
	if full == "" {
		return "", errors.New("dataset empty")
	}

	if mp := strings.TrimSpace(mountpoint); mp != "" {
		_, _ = runCmdCombined(context.Background(), 30*time.Second, "zfs", "set", "mountpoint="+mp, full)
	}

	out, err := runCmdCombined(context.Background(), 30*time.Second, "zfs", "get", "-H", "-o", "value", "mounted", full)
	if err != nil {
		return out, fmt.Errorf("zfs get mounted failed: %w", err)
	}
	if strings.TrimSpace(out) == "yes" {
		return ensureMountPerms(full, mountpoint, mode, recursive, out)
	}

	out, err = runCmdCombined(context.Background(), 60*time.Second, "zfs", "mount", full)
	if err != nil {
		lo := strings.ToLower(out)
		if strings.Contains(lo, "already mounted") {
			return ensureMountPerms(full, mountpoint, mode, recursive, out)
		}
		return out, err
	}
	return ensureMountPerms(full, mountpoint, mode, recursive, out)
}

func ensureMountPerms(dataset string, mountpoint string, mode string, recursive bool, out string) (string, error) {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		return out, nil
	}
	mp := strings.TrimSpace(mountpoint)
	if mp == "" {
		var err error
		mp, err = getDatasetMountpoint(dataset)
		if err != nil {
			return out, err
		}
	}
	if mp == "" || mp == "none" || mp == "-" || mp == "legacy" {
		return out, fmt.Errorf("mountpoint not available for %s", dataset)
	}
	args := []string{}
	if recursive {
		args = append(args, "-R")
	}
	args = append(args, mode, mp)
	out2, err := runCmdCombined(context.Background(), 60*time.Second, "chmod", args...)
	if err != nil {
		return out2, err
	}
	return out2, nil
}

func getDatasetMountpoint(full string) (string, error) {
	out, err := runCmdCombined(context.Background(), 30*time.Second, "zfs", "get", "-H", "-o", "value", "mountpoint", full)
	if err != nil {
		return out, fmt.Errorf("zfs get mountpoint failed: %w", err)
	}
	return strings.TrimSpace(out), nil
}

// -----------------
// Disk discovery
// -----------------

func refreshDiskCache() {
	udevSettle()
	disks := discoverDisks()
	diskCache.mu.Lock()
	diskCache.disks = disks
	diskCache.updated = time.Now().UTC()
	diskCache.mu.Unlock()
}

func getDiskCache() []Disk {
	diskCache.mu.RLock()
	defer diskCache.mu.RUnlock()
	out := make([]Disk, len(diskCache.disks))
	copy(out, diskCache.disks)
	return out
}

func getDiskCacheStatus() DiskCacheStatus {
	diskCache.mu.RLock()
	defer diskCache.mu.RUnlock()
	status := DiskCacheStatus{Count: len(diskCache.disks)}
	if !diskCache.updated.IsZero() {
		status.Updated = diskCache.updated.Format(time.RFC3339)
	}
	return status
}

func queueDiskRefresh() {
	select {
	case diskRefreshCh <- struct{}{}:
	default:
	}
}

func startDiskRefreshLoop(ctx context.Context) {
	for {
		select {
		case <-diskRefreshCh:
			time.Sleep(500 * time.Millisecond)
			for len(diskRefreshCh) > 0 {
				<-diskRefreshCh
			}
			refreshDiskCache()
		case <-ctx.Done():
			return
		}
	}
}

func startUdevMonitor(ctx context.Context) {
	if _, err := exec.LookPath("udevadm"); err != nil {
		log.Printf("udevadm not found; disk refresh on udev events disabled: %v", err)
		return
	}
	cmd := exec.CommandContext(ctx, "udevadm", "monitor", "--udev", "--subsystem-match=block")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("udev monitor stdout pipe failed: %v", err)
		return
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		log.Printf("udev monitor start failed: %v", err)
		return
	}
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		if isUdevDiskEvent(scanner.Text()) {
			queueDiskRefresh()
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("udev monitor error: %v", err)
	}
	_ = cmd.Wait()
}

func isUdevDiskEvent(line string) bool {
	if !strings.HasPrefix(line, "UDEV") {
		return false
	}
	return strings.Contains(line, " add ") || strings.Contains(line, " remove ") || strings.Contains(line, " change ")
}

func parseSmartTimeout(raw string) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 60 * time.Second
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 60 * time.Second
	}
	if n < 5 {
		n = 5
	}
	if n > 300 {
		n = 300
	}
	return time.Duration(n) * time.Second
}

func isFalse(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "0", "false", "no":
		return true
	default:
		return false
	}
}

func resolveDiskPath(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if strings.HasPrefix(ref, "/dev/") {
		if fileExists(ref) {
			return ref
		}
		return ""
	}
	byID := "/dev/disk/by-id/" + ref
	if fileExists(byID) {
		return byID
	}
	byPath := "/dev/disk/by-path/" + ref
	if fileExists(byPath) {
		return byPath
	}
	dev := "/dev/" + ref
	if fileExists(dev) {
		return dev
	}
	return ""
}

func probeSmart(device string, timeout time.Duration, useJSON bool) SmartResponse {
	out, parsed, err := runSmartctl(device, timeout, useJSON)
	resp := SmartResponse{
		OK:     out != "",
		Device: device,
		Output: out,
		JSON:   parsed,
	}
	if err != nil && out == "" {
		resp.OK = false
		resp.Error = err.Error()
	} else if err != nil {
		resp.Error = err.Error()
	}
	return resp
}

func runSmartctl(device string, timeout time.Duration, useJSON bool) (string, map[string]any, error) {
	if !useJSON {
		out, err := runCmdCombined(context.Background(), timeout, "smartctl", "-a", device)
		return out, nil, err
	}
	out, err := runCmdCombined(context.Background(), timeout, "smartctl", "-a", "-j", device)
	parsed := map[string]any{}
	trim := strings.TrimSpace(out)
	if strings.HasPrefix(trim, "{") {
		if err := json.Unmarshal([]byte(trim), &parsed); err == nil {
			return out, parsed, err
		}
	}
	// Fallback to text output if JSON is unavailable.
	out2, err2 := runCmdCombined(context.Background(), timeout, "smartctl", "-a", device)
	if out2 != "" {
		out = out2
		err = err2
	}
	return out, nil, err
}

func listNFSExports() ([]string, error) {
	b, err := os.ReadFile(nfsExportsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	lines := strings.Split(string(b), "\n")
	var out []string
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		out = append(out, trim)
	}
	return out, nil
}

func ensureNFSExport(path string, clients []string, options string) (string, error) {
	if _, err := exec.LookPath("exportfs"); err != nil {
		return "", fmt.Errorf("exportfs not found")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("path empty")
	}
	if len(clients) == 0 {
		clients = []string{"*"}
	}
	options = strings.TrimSpace(options)
	if options == "" {
		options = "rw,sync,no_subtree_check"
	}
	entry := buildNFSExportLine(path, clients, options)
	lines, changed := upsertNFSExportLine(path, entry)
	if changed {
		if err := writeNFSExports(lines); err != nil {
			return "", err
		}
	}
	out, err := runCmdCombined(context.Background(), 30*time.Second, "exportfs", "-ra")
	if err != nil {
		return out, fmt.Errorf("exportfs reload failed: %w", err)
	}
	return out, nil
}

func deleteNFSExport(path string) (string, error) {
	if _, err := exec.LookPath("exportfs"); err != nil {
		return "", fmt.Errorf("exportfs not found")
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return "", errors.New("path empty")
	}
	lines, changed := removeNFSExportLine(path)
	if changed {
		if err := writeNFSExports(lines); err != nil {
			return "", err
		}
	}
	out, err := runCmdCombined(context.Background(), 30*time.Second, "exportfs", "-ra")
	if err != nil {
		return out, fmt.Errorf("exportfs reload failed: %w", err)
	}
	return out, nil
}

func buildNFSExportLine(path string, clients []string, options string) string {
	var b strings.Builder
	b.WriteString(path)
	for _, c := range clients {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		b.WriteString(" ")
		b.WriteString(c)
		b.WriteString("(")
		b.WriteString(options)
		b.WriteString(")")
	}
	return b.String()
}

func upsertNFSExportLine(path string, entry string) ([]string, bool) {
	lines, err := readNFSExportsRaw()
	if err != nil {
		lines = []string{}
	}
	var out []string
	changed := false
	found := false
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			out = append(out, line)
			continue
		}
		fields := strings.Fields(trim)
		if len(fields) > 0 && fields[0] == path {
			out = append(out, entry)
			found = true
			if trim != entry {
				changed = true
			}
			continue
		}
		out = append(out, line)
	}
	if !found {
		out = append(out, entry)
		changed = true
	}
	return out, changed
}

func removeNFSExportLine(path string) ([]string, bool) {
	lines, err := readNFSExportsRaw()
	if err != nil {
		return []string{}, false
	}
	var out []string
	changed := false
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			out = append(out, line)
			continue
		}
		fields := strings.Fields(trim)
		if len(fields) > 0 && fields[0] == path {
			changed = true
			continue
		}
		out = append(out, line)
	}
	return out, changed
}

func readNFSExportsRaw() ([]string, error) {
	b, err := os.ReadFile(nfsExportsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	return strings.Split(string(b), "\n"), nil
}

func writeNFSExports(lines []string) error {
	if err := os.MkdirAll(filepath.Dir(nfsExportsPath), 0755); err != nil {
		return err
	}
	content := strings.Join(lines, "\n")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return os.WriteFile(nfsExportsPath, []byte(content), 0644)
}

func applyNFSSSSDConfig(conf string, caBundle string) (string, error) {
	conf = strings.TrimSpace(conf)
	if conf == "" {
		return "", fmt.Errorf("sssd.conf required")
	}
	if err := os.MkdirAll("/etc/sssd", 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile("/etc/sssd/sssd.conf", []byte(conf), 0600); err != nil {
		return "", err
	}
	if strings.TrimSpace(caBundle) != "" {
		if err := os.MkdirAll("/etc/sssd/certs", 0755); err != nil {
			return "", err
		}
		if err := os.WriteFile("/etc/sssd/certs/ca.crt", []byte(caBundle), 0644); err != nil {
			return "", err
		}
	}

	var out string
	if _, err := exec.LookPath("systemctl"); err == nil {
		cmdOut, cmdErr := runCmdCombined(context.Background(), 30*time.Second, "systemctl", "restart", "sssd")
		out = cmdOut
		if cmdErr != nil {
			out = strings.TrimSpace(out + "\n" + cmdErr.Error())
		}
	}
	return out, nil
}

type lsblkJSON struct {
	Blockdevices []lsblkDev `json:"blockdevices"`
}

type lsblkDev struct {
	Name      string     `json:"name"`
	Type      string     `json:"type"`
	SizeBytes int64      `json:"size"`
	Rota      *int       `json:"rota,omitempty"`
	Model     string     `json:"model,omitempty"`
	Children  []lsblkDev `json:"children,omitempty"`
}

func discoverDisks() []Disk {
	info := lsblkInfo()
	if ds := disksFromSymlinks("/dev/disk/by-id/*", info); len(ds) > 0 {
		return ds
	}
	if ds := disksFromSymlinks("/dev/disk/by-path/*", info); len(ds) > 0 {
		return ds
	}
	if ds := disksFromLsblk(info); len(ds) > 0 {
		return ds
	}
	return []Disk{}
}

type diskInfo struct {
	SizeBytes  int64
	Model      string
	Rotational *bool
}

func lsblkInfo() map[string]diskInfo {
	cmd := exec.Command("lsblk", "-b", "-J", "-o", "NAME,TYPE,SIZE,ROTA,MODEL")
	b, err := cmd.Output()
	if err != nil {
		return map[string]diskInfo{}
	}
	var parsed lsblkJSON
	if err := json.Unmarshal(b, &parsed); err != nil {
		return map[string]diskInfo{}
	}
	info := make(map[string]diskInfo)
	for _, dev := range parsed.Blockdevices {
		if dev.Type != "disk" || dev.Name == "" {
			continue
		}
		var rotational *bool
		if dev.Rota != nil {
			val := *dev.Rota == 1
			rotational = &val
		}
		info[dev.Name] = diskInfo{
			SizeBytes:  dev.SizeBytes,
			Model:      strings.TrimSpace(dev.Model),
			Rotational: rotational,
		}
	}
	return info
}

func disksFromSymlinks(pattern string, info map[string]diskInfo) []Disk {
	matches, _ := filepath.Glob(pattern)
	seen := map[string]bool{}
	out := make([]Disk, 0, len(matches))

	for _, m := range matches {
		base := filepath.Base(m)
		// exclude partitions (both common patterns)
		if strings.Contains(base, "-part") || strings.Contains(base, "part") {
			continue
		}
		if seen[base] {
			continue
		}
		seen[base] = true
		disk := Disk{ID: base, Path: m}
		if target, err := filepath.EvalSymlinks(m); err == nil {
			if meta, ok := info[filepath.Base(target)]; ok {
				disk.SizeBytes = meta.SizeBytes
				disk.Model = meta.Model
				disk.Rotational = meta.Rotational
			}
		}
		out = append(out, disk)
	}
	return out
}

func disksFromLsblk(info map[string]diskInfo) []Disk {
	if len(info) == 0 {
		return []Disk{}
	}
	names := make([]string, 0, len(info))
	for name := range info {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Disk, 0, len(names))
	for _, name := range names {
		meta := info[name]
		out = append(out, Disk{
			ID:         name,
			Path:       "/dev/" + name,
			SizeBytes:  meta.SizeBytes,
			Model:      meta.Model,
			Rotational: meta.Rotational,
		})
	}
	return out
}

func splitLines(s string) []string {
	var out []string
	for _, ln := range strings.Split(s, "\n") {
		ln = strings.TrimSpace(ln)
		if ln != "" {
			out = append(out, ln)
		}
	}
	return out
}
