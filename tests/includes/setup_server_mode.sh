#!/bin/bash

setup_server_mode() {
    # Veth Interface Names
    VETH_CLIENT_NS="v-cli"
    VETH_CLIENT_BR="v-br-cli"
    VETH_DHCPLB_NS="v-lb"
    VETH_DHCPLB_BR="v-br-lb"

    # IP Addresses
    DHCPLB_IP="192.168.100.1"
    DHCPLB_IP6="fd00:100::1"

    for ns in ${NS_CLIENT} ${NS_DHCPLB}; do ip netns add "${ns}"; done &>/dev/null
    ip link add name "${BR_INT}" type bridge &>/dev/null
    ip link set "${BR_INT}" up &>/dev/null

    ip link add "${VETH_CLIENT_NS}" type veth peer name "${VETH_CLIENT_BR}" &>/dev/null
    ip link set "${VETH_CLIENT_NS}" netns "${NS_CLIENT}" &>/dev/null
    ip link set "${VETH_CLIENT_BR}" master "${BR_INT}" &>/dev/null
    ip link set "${VETH_CLIENT_BR}" up &>/dev/null

    ip link add "${VETH_DHCPLB_NS}" type veth peer name "${VETH_DHCPLB_BR}" &>/dev/null
    ip link set "${VETH_DHCPLB_NS}" netns "${NS_DHCPLB}" &>/dev/null
    ip link set "${VETH_DHCPLB_BR}" master "${BR_INT}" &>/dev/null
    ip link set "${VETH_DHCPLB_BR}" up &>/dev/null

    ip netns exec "${NS_CLIENT}" ip link set dev "${VETH_CLIENT_NS}" up &>/dev/null
    ip netns exec "${NS_CLIENT}" ip link set dev lo up &>/dev/null

    ip netns exec "${NS_DHCPLB}" ip addr add "${DHCPLB_IP}/24" dev "${VETH_DHCPLB_NS}" &>/dev/null
    ip netns exec "${NS_DHCPLB}" ip -6 addr add "${DHCPLB_IP6}/64" dev "${VETH_DHCPLB_NS}" &>/dev/null
    ip netns exec "${NS_DHCPLB}" ip link set dev "${VETH_DHCPLB_NS}" up &>/dev/null
    ip netns exec "${NS_DHCPLB}" ip link set dev lo up &>/dev/null

    sleep 2

    ip netns exec "${NS_DHCPLB}" sysctl -w net.ipv4.conf.all.rp_filter=0 &>/dev/null
    ip netns exec "${NS_DHCPLB}" sysctl -w net.ipv4.conf.default.rp_filter=0 &>/dev/null
    ip netns exec "${NS_DHCPLB}" sysctl -w "net.ipv4.conf.${VETH_DHCPLB_NS}.rp_filter=0" &>/dev/null
    ip netns exec "${NS_DHCPLB}" sysctl -w net.ipv4.conf.all.accept_local=1 &>/dev/null
    ip netns exec "${NS_DHCPLB}" sysctl -w net.ipv6.conf.all.forwarding=1 &>/dev/null
    ip netns exec "${NS_DHCPLB}" sysctl -w net.ipv6.conf.default.forwarding=1 &>/dev/null
}
