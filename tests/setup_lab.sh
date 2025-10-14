#!/bin/bash

set -e

source ./includes/common.sh
source ./includes/setup_relay_mode.sh
source ./includes/setup_server_mode.sh

main() {
    MODE=$1
    if [[ "${MODE}" != "relay" && "${MODE}" != "server" ]]; then
        echo "Usage: $0 [relay|server]"
        exit 1
    fi

    setup_common_config
    cleanup
    install_dhcplb
    mkdir -p "${LOG_DIR}" &>/dev/null
    mkdir -p "${CONF_DIR}" &>/dev/null
    mkdir -p "${DHCP_DIR}" &>/dev/null

    if [ "${MODE}" == "relay" ]; then
        echo -e "\n🧪 Setting up lab in RELAY mode..."
        setup_relay_mode
        start_relay_daemons
    elif [ "${MODE}" == "server" ]; then
        echo -e "\n🧪 Setting up lab in SERVER mode..."
        setup_server_mode
        start_server_daemons
    fi
    configure_etc_hosts "${MODE}"
}

# --- Script Entry Point ---
main "$@"