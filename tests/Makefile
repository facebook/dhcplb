# Makefile for managing the DHCPLB Lima Test Lab
# Should be run from the 'tests' directory.

# --- Configuration ---
LIMA_YAML_FILE := dhcplb-vm.yaml
LIMA_INSTANCE_NAME := $(shell basename $(LIMA_YAML_FILE) .yaml)

# --- Commands ---
.PHONY: help start stop delete clean setup-relay setup-server shell logs test-relay-v4 test-relay-v6 test-server-v4 test-server-v6 test-all delete-lease

.DEFAULT_GOAL := help

help:
	@echo "Makefile for DHCPLB Test Lab"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "  Primary Targets:"
	@echo "    test-all         - 🧪 Set up and run all test scenarios (relay and server)."
	@echo ""
	@echo "  Relay Scenario:"
	@echo "    setup-relay      - 🛠️ Set up the relay test lab."
	@echo "    test-relay-v4    - ▶️ Run a DHCPv4 client test in the relay lab."
	@echo "    test-relay-v6    - ▶️ Run a DHCPv6 client test in the relay lab."
	@echo ""
	@echo "  Server Scenario:"
	@echo "    setup-server     - 🛠️ Set up the server/broadcast test lab."
	@echo "    test-server-v4   - ▶️ Run a DHCPv4 client test in the server lab."
	@echo "    test-server-v6   - ▶️ Run a DHCPv6 client test in the server lab."
	@echo ""
	@echo "  VM & Debugging:"
	@echo "    start            - 🚀 Start the Lima VM instance '$(LIMA_INSTANCE_NAME)'."
	@echo "    logs             - 📜 Tail the logs from all services."
	@echo "    shell            - 💻 Open an interactive shell inside the Lima VM."
	@echo "    stop             - 🛑 Stop the Lima VM instance."
	@echo "    delete           - 🗑️ Delete the Lima VM instance and all its data."
	@echo "    delete-lease     - 📄 Force-deletes the client's DHCP lease file."

start:
	@if ! limactl list $(LIMA_INSTANCE_NAME) --format '{{.Status}}' 2>/dev/null | grep -q "Running"; then \
		echo "🚀 Starting Lima VM instance '$(LIMA_INSTANCE_NAME)'..."; \
		limactl start --log-level warn --tty=false ./$(LIMA_YAML_FILE); \
	else \
		echo "✅ Lima VM instance '$(LIMA_INSTANCE_NAME)' is already running."; \
	fi

stop:
	@echo "🛑 Stopping Lima VM instance '$(LIMA_INSTANCE_NAME)'..."
	@limactl stop -f $(LIMA_INSTANCE_NAME) &>/dev/null

delete:
	@echo "🗑️  Deleting Lima VM instance '$(LIMA_INSTANCE_NAME)'..."
	@limactl delete -f $(LIMA_INSTANCE_NAME) &>/dev/null

clean: stop delete

# Generic setup logic
define SETUP_SCRIPT
	@echo "📦 Copying project files to VM..."
	@limactl shell $(LIMA_INSTANCE_NAME) mkdir -p /tmp/dhcplb &>/dev/null
	@limactl shell $(LIMA_INSTANCE_NAME) cp -r ../* /tmp/dhcplb &>/dev/null
	@limactl shell $(LIMA_INSTANCE_NAME) cp setup_lab.sh /tmp/setup_lab.sh &>/dev/null
	@limactl shell $(LIMA_INSTANCE_NAME) chmod +x /tmp/setup_lab.sh &>/dev/null
	@echo "🛠️  Running lab setup script (setup_lab.sh $(1))..."
	@limactl shell $(LIMA_INSTANCE_NAME) sudo /tmp/setup_lab.sh $(1)
endef

setup-relay: start
	$(call SETUP_SCRIPT,relay)

setup-server: start
	$(call SETUP_SCRIPT,server)

shell:
	@echo "💻 Opening a shell in VM '$(LIMA_INSTANCE_NAME)'..."
	@limactl shell $(LIMA_INSTANCE_NAME)

logs:
	@echo "📜 Tailing logs in VM. Press Ctrl+C to exit."
	@limactl shell $(LIMA_INSTANCE_NAME) sudo bash -c "tail -F /var/log/dhcplb_lab/*.log"

delete-lease:
	@echo "📄 Deleting DHCP client lease file..."
	@limactl shell $(LIMA_INSTANCE_NAME) sudo ip netns exec ns-client bash -c "rm -f /var/lib/dhcp/*.leases"

test-relay-v4: delete-lease
	@echo "▶️  Running DHCPv4 RELAY client test..."
	@limactl shell $(LIMA_INSTANCE_NAME) sudo ip netns exec ns-client dhclient -v -1 v-cli

test-relay-v6: delete-lease
	@echo "▶️  Running DHCPv6 RELAY client test..."
	@limactl shell $(LIMA_INSTANCE_NAME) sudo ip netns exec ns-client dhclient -v -6 -1 v-cli

test-server-v4: delete-lease
	@echo "▶️  Running DHCPv4 SERVER client test..."
	@limactl shell $(LIMA_INSTANCE_NAME) sudo ip netns exec ns-client dhclient -v -1 v-cli

test-server-v6: delete-lease
	@echo "▶️  Running DHCPv6 SERVER client test..."
	@limactl shell $(LIMA_INSTANCE_NAME) sudo ip netns exec ns-client dhclient -v -6 -1 v-cli

test-all: \
	start \
	setup-relay \
	test-relay-v4 \
	test-relay-v6 \
	setup-server \
	test-server-v4 \
	test-server-v6