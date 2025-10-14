#!/bin/bash -e
#
# run_lab.sh - Restored to the 4-namespace topology with all fixes.
#
# This script sets up a DHCP testing lab using Linux network namespaces,
# virtual Ethernet pairs (veth), and bridges.
#
# Topology:
#
# [ ns-client ] <--> [ br-int ] <--> [ ns-relay ] <--> [ br-ext ] <--> [ ns-dhcplb ]
#                                       ^ ^ ^
#                                       | | |
#                                       +---+------> [ ns-server ]
#
# - br-int: "Internal" client-side LAN.
# - br-ext: "External" server-side LAN.
#

set -e

# --- Configuration ---
# Namespaces
NS_CLIENT="ns-client"
NS_RELAY="ns-relay"
NS_DHCPLB="ns-dhcplb"
NS_SERVER="ns-server"



# Bridges
BR_INT="br-int"
BR_EXT="br-ext"

# Veth Interface Names (15 char limit)
VETH_CLIENT_NS="v-cli"
VETH_CLIENT_BR="v-br-cli"
VETH_RELAY_INT_NS="v-rly-int"
VETH_RELAY_INT_BR="v-br-rly-int"
VETH_RELAY_EXT_NS="v-rly-ext"
VETH_RELAY_EXT_BR="v-br-rly-ext"
VETH_DHCPLB_NS="v-lb"
VETH_DHCPLB_BR="v-br-lb"
VETH_SERVER_NS="v-srv"
VETH_SERVER_BR="v-br-srv"
VETH_SERVER_INT_NS="v-srv-int"
VETH_SERVER_INT_BR="v-br-srv-int"

# Subnets
INT_NET="192.168.100.0/24"
EXT_NET="192.168.200.0/24"

# IP Addresses
RELAY_IP_INT="192.168.100.1"
RELAY_IP_EXT="192.168.200.1"
DHCPLB_IP="192.168.200.2"
SERVER_IP="192.168.200.3"
SERVER_IP_INT="192.168.100.3"

# DHCP Range for clients
DHCP_RANGE_START="192.168.100.10"
DHCP_RANGE_END="192.168.100.50"
DHCP_LEASE_TIME="12h"

# Log directory
LOG_DIR="/var/log/dhcplb_lab"

# --- Cleanup ---
echo "--- Cleaning up previous lab setup ---"
# Kill all known processes that might be running
pkill -f dnsmasq || true
pkill -f dhcplb || true
pkill -f dhclient || true

# Forcefully delete network namespaces and their contents
for ns in ${NS_CLIENT} ${NS_RELAY} ${NS_DHCPLB} ${NS_SERVER}; do
    ip netns exec "${ns}" pkill -f . &>/dev/null || true # Kill everything inside
    ip netns del "${ns}" &>/dev/null || true
done

# Delete bridges, which also removes connected veth pairs
for br in ${BR_INT} ${BR_EXT}; do
    ip link del "${br}" &>/dev/null || true
done

# Clean up logs
rm -rf "${LOG_DIR}"

# --- Setup ---
# On unbuntu x86_64 runs this should already be installed
if ! command -v dhcplb &> /dev/null; then
    echo "--- dhcplb not found, installing ---"
    export PATH=$PATH:/usr/local/go/bin
    # The project root is mounted at /dhcplb
    cd ..
    go install .
    cd -
    cp /root/go/bin/dhcplb /usr/local/bin/
    echo "--- dhcplb installed: $(command -v dhcplb) ---"
fi

echo "--- Setting up network namespaces and bridges ---"
# Ensure the bridge netfilter module is loaded, as it's required on some systems.
modprobe br_netfilter &>/dev/null || true
# Disable bridge netfilter to prevent iptables from interfering with bridge traffic.
sysctl -w net.bridge.bridge-nf-call-iptables=0 &>/dev/null || true
sysctl -w net.bridge.bridge-nf-call-ip6tables=0 &>/dev/null || true
sysctl -w net.bridge.bridge-nf-call-arptables=0 &>/dev/null || true

# Create namespaces
ip netns add "${NS_CLIENT}"
ip netns add "${NS_RELAY}"
ip netns add "${NS_DHCPLB}"
ip netns add "${NS_SERVER}"

# Create bridges
ip link add name "${BR_INT}" type bridge
ip link set "${BR_INT}" up
ip link add name "${BR_EXT}" type bridge
ip link set "${BR_EXT}" up
echo "Namespaces and bridges created."
echo

# --- Create and Connect Veth Pairs ---
echo "--- Creating and connecting virtual Ethernet pairs ---"
# 1. Connect Client to Internal Bridge
ip link add "${VETH_CLIENT_NS}" type veth peer name "${VETH_CLIENT_BR}"
ip link set "${VETH_CLIENT_NS}" netns "${NS_CLIENT}"
ip link set "${VETH_CLIENT_BR}" master "${BR_INT}"
ip link set "${VETH_CLIENT_BR}" up

# 2. Connect Relay to Internal Bridge
ip link add "${VETH_RELAY_INT_NS}" type veth peer name "${VETH_RELAY_INT_BR}"
ip link set "${VETH_RELAY_INT_NS}" netns "${NS_RELAY}"
ip link set "${VETH_RELAY_INT_BR}" master "${BR_INT}"
ip link set "${VETH_RELAY_INT_BR}" up

# 3. Connect Relay to External Bridge
ip link add "${VETH_RELAY_EXT_NS}" type veth peer name "${VETH_RELAY_EXT_BR}"
ip link set "${VETH_RELAY_EXT_NS}" netns "${NS_RELAY}"
ip link set "${VETH_RELAY_EXT_BR}" master "${BR_EXT}"
ip link set "${VETH_RELAY_EXT_BR}" up

# 4. Connect DHCPLB to External Bridge
ip link add "${VETH_DHCPLB_NS}" type veth peer name "${VETH_DHCPLB_BR}"
ip link set "${VETH_DHCPLB_NS}" netns "${NS_DHCPLB}"
ip link set "${VETH_DHCPLB_BR}" master "${BR_EXT}"
ip link set "${VETH_DHCPLB_BR}" up

# 5. Connect Server to External Bridge
ip link add "${VETH_SERVER_NS}" type veth peer name "${VETH_SERVER_BR}"
ip link set "${VETH_SERVER_NS}" netns "${NS_SERVER}"
ip link set "${VETH_SERVER_BR}" master "${BR_EXT}"
ip link set "${VETH_SERVER_BR}" up

# 6. Connect Server to Internal Bridge
ip link add "${VETH_SERVER_INT_NS}" type veth peer name "${VETH_SERVER_INT_BR}"
ip link set "${VETH_SERVER_INT_NS}" netns "${NS_SERVER}"
ip link set "${VETH_SERVER_INT_BR}" master "${BR_INT}"
ip link set "${VETH_SERVER_INT_BR}" up
echo "Veth pairs created and connected."
echo

# --- Configure IP Addresses ---
echo "--- Configuring IP addresses and bringing interfaces up ---"
# Client (no IP, will use DHCP)
ip netns exec "${NS_CLIENT}" ip link set dev "${VETH_CLIENT_NS}" up
ip netns exec "${NS_CLIENT}" ip link set dev lo up

# Relay
ip netns exec "${NS_RELAY}" ip addr add "${RELAY_IP_INT}/24" dev "${VETH_RELAY_INT_NS}"
ip netns exec "${NS_RELAY}" ip link set dev "${VETH_RELAY_INT_NS}" up
ip netns exec "${NS_RELAY}" ip addr add "${RELAY_IP_EXT}/24" dev "${VETH_RELAY_EXT_NS}"
ip netns exec "${NS_RELAY}" ip link set dev "${VETH_RELAY_EXT_NS}" up
ip netns exec "${NS_RELAY}" ip link set dev lo up

# DHCPLB
ip netns exec "${NS_DHCPLB}" ip addr add "${DHCPLB_IP}/24" dev "${VETH_DHCPLB_NS}"
ip netns exec "${NS_DHCPLB}" ip link set dev "${VETH_DHCPLB_NS}" up
ip netns exec "${NS_DHCPLB}" ip link set dev lo up

# Server
ip netns exec "${NS_SERVER}" ip addr add "${SERVER_IP}/24" dev "${VETH_SERVER_NS}"
ip netns exec "${NS_SERVER}" ip link set dev "${VETH_SERVER_NS}" up
ip netns exec "${NS_SERVER}" ip addr add "${SERVER_IP_INT}/24" dev "${VETH_SERVER_INT_NS}"
ip netns exec "${NS_SERVER}" ip link set dev "${VETH_SERVER_INT_NS}" up
ip netns exec "${NS_SERVER}" ip link set dev lo up
echo "IP configuration complete."
echo

# --- Configure Name Resolution ---
echo "--- Configuring /etc/hosts for all namespaces (idempotent) ---"
for ns in ${NS_CLIENT} ${NS_RELAY} ${NS_DHCPLB} ${NS_SERVER}; do
    HOSTS_BLOCK="
# BEGIN Lab Hosts
${RELAY_IP_INT}    relay-int
${RELAY_IP_EXT}    relay-ext
${DHCPLB_IP}       dhcplb
${SERVER_IP}       server
# END Lab Hosts
"
    CURRENT_HOSTS=$(ip netns exec "${ns}" cat /etc/hosts 2>/dev/null || true)
    CLEANED_HOSTS=$(echo "${CURRENT_HOSTS}" | sed '/# BEGIN Lab Hosts/,/# END Lab Hosts/d')
    printf "%s%s\n" "$(echo -n "${CLEANED_HOSTS}")" "${HOSTS_BLOCK}" | ip netns exec "${ns}" sh -c "cat > /etc/hosts"
done
echo "Name resolution configured."
echo

# --- Configure Routing & Forwarding ---
echo "--- Configuring routing and IP forwarding ---"
# Enable IP forwarding in the relay namespace
ip netns exec "${NS_RELAY}" sysctl -w net.ipv4.ip_forward=1

# Kernel settings to allow the DHCP reply to be routed correctly.
ip netns exec "${NS_RELAY}" sysctl -w net.ipv4.conf.all.rp_filter=0
ip netns exec "${NS_RELAY}" sysctl -w net.ipv4.conf.default.rp_filter=0
ip netns exec "${NS_RELAY}" sysctl -w "net.ipv4.conf.${VETH_RELAY_EXT_NS}.rp_filter=0"
ip netns exec "${NS_RELAY}" sysctl -w net.ipv4.conf.all.accept_local=1

# Add route on DHCPLB to forward replies to the relay
ip netns exec "${NS_DHCPLB}" ip route add "${INT_NET}" via "${RELAY_IP_EXT}"
echo "Routing complete."
echo

# --- Configure and Start Services ---
echo "--- Configuring and starting services ---"
mkdir -p "${LOG_DIR}"

# Ensure the go binary path is in the PATH
export PATH=$PATH:/usr/local/go/bin

# 1. DHCP Server (dnsmasq)
echo "Starting DHCP server in '${NS_SERVER}'..."
DNSMASQ_SERVER_CONF="/tmp/dnsmasq-server.conf"
cat <<EOF > "${DNSMASQ_SERVER_CONF}"
interface=${VETH_SERVER_NS}
port=0
dhcp-range=${DHCP_RANGE_START},${DHCP_RANGE_END},${DHCP_LEASE_TIME}
dhcp-option=option:router,${RELAY_IP_INT}
log-dhcp
dhcp-leasefile=/var/lib/dhcp/dnsmasq.leases
EOF
mkdir -p /var/lib/dhcp
touch /var/lib/dhcp/dnsmasq.leases
ip netns exec "${NS_SERVER}" dnsmasq --no-daemon -C "${DNSMASQ_SERVER_CONF}" > "${LOG_DIR}/dnsmasq-server.log" 2>&1 &

# 2. DHCP Relay (dnsmasq)
echo "Starting DHCP relay in '${NS_RELAY}'..."
DNSMASQ_RELAY_CONF="/tmp/dnsmasq-relay.conf"
cat <<EOF > "${DNSMASQ_RELAY_CONF}"
interface=${VETH_RELAY_INT_NS}
interface=${VETH_RELAY_EXT_NS}
dhcp-relay=${RELAY_IP_INT},${DHCPLB_IP}
log-dhcp
EOF
ip netns exec "${NS_RELAY}" dnsmasq --no-daemon -C "${DNSMASQ_RELAY_CONF}" > "${LOG_DIR}/dnsmasq-relay.log" 2>&1 &

# 3. DHCPLB
echo "Starting DHCPLB in '${NS_DHCPLB}'..."
DHCPLB_SERVERS_FILE="/tmp/dhcp-servers-v4.cfg"
echo "${SERVER_IP}" > "${DHCPLB_SERVERS_FILE}"

DHCPLB_CONF_FILE="/tmp/dhcplb.config.json"
cat <<EOF > "${DHCPLB_CONF_FILE}" 
{
  "v4": {
    "version": 4,
    "listen_addr": "0.0.0.0",
    "port": 67,
    "reply_addr": "${DHCPLB_IP}",
    "packet_buf_size": 1024,
    "update_server_interval": 30,
    "algorithm": "xid",
    "host_sourcer": "file:${DHCPLB_SERVERS_FILE}",
    "rc_ratio": 0,
    "throttle_cache_size": 1024,
    "throttle_cache_rate": 128,
    "throttle_rate": 256
  }
}
EOF

ip netns exec "${NS_DHCPLB}" /usr/local/bin/dhcplb -config "${DHCPLB_CONF_FILE}" > "${LOG_DIR}/dhcplb.log" 2>&1 &

echo "Services started."
echo

# --- Lab is Ready ---
echo "✅ test lab is running."