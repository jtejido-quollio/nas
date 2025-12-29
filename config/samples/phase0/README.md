# Phase 0 samples

Phase 0 is about validating the node-agent API on a fresh machine. There are no
CRDs to apply at this stage.

Bootstrap helper (optional):
```bash
./scripts/bootstrap.sh --build-images
```
Use `--skip-nfs` if you don't want kernel NFS installed.

Systemd wrapper (optional):
```bash
sudo cp scripts/nas-bootstrap.service /etc/systemd/system/nas-bootstrap.service
sudo tee /etc/default/nas-bootstrap >/dev/null <<'EOF'
REPO_ROOT=/opt/nas
BOOTSTRAP_OPTS=--build-images
EOF
sudo systemctl daemon-reload
sudo systemctl enable --now nas-bootstrap.service
```

Phase 0 smoke (optional):
```bash
make deploy-phase0
make phase0-smoke NODE_AGENT_URL=http://<node-ip>:9808
```

## Node-agent health and disk discovery
```bash
curl http://<node-ip>:9808/health
curl http://<node-ip>:9808/v1/disks
curl http://<node-ip>:9808/v1/disks?refresh=1
curl http://<node-ip>:9808/v1/disks/updated
curl http://<node-ip>:9808/v1/disks/smart?device=/dev/sdb
curl http://<node-ip>:9808/v1/disks/smart?device=/dev/sdb&json=0
curl http://<node-ip>:9808/v1/disks/smart?all=1
curl http://<node-ip>:9808/v1/disks/smart?all=1&timeout=20
```

Disk discovery uses udev-managed `/dev/disk/by-id` and listens for udev block
events to refresh the cache.

## Auth header (optional/forward-looking)
The operator is configured with `NODE_AGENT_AUTH_HEADER` and
`NODE_AGENT_AUTH_VALUE` and will pass them to the node-agent. The node-agent
does not enforce these yet, but keep the values consistent with
`config/operator/deployment.yaml` and `config/node-agent/daemonset.yaml` if you
add auth later.
