#!/bin/bash

setup_relay_mode() {
    # Namespaces
    NS_RELAY="ns-relay"
    NS_SERVER="ns-server"

    # Bridges
    BR_EXT="br-ext"

    # Veth Interface Names
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
    EXT_NET="192.168.200.0/24"
    EXT_NET_V6="fd00:200::/64"

    # IP Addresses
    RELAY_IP_INT="192.168.100.1"
    RELAY_IP_EXT="192.168.200.1"
    DHCPLB_IP="192.168.200.2"
    SERVER_IP="192.168.200.3"
    SERVER_IP_INT="192.168.100.3"

    # IPv6 Addresses
    RELAY_IP6_INT="fd00:100::1"
    RELAY_IP6_EXT="fd00:200::1"
    DHCPLB_IP6="fd00:200::2"
    SERVER_IP6="fd00:200::3"
    SERVER_IP6_INT="fd00:100::3"

    modprobe br_netfilter &>/dev/null || true
    sysctl -w net.bridge.bridge-nf-call-iptables=0 &>/dev/null || true
    sysctl -w net.bridge.bridge-nf-call-ip6tables=0 &>/dev/null || true
    sysctl -w net.bridge.bridge-nf-call-arptables=0 &>/dev/null || true

    ip netns add "${NS_CLIENT}" &>/dev/null
    ip netns add "${NS_RELAY}" &>/dev/null
    ip netns add "${NS_DHCPLB}" &>/dev/null
    ip netns add "${NS_SERVER}" &>/dev/null

    ip link add name "${BR_INT}" type bridge &>/dev/null
    ip link set "${BR_INT}" up &>/dev/null
    ip link add name "${BR_EXT}" type bridge &>/dev/null
    ip link set "${BR_EXT}" up &>/dev/null

    ip link add "${VETH_CLIENT_NS}" type veth peer name "${VETH_CLIENT_BR}" &>/dev/null
    ip link set "${VETH_CLIENT_NS}" netns "${NS_CLIENT}" &>/dev/null
    ip link set "${VETH_CLIENT_BR}" master "${BR_INT}" &>/dev/null
    ip link set "${VETH_CLIENT_BR}" up &>/dev/null

    ip link add "${VETH_RELAY_INT_NS}" type veth peer name "${VETH_RELAY_INT_BR}" &>/dev/null
    ip link set "${VETH_RELAY_INT_NS}" netns "${NS_RELAY}" &>/dev/null
    ip link set "${VETH_RELAY_INT_BR}" master "${BR_INT}" &>/dev/null
    ip link set "${VETH_RELAY_INT_BR}" up &>/dev/null

    ip link add "${VETH_RELAY_EXT_NS}" type veth peer name "${VETH_RELAY_EXT_BR}" &>/dev/null
    ip link set "${VETH_RELAY_EXT_NS}" netns "${NS_RELAY}" &>/dev/null
    ip link set "${VETH_RELAY_EXT_BR}" master "${BR_EXT}" &>/dev/null
    ip link set "${VETH_RELAY_EXT_BR}" up &>/dev/null

    ip link add "${VETH_DHCPLB_NS}" type veth peer name "${VETH_DHCPLB_BR}" &>/dev/null
    ip link set "${VETH_DHCPLB_NS}" netns "${NS_DHCPLB}" &>/dev/null
    ip link set "${VETH_DHCPLB_BR}" master "${BR_EXT}" &>/dev/null
    ip link set "${VETH_DHCPLB_BR}" up &>/dev/null

    ip link add "${VETH_SERVER_NS}" type veth peer name "${VETH_SERVER_BR}" &>/dev/null
    ip link set "${VETH_SERVER_NS}" netns "${NS_SERVER}" &>/dev/null
    ip link set "${VETH_SERVER_BR}" master "${BR_EXT}" &>/dev/null
    ip link set "${VETH_SERVER_BR}" up &>/dev/null

    ip link add "${VETH_SERVER_INT_NS}" type veth peer name "${VETH_SERVER_INT_BR}" &>/dev/null
    ip link set "${VETH_SERVER_INT_NS}" netns "${NS_SERVER}" &>/dev/null
    ip link set "${VETH_SERVER_INT_BR}" master "${BR_INT}" &>/dev/null
    ip link set "${VETH_SERVER_INT_BR}" up &>/dev/null

    ip netns exec "${NS_CLIENT}" ip link set dev "${VETH_CLIENT_NS}" up &>/dev/null
    ip netns exec "${NS_CLIENT}" ip link set dev lo up &>/dev/null

    ip netns exec "${NS_RELAY}" ip addr add "${RELAY_IP_INT}/24" dev "${VETH_RELAY_INT_NS}" &>/dev/null
    ip netns exec "${NS_RELAY}" ip -6 addr add "${RELAY_IP6_INT}/64" dev "${VETH_RELAY_INT_NS}" &>/dev/null
    ip netns exec "${NS_RELAY}" ip link set dev "${VETH_RELAY_INT_NS}" up &>/dev/null
    ip netns exec "${NS_RELAY}" ip addr add "${RELAY_IP_EXT}/24" dev "${VETH_RELAY_EXT_NS}" &>/dev/null
    ip netns exec "${NS_RELAY}" ip -6 addr add "${RELAY_IP6_EXT}/64" dev "${VETH_RELAY_EXT_NS}" &>/dev/null
    ip netns exec "${NS_RELAY}" ip link set dev "${VETH_RELAY_EXT_NS}" up &>/dev/null
    ip netns exec "${NS_RELAY}" ip link set dev lo up &>/dev/null

    ip netns exec "${NS_DHCPLB}" ip addr add "${DHCPLB_IP}/24" dev "${VETH_DHCPLB_NS}" &>/dev/null
    ip netns exec "${NS_DHCPLB}" ip -6 addr add "${DHCPLB_IP6}/64" dev "${VETH_DHCPLB_NS}" &>/dev/null
    ip netns exec "${NS_DHCPLB}" ip link set dev "${VETH_DHCPLB_NS}" up &>/dev/null
    ip netns exec "${NS_DHCPLB}" ip link set dev lo up &>/dev/null

    ip netns exec "${NS_SERVER}" ip addr add "${SERVER_IP}/24" dev "${VETH_SERVER_NS}" &>/dev/null
    ip netns exec "${NS_SERVER}" ip -6 addr add "${SERVER_IP6}/64" dev "${VETH_SERVER_NS}" &>/dev/null
    ip netns exec "${NS_SERVER}" ip link set dev "${VETH_SERVER_NS}" up &>/dev/null
    ip netns exec "${NS_SERVER}" ip addr add "${SERVER_IP_INT}/24" dev "${VETH_SERVER_INT_NS}" &>/dev/null
    ip netns exec "${NS_SERVER}" ip -6 addr add "${SERVER_IP6_INT}/64" dev "${VETH_SERVER_INT_NS}" &>/dev/null
    ip netns exec "${NS_SERVER}" ip link set dev "${VETH_SERVER_INT_NS}" up &>/dev/null
    ip netns exec "${NS_SERVER}" ip link set dev lo up &>/dev/null

    ip netns exec "${NS_CLIENT}" ethtool -K "${VETH_CLIENT_NS}" tx off &>/dev/null
    ip netns exec "${NS_RELAY}" ethtool -K "${VETH_RELAY_INT_NS}" tx off &>/dev/null
    ip netns exec "${NS_RELAY}" ethtool -K "${VETH_RELAY_EXT_NS}" tx off &>/dev/null
    ip netns exec "${NS_DHCPLB}" ethtool -K "${VETH_DHCPLB_NS}" tx off &>/dev/null
    ip netns exec "${NS_SERVER}" ethtool -K "${VETH_SERVER_NS}" tx off &>/dev/null
    ip netns exec "${NS_SERVER}" ethtool -K "${VETH_SERVER_INT_NS}" tx off &>/dev/null

    ip netns exec "${NS_RELAY}" sysctl -w net.ipv4.ip_forward=1 &>/dev/null
    ip netns exec "${NS_RELAY}" sysctl -w net.ipv6.conf.all.forwarding=1 &>/dev/null
    ip netns exec "${NS_RELAY}" sysctl -w net.ipv4.conf.all.rp_filter=0 &>/dev/null
    ip netns exec "${NS_RELAY}" sysctl -w net.ipv4.conf.default.rp_filter=0 &>/dev/null
    ip netns exec "${NS_RELAY}" sysctl -w "net.ipv4.conf.${VETH_RELAY_EXT_NS}.rp_filter=0" &>/dev/null
    ip netns exec "${NS_RELAY}" sysctl -w net.ipv4.conf.all.accept_local=1 &>/dev/null
    ip netns exec "${NS_DHCPLB}" ip route add "${INT_NET}" via "${RELAY_IP_EXT}" &>/dev/null
    ip netns exec "${NS_DHCPLB}" ip -6 route add "${INT_NET_V6}" via "${RELAY_IP6_EXT}" &>/dev/null
}