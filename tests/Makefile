# Makefile for managing the DHCPLB Lima Test Lab
# Should be run from the 'tests' directory.

# --- Configuration ---
LIMA_YAML_FILE := dhcplb-vm.yaml
LIMA_INSTANCE_NAME := $(shell basename $(LIMA_YAML_FILE) .yaml)

# --- Commands ---
.PHONY: help all start stop delete clean setup shell logs test-client delete-lease

# Set 'help' as the default target
.DEFAULT_GOAL := help

help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "This Makefile should be run from the 'tests' directory."
	@echo ""
	@echo "Primary Targets:"
	@echo "  all            - Start the VM, set up the lab, and run a client test."
	@echo "  start          - Start the Lima VM instance '$(LIMA_INSTANCE_NAME)' if not running."
	@echo "  setup          - Run the lab setup script inside the running VM."
	@echo "  test-client    - Run a full DHCP client test (Discover, Offer, Request, Ack)."
	@echo "  logs           - Tail the logs from all services (dhcplb, relay, server)."
	@echo "  shell          - Open an interactive shell inside the Lima VM."
	@echo ""
	@echo "Lifecycle Targets:"
	@echo "  stop           - Stop the Lima VM instance."
	@echo "  delete         - Delete the Lima VM instance and all its data."
	@echo "  clean          - Alias for 'delete'."
	@echo "  delete-lease   - Force-deletes the client's DHCP lease file."

all: start setup test-client

start:
	@if ! limactl list $(LIMA_INSTANCE_NAME) --format '{{.Status}}' 2>/dev/null | grep -q "Running"; then \
		echo "--> Starting Lima VM instance '$(LIMA_INSTANCE_NAME)' from '$(LIMA_YAML_FILE)'..."; \
		limactl start --tty=false ./$(LIMA_YAML_FILE); \
	else \
		echo "--> Lima VM instance '$(LIMA_INSTANCE_NAME)' is already running."; \
	fi

stop:
	echo "--> Stopping Lima VM instance '$(LIMA_INSTANCE_NAME)'..."
	limactl stop -f $(LIMA_INSTANCE_NAME)

delete:
	echo "--> Deleting Lima VM instance '$(LIMA_INSTANCE_NAME)'..."
	limactl delete -f $(LIMA_INSTANCE_NAME)

clean: stop delete

setup: start
	echo "--> Running lab setup script in VM '$(LIMA_INSTANCE_NAME)'..."
	limactl shell $(LIMA_INSTANCE_NAME) cp setup_lab.sh /tmp/setup_lab.sh
	limactl shell $(LIMA_INSTANCE_NAME) mkdir -p /tmp/dhcplb
	limactl shell $(LIMA_INSTANCE_NAME) cp -rv ../* /tmp/dhcplb
	limactl shell $(LIMA_INSTANCE_NAME) chmod +x /tmp/setup_lab.sh
	limactl shell $(LIMA_INSTANCE_NAME) sudo /tmp/setup_lab.sh

shell: 
	echo "--> Opening a shell in VM '$(LIMA_INSTANCE_NAME)'..."
	limactl shell $(LIMA_INSTANCE_NAME)

test-client: 
	echo "--> Running DHCP client test in 'ns-client'..."
	limactl shell $(LIMA_INSTANCE_NAME) sudo ip netns exec ns-client dhclient -v -1 v-cli