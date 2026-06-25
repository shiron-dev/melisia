ANSIBLE_DEFAULT_OPT ?=
ANSIBLE_DIR := ansible
UV_ANSIBLE := uv run --project tools/ansible --
PROJECT_ID := shiron-dev
HOME_EP_SSH_PRIVATE_KEY_SECRET := github-actions-home-ep-ssh-private-key
HOME_EP_SSH_KEY := .local/ssh/home_ep_key
HOME_KIOSK_SSH_PRIVATE_KEY_SECRET := github-actions-home-kiosk-ssh-private-key
HOME_KIOSK_SSH_KEY := .local/ssh/home_kiosk_key

CHECK_SECRETS_SCRIPT := scripts/check-secrets.sh
KICS_IMAGE ?= checkmarx/kics:v2.1.20

.PHONY: help
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Available targets:"
	@awk ' BEGIN { FS = ":[ \t]*"; comment = "" } /^#/ { comment = substr($$0, 3); next } /^\.PHONY:/ { if ($$2) { \
			n = split($$2, targets, " "); for (i = 1; i <= n; i++) {if (targets[i] != "") {printf "  \033[36m%-20s\033[0m %s\n", targets[i], comment;}}} comment = "";} \
		{ if (!/^\.PHONY:/) { comment = "" } }' $(MAKEFILE_LIST)

.PHONY: init
init:
	@echo "Initializing..."

.PHONY: ansible-init
ansible-init: init
	$(UV_ANSIBLE) bash -c "cd $(ANSIBLE_DIR) && ansible-galaxy install -r requirements.yml"

.PHONY: ansible-lint
ansible-lint: ansible-init
	$(UV_ANSIBLE) bash -c "cd $(ANSIBLE_DIR) && ansible-lint -c .ansible-lint --fix"

define check_gcloud_auth
	@if ! (gcloud config get-value project 2>/dev/null | grep -q "^$(PROJECT_ID)$$" && \
	    gcloud auth list --filter=status:ACTIVE --format="value(account)" | grep -q . && \
	    gcloud auth application-default print-access-token >/dev/null 2>&1); then \
		gcloud auth login; \
		gcloud auth application-default login; \
	fi
endef

.PHONY: auth
auth: init
	$(call check_gcloud_auth)
	gcloud config set project $(PROJECT_ID)
	$(MAKE) home-ep-ssh-key
	$(MAKE) home-kiosk-ssh-key
	ssh -o BatchMode=yes -o ConnectTimeout=5 -F $(ANSIBLE_DIR)/ssh_config ansible_user@arm-srv.shiron.dev exit
	cd $(ANSIBLE_DIR) && ssh -o BatchMode=yes -o ConnectTimeout=5 -F ssh_config home-ep exit
	cd $(ANSIBLE_DIR) && ssh -o BatchMode=yes -o ConnectTimeout=5 -F ssh_config home-kiosk exit

.PHONY: home-ep-ssh-key
home-ep-ssh-key: init
	@set -eu; \
	dir="$$(dirname "$(HOME_EP_SSH_KEY)")"; \
	install -m 700 -d "$$dir"; \
	tmp="$$(mktemp "$$dir/home_ep_key.XXXXXX")"; \
	trap 'rm -f "$$tmp"' EXIT; \
	gcloud secrets versions access latest --project="$(PROJECT_ID)" --secret="$(HOME_EP_SSH_PRIVATE_KEY_SECRET)" > "$$tmp"; \
	install -m 600 "$$tmp" "$(HOME_EP_SSH_KEY)"
	@echo "Wrote $(HOME_EP_SSH_KEY)"

.PHONY: home-kiosk-ssh-key
home-kiosk-ssh-key: init
	@set -eu; \
	dir="$$(dirname "$(HOME_KIOSK_SSH_KEY)")"; \
	install -m 700 -d "$$dir"; \
	tmp="$$(mktemp "$$dir/home_kiosk_key.XXXXXX")"; \
	trap 'rm -f "$$tmp"' EXIT; \
	gcloud secrets versions access latest --project="$(PROJECT_ID)" --secret="$(HOME_KIOSK_SSH_PRIVATE_KEY_SECRET)" > "$$tmp"; \
	install -m 600 "$$tmp" "$(HOME_KIOSK_SSH_KEY)"
	@echo "Wrote $(HOME_KIOSK_SSH_KEY)"

.PHONY: ansible-ci
ansible-ci: ansible-lint

.PHONY: ansible-check
ansible-check: ansible-init
	$(UV_ANSIBLE) bash -c "cd $(ANSIBLE_DIR) && ansible-playbook -i hosts.yml site.yml -C $(ANSIBLE_DEFAULT_OPT)"

.PHONY: ansible-run
ansible-run: ansible-init
	$(UV_ANSIBLE) bash -c "cd $(ANSIBLE_DIR) && ansible-playbook -i hosts.yml site.yml $(ANSIBLE_DEFAULT_OPT)"

CMT_DIR := tools/cmt
CMT_BIN := $(CMT_DIR)/cmt
CMT_CONFIG := compose/config.yml
CMT_SCHEMA_DIR := $(CMT_DIR)/schemas
CMT_OPT ?=
TERRAFORM_TARGET ?= terraform
TERRAFORM_DIR = terraform/$(TERRAFORM_TARGET)
TERRAFORM_SECRETS_TFVARS = terraform.secrets.tfvars
TERRAFORM_SECRETS_ARG := $(if $(wildcard $(TERRAFORM_DIR)/$(TERRAFORM_SECRETS_TFVARS)),-var-file=$(TERRAFORM_SECRETS_TFVARS),)
TRUENAS_PROVIDER_DIR := $(CURDIR)/terraform/provider
TRUENAS_PROVIDER_BIN := $(TRUENAS_PROVIDER_DIR)/terraform-provider-truenas
TRUENAS_PROVIDER_DEVRC := $(CURDIR)/.local/terraform-provider-truenas.tfrc
TRUENAS_PROVIDER_VERSION := 0.0.1
TRUENAS_PROVIDER_HOST := registry.terraform.io
TRUENAS_PROVIDER_NAMESPACE := shiron-dev
TRUENAS_PROVIDER_TYPE := truenas
TRUENAS_PROVIDER_OS := $(shell uname -s | tr '[:upper:]' '[:lower:]')
TRUENAS_PROVIDER_ARCH := $(shell uname -m | sed 's/x86_64/amd64/')
TRUENAS_PROVIDER_MIRROR_DIR := $(CURDIR)/.local/terraform-provider-mirror
TRUENAS_PROVIDER_MIRROR_PACKAGE_DIR := $(TRUENAS_PROVIDER_MIRROR_DIR)/$(TRUENAS_PROVIDER_HOST)/$(TRUENAS_PROVIDER_NAMESPACE)/$(TRUENAS_PROVIDER_TYPE)/$(TRUENAS_PROVIDER_VERSION)/$(TRUENAS_PROVIDER_OS)_$(TRUENAS_PROVIDER_ARCH)
TRUENAS_PROVIDER_MIRROR_BIN := $(TRUENAS_PROVIDER_MIRROR_PACKAGE_DIR)/terraform-provider-$(TRUENAS_PROVIDER_TYPE)_v$(TRUENAS_PROVIDER_VERSION)
TERRAFORM_CLI_CONFIG_ENV := $(if $(filter truenas,$(TERRAFORM_TARGET)),TF_CLI_CONFIG_FILE=$(TRUENAS_PROVIDER_DEVRC),)
GO ?= go

.PHONY: terraform-provider-build
terraform-provider-build:
	cd $(TRUENAS_PROVIDER_DIR) && GOROOT= GOTOOLCHAIN=auto $(GO) build -o $(TRUENAS_PROVIDER_BIN)

.PHONY: terraform-provider-devrc
terraform-provider-devrc: terraform-provider-build
	@mkdir -p $(dir $(TRUENAS_PROVIDER_DEVRC))
	@mkdir -p $(TRUENAS_PROVIDER_MIRROR_PACKAGE_DIR)
	@cp $(TRUENAS_PROVIDER_BIN) $(TRUENAS_PROVIDER_MIRROR_BIN)
	@printf '%s\n' \
		'provider_installation {' \
		'  filesystem_mirror {' \
		'    path = "$(TRUENAS_PROVIDER_MIRROR_DIR)"' \
		'    include = ["$(TRUENAS_PROVIDER_HOST)/$(TRUENAS_PROVIDER_NAMESPACE)/$(TRUENAS_PROVIDER_TYPE)"]' \
		'  }' \
		'' \
		'  direct {' \
		'    exclude = ["$(TRUENAS_PROVIDER_HOST)/$(TRUENAS_PROVIDER_NAMESPACE)/$(TRUENAS_PROVIDER_TYPE)"]' \
		'  }' \
		'}' > $(TRUENAS_PROVIDER_DEVRC)

.PHONY: terraform-truenas-replace-provider-state
terraform-truenas-replace-provider-state: terraform-provider-devrc
	cd terraform/truenas && TF_CLI_CONFIG_FILE=$(TRUENAS_PROVIDER_DEVRC) terraform state replace-provider registry.terraform.io/baladithyab/truenas registry.terraform.io/shiron-dev/truenas

# cmt ビルド + JSON Schema 生成
.PHONY: cmt-init
cmt-init: init
	cd $(CMT_DIR) && go build -o cmt .
	@mkdir -p $(CMT_SCHEMA_DIR)
	$(CMT_BIN) schema cmt  > $(CMT_SCHEMA_DIR)/cmt-config.schema.json
	$(CMT_BIN) schema host > $(CMT_SCHEMA_DIR)/host-config.schema.json

# cmt plan（変更内容の確認）
.PHONY: cmt-plan
cmt-plan: cmt-init
	$(CMT_BIN) --config $(CMT_CONFIG) plan $(CMT_OPT)

# cmt apply（変更の適用）
.PHONY: cmt-apply
cmt-apply: cmt-init
	$(CMT_BIN) --config $(CMT_CONFIG) apply $(CMT_OPT)

.PHONY: terraform-init
terraform-init: init $(if $(filter truenas,$(TERRAFORM_TARGET)),terraform-provider-devrc)
	cd $(TERRAFORM_DIR) && $(TERRAFORM_CLI_CONFIG_ENV) terraform init

.PHONY: terraform-plan
terraform-plan: terraform-init
	cd $(TERRAFORM_DIR) && $(TERRAFORM_CLI_CONFIG_ENV) terraform plan $(TERRAFORM_SECRETS_ARG)

.PHONY: terraform-apply
terraform-apply: terraform-init
	cd $(TERRAFORM_DIR) && $(TERRAFORM_CLI_CONFIG_ENV) terraform apply $(TERRAFORM_SECRETS_ARG) -lock=false
	$(MAKE) sops-encrypt

.PHONY: terraform-lint
terraform-lint: terraform-init
	cd $(TERRAFORM_DIR) && tflint

.PHONY: terraform-fmt
terraform-fmt:
	cd $(TERRAFORM_DIR) && terraform fmt -recursive

.PHONY: terraform-validate
terraform-validate: terraform-init
	cd $(TERRAFORM_DIR) && $(TERRAFORM_CLI_CONFIG_ENV) terraform validate

# https://github.com/aquasecurity/trivy - IaC misconfig and vulnerability scanning
.PHONY: terraform-trivy
terraform-trivy:
	trivy config terraform/

# https://www.checkov.io/ - Infrastructure as Code scanning
.PHONY: terraform-checkov
terraform-checkov:
	checkov --directory terraform/ --framework terraform --compact

# https://github.com/aquasecurity/tfsec - Terraform security scanner
.PHONY: terraform-tfsec
terraform-tfsec:
	tfsec --ignore-hcl-errors terraform/

# https://www.pluralith.com/ - Terraform visualization and cost analysis
.PHONY: pluralith
pluralith:
	cd terraform && pluralith graph

# https://github.com/gitleaks/gitleaks - Secret scanning
.PHONY: gitleaks
gitleaks:
	gitleaks detect --source . --verbose

# https://github.com/trufflesecurity/trufflehog - Secret scanning
.PHONY: trufflehog
trufflehog:
	trufflehog filesystem . --json

.PHONY: terraform-ci
terraform-ci: terraform-lint terraform-fmt terraform-validate terraform-trivy terraform-checkov terraform-tfsec

# コスト比較前のベースライン作成
.PHONY: infracost-base
infracost-base: terraform-plan
	cd $(TERRAFORM_DIR) && infracost breakdown --path=. --format json --out-file infracost-base.json

# コスト比較
.PHONY: infracost-diff
infracost-diff: terraform-plan
	@if [ ! -f $(TERRAFORM_DIR)/infracost-base.json ]; then \
		echo "Error: infracost-base.json not found. Run 'make infracost-base' first."; \
		exit 1; \
	fi
	cd $(TERRAFORM_DIR) && infracost diff --path=. --compare-to infracost-base.json

# 今のコストチェック
.PHONY: infracost-breakdown
infracost-breakdown: terraform-plan
	cd $(TERRAFORM_DIR) && infracost breakdown --path=.

.PHONY: sops-encrypt
sops-encrypt:
	@echo "Encrypting with SOPS..."; \
	if [ -n "$(FILE)" ]; then \
		if [ -f "$(FILE)" ] && [ "$${FILE##*.}" != "sops" ] && [ "$${FILE#*.sops.}" = "$(FILE)" ]; then FILES="$(FILE)"; \
		elif [ -f "$(FILE)" ] && [ "$${FILE##*.}" = "sops" ]; then base="$${FILE%.sops}"; if [ -f "$$base" ]; then FILES="$$base"; else echo "Error: plaintext $$base not found for $(FILE)" >&2; exit 1; fi; \
		elif [ -f "$(FILE)" ] && [ "$${FILE#*.sops.}" != "$(FILE)" ]; then base="$$(printf '%s\n' "$(FILE)" | sed 's/\.sops\././')"; if [ -f "$$base" ]; then FILES="$$base"; else echo "Error: plaintext $$base not found for $(FILE)" >&2; exit 1; fi; \
		elif [ -f "$(FILE).sops" ]; then base="$(FILE)"; if [ -f "$$base" ]; then FILES="$$base"; else echo "Error: plaintext $$base not found (got $(FILE).sops)" >&2; exit 1; fi; \
		else echo "Error: $(FILE) not found" >&2; exit 1; fi; \
	else \
		FILES="$$(find . -name "*.secrets.*" -type f ! -name "*.sops" ! -name "*.sops.*")"; \
	fi; \
	for file in $$FILES; do \
		target="$$file.sops"; \
		ext="$${file##*.}"; \
		stem="$${file%.*}"; \
		if [ -f "$$stem.sops.$$ext" ]; then target="$$stem.sops.$$ext"; fi; \
		echo "Encrypting $$file..."; \
		sops --output-type json --encrypt "$$file" > "$$target"; \
	done

.PHONY: sops-decrypt
sops-decrypt:
	@echo "Decrypting with SOPS..."; \
	if [ -n "$(FILE)" ]; then \
		file_arg="$(FILE)"; \
		if [ -f "$$file_arg" ]; then FILES="$$file_arg"; \
		elif [ -f "$$file_arg.sops" ]; then FILES="$$file_arg.sops"; \
		else ext="$${file_arg##*.}"; stem="$${file_arg%.*}"; if [ -f "$$stem.sops.$$ext" ]; then FILES="$$stem.sops.$$ext"; else echo "Error: $$file_arg, $$file_arg.sops, or $$stem.sops.$$ext not found" >&2; exit 1; fi; fi; \
	else \
		FILES="$$(find . \( -name "*.secrets.*.sops" -o -name "*.secrets.sops.*" \) -type f)"; \
	fi; \
	for file in $$FILES; do \
		echo "Decrypting $$file..."; \
		if [ "$${file##*.}" = "sops" ]; then base="$${file%.sops}"; else base="$$(printf '%s\n' "$$file" | sed 's/\.sops\././')"; fi; \
		ext="$${base##*.}"; \
		case "$$ext" in \
		  yaml|yml) output_type="yaml" ;; \
		  *) output_type="binary" ;; \
		esac; \
		sops --decrypt --output-type "$$output_type" "$$file" > "$$base"; \
	done

.PHONY: sops-edit
sops-edit:
	@$(MAKE) sops-decrypt $(if $(FILE),FILE="$(FILE)",); \
	if [ -n "$(FILE)" ]; then \
		base="$(FILE)"; \
		[ "$${base%.sops}" != "$$base" ] && base="$${base%.sops}"; \
		[ "$${base#*.sops.}" != "$$base" ] && base="$$(printf '%s\n' "$$base" | sed 's/\.sops\././')"; \
		echo "Edit the decrypted file(s). Press Enter when done to re-encrypt."; \
		code --wait "$$base"; \
		$(MAKE) sops-encrypt FILE="$$base"; \
	else \
		echo "Edit the decrypted file(s). Press Enter when done to re-encrypt."; \
		read -r _; \
		$(MAKE) sops-encrypt; \
	fi

.PHONY: sops-ci
sops-ci:
	@echo "Checking for unencrypted secrets tracked by git..."; \
	FILES="$$(find . -name '*.secrets.*' ! -name '*.secrets.*.sops' ! -name '*.secrets.sops.*' -type f)"; \
	EXIT=0; \
	for file in $$FILES; do \
		if git ls-files --error-unmatch "$$file" >/dev/null 2>&1; then \
			echo "Error: Unencrypted secrets file tracked by git: $$file" >&2; \
			EXIT=1; \
		fi; \
	done; \
	if [ $$EXIT -ne 0 ]; then \
		echo "One or more unencrypted secrets files are tracked by git. Please remove them from version control." >&2; \
		exit 1; \
	fi

.PHONY: kics
kics:
	@dir=$$(mktemp -d) && \
	rsync -a --exclude='tools/ansible/.venv' --exclude='*.secrets.*' --exclude='terraform/terraform.tfvars' $(PWD)/ $$dir/ && \
	docker run -t -v $$dir:/path $(KICS_IMAGE) scan -p /path; \
	status=$$?; rm -rf $$dir; exit $$status

.PHONY: ci
ci:
	@if git diff --name-only origin/main...HEAD | grep -q "^ansible/"; then \
		echo "Running ansible-ci..."; \
		$(MAKE) ansible-ci; \
	fi
	@if git diff --name-only origin/main...HEAD | grep -q "^terraform/"; then \
		echo "Running terraform-ci..."; \
		$(MAKE) terraform-ci; \
	fi

	echo "Running sops-ci...";
	$(MAKE) sops-ci;

	echo "Running kics...";
	$(MAKE) kics;

	echo "Running security scanning...";
	$(MAKE) gitleaks;
	$(MAKE) trufflehog;

.DEFAULT_GOAL := help
