SHELL := /bin/bash

KUBECTL ?= kubectl
KUSTOMIZE ?= kustomize
DOCKER ?= docker
K3S_CTR ?= k3s ctr
PLATFORM ?=
CURL ?= curl
IMAGE_TAR_DIR ?= /tmp
NODE_AGENT_TAR ?= $(IMAGE_TAR_DIR)/nas-node-agent.tar
OPERATOR_TAR ?= $(IMAGE_TAR_DIR)/nas-operator.tar
API_TAR ?= $(IMAGE_TAR_DIR)/nas-api.tar
OPENEBS_ZFS_MANIFEST ?= https://raw.githubusercontent.com/openebs/zfs-localpv/master/deploy/zfs-operator.yaml
NODE_AGENT_URL ?= http://127.0.0.1:9808

NAMESPACE ?= nas-system
IMG_NODE_AGENT ?= localhost/nas-node-agent:dev
IMG_OPERATOR ?= localhost/nas-operator:dev
IMG_API ?= localhost/nas-api:dev

.PHONY: fmt tidy build images save-images load-images \
  deploy deploy-samples samples-smoke cleanup-samples cleanup \
  deploy-crds deploy-node-agent deploy-operator deploy-storage deploy-api cleanup-api \
  samples-ad-smoke samples-ldap-smoke

fmt:
	gofmt -w .

tidy:
	go mod tidy

build:
	GOOS=linux GOARCH=amd64 go build -o bin/node-agent ./cmd/node-agent
	GOOS=linux GOARCH=amd64 go build -o bin/operator ./cmd/operator
	GOOS=linux GOARCH=amd64 go build -o bin/nas-api ./cmd/nas-api

images:
	$(DOCKER) build $(if $(PLATFORM),--platform $(PLATFORM)) -f build/node-agent.Dockerfile -t $(IMG_NODE_AGENT) .
	$(DOCKER) build $(if $(PLATFORM),--platform $(PLATFORM)) -f build/operator.Dockerfile -t $(IMG_OPERATOR) .
	$(DOCKER) build $(if $(PLATFORM),--platform $(PLATFORM)) -f build/nas-api.Dockerfile -t $(IMG_API) .

save-images:
	$(DOCKER) save -o $(NODE_AGENT_TAR) $(IMG_NODE_AGENT)
	$(DOCKER) save -o $(OPERATOR_TAR) $(IMG_OPERATOR)
	$(DOCKER) save -o $(API_TAR) $(IMG_API)

load-images: save-images
	$(K3S_CTR) images import $(NODE_AGENT_TAR)
	$(K3S_CTR) images import $(OPERATOR_TAR)
	$(K3S_CTR) images import $(API_TAR)

deploy: deploy-crds deploy-node-agent deploy-operator deploy-storage deploy-api
	@echo "NAS deployed."

deploy-samples: deploy
	$(KUBECTL) apply -k config/samples
	@echo "Samples applied."
	@$(MAKE) samples-smoke

samples-smoke:
	KUBECTL="$(KUBECTL)" NAMESPACE="$(NAMESPACE)" NODE_AGENT_URL="$(NODE_AGENT_URL)" CURL="$(CURL)" ./scripts/samples-health.sh

	@echo "== Helpful commands =="
	@echo "kubectl -n $(NAMESPACE) describe nasshare home-share"
	@echo "kubectl -n $(NAMESPACE) describe zsnapshotschedule home-every-2min"
	@echo "kubectl -n $(NAMESPACE) get svc -o wide"
	@echo ""
	@echo "SMB NodePorts (default samples):"
	@echo "  home:       30445"
	@echo "  timemachine:31445"

cleanup-samples:
	-$(KUBECTL) delete -k config/samples --ignore-not-found=true
	-$(KUBECTL) -n $(NAMESPACE) delete deploy/smbshare-home-share svc/smbshare-home-share cm/smbshare-home-share-conf --ignore-not-found=true
	-$(KUBECTL) -n $(NAMESPACE) delete deploy/smbshare-timemachine-share svc/smbshare-timemachine-share cm/smbshare-timemachine-share-conf --ignore-not-found=true
	-$(KUBECTL) -n $(NAMESPACE) delete pvc timemachine-pvc --ignore-not-found=true
	-$(KUBECTL) -n $(NAMESPACE) patch pvc timemachine-pvc --type=merge -p '{"metadata":{"finalizers":[]}}' >/dev/null 2>&1 || true
	@echo "Samples cleanup complete (CRDs + core stack preserved)."

cleanup:
	-$(KUBECTL) delete -k config/nas-api --ignore-not-found=true
	-$(KUBECTL) delete -k config/operator --ignore-not-found=true
	-$(KUBECTL) delete -k config/node-agent --ignore-not-found=true
	-$(KUBECTL) delete -k config/storage --ignore-not-found=true
	-$(KUBECTL) delete -k config/crd --ignore-not-found=true
	-$(KUBECTL) delete ns $(NAMESPACE) --ignore-not-found=true
	@echo "Full cleanup complete."

cleanup-api:
	-$(KUBECTL) delete -k config/nas-api --ignore-not-found=true
	@echo "nas-api cleanup complete."

deploy-crds:
	$(KUBECTL) apply -k config/crd

deploy-node-agent:
	$(KUBECTL) apply -k config/node-agent

deploy-operator:
	$(KUBECTL) apply -k config/operator

deploy-api:
	$(KUBECTL) apply -k config/nas-api

deploy-storage:
	$(KUBECTL) apply -f $(OPENEBS_ZFS_MANIFEST)
	$(KUBECTL) apply -f config/storage/openebs-zfs/storageclass.yaml
	$(KUBECTL) wait --for=condition=Established \
		crd/volumesnapshotclasses.snapshot.storage.k8s.io \
		crd/volumesnapshotcontents.snapshot.storage.k8s.io \
		crd/volumesnapshots.snapshot.storage.k8s.io \
		--timeout=120s
	$(KUBECTL) apply -f config/storage/openebs-zfs/volumesnapshotclass.yaml


samples-ad-smoke:
	KUBECTL="$(KUBECTL)" NAMESPACE="$(NAMESPACE)" ./scripts/samples-ad-smoke.sh

samples-ldap-smoke:
	KUBECTL="$(KUBECTL)" NAMESPACE="$(NAMESPACE)" ./scripts/samples-ldap-smoke.sh
