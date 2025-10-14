#!/bin/bash

# --- Function Definitions ---

setup_common_config() {
    # Namespaces
    NS_CLIENT="ns-client"
    NS_DHCPLB="ns-dhcplb"

    # Bridge
    BR_INT="br-int"

    # Subnets
    INT_NET="192.168.100.0/24"
    INT_NET_V6="fd00:100::/64"

    # DHCP Range for clients
    DHCP_RANGE_START="192.168.100.10"
    DHCP_RANGE_END="192.168.100.50"
    DHCP_RANGE_V6_START="fd00:100::10"
    DHCP_RANGE_V6_END="fd00:100::50"
    DHCP_LEASE_TIME="12h"

    # Log and config directories
    LOG_DIR="/var/log/dhcplb_lab"
    CONF_DIR="/etc/dhcplb_lab"
    DHCP_DIR="/var/lib/dhcp"
}

cleanup() {
    echo "🧹 Cleaning up previous lab setup..."
    pkill -f dnsmasq &>/dev/null || true
    pkill -f /usr/local/bin/dhcplb &>/dev/null || true
    pkill -f dhclient &>/dev/null || true

    for ns in ns-client ns-dhcplb ns-relay ns-server; do
        ip netns del "${ns}" &>/dev/null || true
    done

    for br in br-int br-ext; do
        ip link del "${br}" &>/dev/null || true
    
    done

    rm -rf "${LOG_DIR}" &>/dev/null
    rm -rf "${CONF_DIR}" &>/dev/null
    rm -f "${DHCP_DIR}/dnsmasq.leases" &>/dev/null
}

install_dhcplb() {
    if [ -z "$CI" ]; then
      echo "📦 Installing dhcplb..."
      export PATH=$PATH:/usr/local/go/bin
      rm -f /usr/local/bin/dhcplb /root/go/bin/dhcplb &>/dev/null
      (cd .. && go install . && cd -) &>/dev/null
      cp /root/go/bin/dhcplb /usr/local/bin/ &>/dev/null
      echo "✅ dhcplb installed."
    fi
}

start_relay_daemons() {
    echo "🚀 Starting services..."
    render_dnsmasq_server_config
    touch "${CONF_DIR}/dnsmasq.leases" &>/dev/null
    ip netns exec "${NS_SERVER}" dnsmasq --no-daemon -C "${DNSMASQ_SERVER_CONF}" > "${LOG_DIR}/dnsmasq-server.log" 2>&1 &

    render_dnsmasq_relay_config
    ip netns exec "${NS_RELAY}" dnsmasq --no-daemon -C "${DNSMASQ_RELAY_CONF}" > "${LOG_DIR}/dnsmasq-relay.log" 2>&1 &

    sleep 2
    DHCPLB_SERVERS_FILE_V4="${CONF_DIR}/dhcp-servers-v4.cfg"
    echo "${SERVER_IP}" > "${DHCPLB_SERVERS_FILE_V4}"
    DHCPLB_SERVERS_FILE_V6="${CONF_DIR}/dhcp-servers-v6.cfg"
    echo "${SERVER_IP6}" > "${DHCPLB_SERVERS_FILE_V6}"

    render_dhcplb_relay_config

    ip netns exec "${NS_DHCPLB}" /usr/local/bin/dhcplb -config "${DHCPLB_CONF_FILE}" -version=4 > "${LOG_DIR}/dhcplb-v4.log" 2>&1 &
    ip netns exec "${NS_DHCPLB}" /usr/local/bin/dhcplb -config "${DHCPLB_CONF_FILE}" -version=6 -v=2 > "${LOG_DIR}/dhcplb-v6.log" 2>&1 &

    echo "✅ Relay test lab is running."
}

start_server_daemons() {
    echo "🚀 Starting services..."
    render_dhcplb_server_config
    
    echo "{}" > "${CONF_DIR}/dhcplb-v4.leases"
    echo "{}" > "${CONF_DIR}/dhcplb-v6.leases"

    ip netns exec "${NS_DHCPLB}" /usr/local/bin/dhcplb -config "${DHCPLB_CONF_FILE}" -server -version=4 -v=2 > "${LOG_DIR}/dhcplb-v4.log" 2>&1 &
    ip netns exec "${NS_DHCPLB}" /usr/local/bin/dhcplb -config "${DHCPLB_CONF_FILE}" -server -version=6 -v=2 > "${LOG_DIR}/dhcplb-v6.log" 2>&1 &

    echo "✅ Server test lab is running."
}

configure_etc_hosts() {
    MODE=$1
    if [ "${MODE}" == "relay" ]; then
        NAMESPACES="${NS_CLIENT} ${NS_RELAY} ${NS_DHCPLB} ${NS_SERVER}"
        HOSTS_BLOCK="
# BEGIN Lab Hosts
${RELAY_IP_INT}    relay-int
${RELAY_IP_EXT}    relay-ext
${DHCPLB_IP}       dhcplb
${SERVER_IP}       server
${RELAY_IP6_INT}   relay-int-v6
${RELAY_IP6_EXT}   relay-ext-v6
${DHCPLB_IP6}      dhcplb-v6
${SERVER_IP6}      server-v6
# END Lab Hosts
"
    elif [ "${MODE}" == "server" ]; then
        NAMESPACES="${NS_CLIENT} ${NS_DHCPLB}"
        HOSTS_BLOCK="
# BEGIN Lab Hosts
${DHCPLB_IP}       dhcplb
${DHCPLB_IP6}      dhcplb-v6
# END Lab Hosts
"
    fi

    for ns in ${NAMESPACES}; do
        CURRENT_HOSTS=$(ip netns exec "${ns}" cat /etc/hosts 2>/dev/null || true)
        CLEANED_HOSTS=$(echo "${CURRENT_HOSTS}" | sed '/# BEGIN Lab Hosts/,/# END Lab Hosts/d')
        printf "%s%s\n" "$(echo -n "${CLEANED_HOSTS}")" "${HOSTS_BLOCK}" | ip netns exec "${ns}" sh -c "cat > /etc/hosts" &>/dev/null
    done
}

render_dnsmasq_server_config() {
    DNSMASQ_SERVER_CONF="${CONF_DIR}/dnsmasq-server.conf"
    cat <<EOF > "${DNSMASQ_SERVER_CONF}"
interface=${VETH_SERVER_NS}
port=0
dhcp-range=${DHCP_RANGE_START},${DHCP_RANGE_END},${DHCP_LEASE_TIME}
dhcp-range=${DHCP_RANGE_V6_START},${DHCP_RANGE_V6_END},${DHCP_LEASE_TIME}
dhcp-option=option:router,${RELAY_IP_INT}
log-dhcp
dhcp-leasefile=${DHCP_DIR}/dnsmasq.leases
EOF
}

render_dnsmasq_relay_config() {
    DNSMASQ_RELAY_CONF="${CONF_DIR}/dnsmasq-relay.conf"
    cat <<EOF > "${DNSMASQ_RELAY_CONF}"
port=0
interface=${VETH_RELAY_INT_NS}
interface=${VETH_RELAY_EXT_NS}
dhcp-relay=${RELAY_IP_INT},${DHCPLB_IP}
dhcp-relay=${RELAY_IP6_INT},${DHCPLB_IP6}
enable-ra
log-dhcp
EOF
}

render_dhcplb_relay_config() {
    DHCPLB_CONF_FILE="${CONF_DIR}/dhcplb.config.json"
    cat <<EOF > "${DHCPLB_CONF_FILE}"
{
  "v4": {
    "version": 4,
    "listen_addr": "0.0.0.0",
    "port": 67,
    "packet_buf_size": 1024,
    "update_server_interval": 30,
    "algorithm": "xid",
    "host_sourcer": "file:${DHCPLB_SERVERS_FILE_V4}",
    "rc_ratio": 0,
    "throttle_cache_size": 1024,
    "throttle_cache_rate": 128,
    "throttle_rate": 256
  },
  "v6": {
    "version": 6,
    "listen_addr": "${DHCPLB_IP6}",
    "port": 547,
    "packet_buf_size": 1024,
    "host_sourcer": "file:${DHCPLB_SERVERS_FILE_V6}",
    "update_server_interval": 30,
    "algorithm": "xid",
    "rc_ratio": 0,
    "throttle_cache_size": 1024,
    "throttle_cache_rate": 128,
    "throttle_rate": 256
  }
}
EOF
}

render_dhcplb_server_config() {
    DHCPLB_CONF_FILE="${CONF_DIR}/dhcplb.config.json"
    cat <<EOF > "${DHCPLB_CONF_FILE}"
{
  "v4": {
    "version": 4,
    "listen_addr": "0.0.0.0",
    "port": 67,
    "packet_buf_size": 1024,
    "algorithm": "xid",
    "throttle_cache_size": 1024,
    "throttle_cache_rate": 128,
    "throttle_rate": 256,
    "handler": {
      "type": "range",
      "start_ip": "${DHCP_RANGE_START}",
      "end_ip": "${DHCP_RANGE_END}",
      "lease_time": "10m",
      "lease_file": "${CONF_DIR}/dhcplb-v4.leases",
      "options": {
        "subnet-mask": "255.255.255.0",
        "router": "${DHCPLB_IP}"
      }
    }
  },
  "v6": {
    "version": 6,
    "listen_addr": "::",
    "port": 547,
    "packet_buf_size": 1024,
    "algorithm": "xid",
    "throttle_cache_size": 1024,
    "throttle_cache_rate": 128,
    "throttle_rate": 256,
    "handler": {
      "type": "range",
      "start_ip": "${DHCP_RANGE_V6_START}",
      "end_ip": "${DHCP_RANGE_V6_END}",
      "lease_time": "10m",
      "lease_file": "${CONF_DIR}/dhcplb-v6.leases",
      "options": {
        "dns-server": ["${DHCPLB_IP6}"]
      }
    }
  }
}
EOF
}
