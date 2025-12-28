SHELL := /bin/bash

KUBECTL ?= kubectl
KUSTOMIZE ?= kustomize
DOCKER ?= docker
K3S_CTR ?= k3s ctr
PLATFORM ?=
IMAGE_TAR_DIR ?= /tmp
NODE_AGENT_TAR ?= $(IMAGE_TAR_DIR)/nas-node-agent.tar
OPERATOR_TAR ?= $(IMAGE_TAR_DIR)/nas-operator.tar
OPENEBS_ZFS_MANIFEST ?= https://raw.githubusercontent.com/openebs/zfs-localpv/master/deploy/zfs-operator.yaml

NAMESPACE ?= nas-system
IMG_NODE_AGENT ?= localhost/nas-node-agent:dev
IMG_OPERATOR ?= localhost/nas-operator:dev

.PHONY: fmt tidy build images save-images load-images \
  deploy-crds deploy-node-agent deploy-operator deploy-storage \
  deploy-phase1 phase1-smoke cleanup-phase1 \
  deploy-phase2 phase2-smoke cleanup-phase2

fmt:
	gofmt -w .

tidy:
	go mod tidy

build:
	GOOS=linux GOARCH=amd64 go build -o bin/node-agent ./cmd/node-agent
	GOOS=linux GOARCH=amd64 go build -o bin/operator ./cmd/operator

images:
	$(DOCKER) build $(if $(PLATFORM),--platform $(PLATFORM)) -f build/node-agent.Dockerfile -t $(IMG_NODE_AGENT) .
	$(DOCKER) build $(if $(PLATFORM),--platform $(PLATFORM)) -f build/operator.Dockerfile -t $(IMG_OPERATOR) .

save-images:
	$(DOCKER) save -o $(NODE_AGENT_TAR) $(IMG_NODE_AGENT)
	$(DOCKER) save -o $(OPERATOR_TAR) $(IMG_OPERATOR)

load-images: save-images
	$(K3S_CTR) images import $(NODE_AGENT_TAR)
	$(K3S_CTR) images import $(OPERATOR_TAR)

deploy-crds:
	$(KUBECTL) apply -k config/crd

deploy-node-agent:
	$(KUBECTL) apply -k config/node-agent

deploy-operator:
	$(KUBECTL) apply -k config/operator

deploy-storage:
	$(KUBECTL) apply -f $(OPENEBS_ZFS_MANIFEST)
	$(KUBECTL) apply -f config/storage/openebs-zfs/storageclass.yaml
	$(KUBECTL) wait --for=condition=Established \
		crd/volumesnapshotclasses.snapshot.storage.k8s.io \
		crd/volumesnapshotcontents.snapshot.storage.k8s.io \
		crd/volumesnapshots.snapshot.storage.k8s.io \
		--timeout=120s
	$(KUBECTL) apply -f config/storage/openebs-zfs/volumesnapshotclass.yaml

deploy-phase1: deploy-crds deploy-node-agent deploy-operator deploy-storage
	$(KUBECTL) apply -k config/samples/phase1
	@echo "Phase1 resources applied."
	@$(MAKE) phase1-smoke

phase1-smoke:
	@echo "== Waiting for namespace $(NAMESPACE) =="
	$(KUBECTL) get ns $(NAMESPACE) >/dev/null 2>&1 || (echo "Namespace missing"; exit 1)

	@echo "== Waiting for node-agent DaemonSet =="
	$(KUBECTL) -n $(NAMESPACE) rollout status ds/nas-node-agent --timeout=180s

	@echo "== Waiting for operator =="
	$(KUBECTL) -n $(NAMESPACE) rollout status deploy/nas-operator --timeout=180s

	@echo "== Waiting for OpenEBS ZFS CSI components (kube-system) =="
	-$(KUBECTL) -n kube-system rollout status deploy/openebs-zfs-localpv-controller --timeout=240s
	-$(KUBECTL) -n kube-system rollout status ds/openebs-zfs-localpv-node --timeout=240s

	@echo "== Phase1 CRs =="
	-$(KUBECTL) -n $(NAMESPACE) get zpool,zdataset,zsnapshot,zsnapshotrestore 2>/dev/null || true
	-$(KUBECTL) -n $(NAMESPACE) get pvc,pod,volumesnapshot 2>/dev/null || true

	@echo "== Helpful commands =="
	@echo "kubectl -n $(NAMESPACE) describe zpool test-pool"
	@echo "kubectl -n $(NAMESPACE) get pvc,pv -o wide"
	@echo "kubectl -n $(NAMESPACE) describe zsnapshot demo-snap"
	@echo "kubectl -n $(NAMESPACE) describe zsnapshotrestore demo-restore"

cleanup-phase1:
	-$(KUBECTL) delete -k config/samples/phase1 --ignore-not-found=true
	-$(KUBECTL) delete -k config/storage --ignore-not-found=true
	-$(KUBECTL) delete -k config/operator --ignore-not-found=true
	-$(KUBECTL) delete -k config/node-agent --ignore-not-found=true
	-$(KUBECTL) delete -k config/crd --ignore-not-found=true
	@echo "Cleanup complete."
	@echo "Phase1 sample cleanup complete."

deploy-phase2: deploy-crds deploy-node-agent deploy-operator
	$(KUBECTL) apply -k config/samples/phase2
	@echo "Phase2 resources applied."
	@$(MAKE) phase2-smoke

phase2-smoke:
	@echo "== Waiting for namespace $(NAMESPACE) =="
	$(KUBECTL) get ns $(NAMESPACE) >/dev/null 2>&1 || (echo "Namespace missing"; exit 1)

	@echo "== Waiting for node-agent DaemonSet =="
	$(KUBECTL) -n $(NAMESPACE) rollout status ds/nas-node-agent --timeout=180s

	@echo "== Waiting for operator =="
	$(KUBECTL) -n $(NAMESPACE) rollout status deploy/nas-operator --timeout=180s

	@echo "== Current core pods =="
	$(KUBECTL) -n $(NAMESPACE) get pods -o wide

	@echo "== Sample CRs =="
	-$(KUBECTL) -n $(NAMESPACE) get zpool,zdataset,smbshare,zsnapshotschedule,zsnapshotrestore 2>/dev/null || true

	@echo "== Helpful commands =="
	@echo "kubectl -n $(NAMESPACE) describe smbshare home-share"
	@echo "kubectl -n $(NAMESPACE) describe zsnapshotschedule home-every-2min"
	@echo "kubectl -n $(NAMESPACE) get svc -o wide"
	@echo ""
	@echo "SMB NodePorts (default samples):"
	@echo "  home:       30445"
	@echo "  timemachine:31445"

cleanup-phase2:
	-$(KUBECTL) delete -k config/samples/phase2 --ignore-not-found=true
	-$(KUBECTL) delete -k config/operator --ignore-not-found=true
	-$(KUBECTL) delete -k config/node-agent --ignore-not-found=true
	-$(KUBECTL) delete -k config/crd --ignore-not-found=true
	@echo "Cleanup complete."
