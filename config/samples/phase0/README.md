# Phase 0 samples

Phase 0 is about validating the node-agent API on a fresh machine. There are no
CRDs to apply at this stage.

## Node-agent health and disk discovery
```bash
curl http://<node-ip>:9808/health
curl http://<node-ip>:9808/v1/disks
```

## Auth header (optional/forward-looking)
The operator is configured with `NODE_AGENT_AUTH_HEADER` and
`NODE_AGENT_AUTH_VALUE` and will pass them to the node-agent. The node-agent
does not enforce these yet, but keep the values consistent with
`config/operator/deployment.yaml` and `config/node-agent/daemonset.yaml` if you
add auth later.
