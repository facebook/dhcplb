# DHCPLB Lima Test Lab

This directory contains a self-contained lab environment for testing `dhcplb` using [Lima](https://github.com/lima-vm/lima). The lab uses Linux network namespaces to simulate different DHCP scenarios.

## Files

-   `Makefile`: Simplifies the entire lab workflow (start, stop, setup, test, etc.).
-   `dhcplb-vm.yaml`: The Lima VM configuration file.
-   `setup_lab.sh`: The main entry point script for setting up the test lab. It takes an argument (`relay` or `server`) to determine which topology to create.
-   `includes/`: Directory containing containing common shell functions and variables sourced by `setup_lab.sh`. It includes functions for cleanup, configuration, and service management.

## Test Scenarios

The lab supports two primary test topologies:

### 1. Relay Lab

This is the standard 4-actor topology that simulates `dhcplb` operating as a load balancer behind another DHCP relay.

**Topology:**
`[ ns-client ] <-> [ br-int ] <-> [ ns-relay ] <-> [ br-ext ] <-> [ ns-dhcplb ] <-> [ ns-server ]`

- **Use Case:** Tests the core load balancing and relaying of unicast DHCP packets.
- **Setup Command:** `make setup-relay`
- **Test Commands:** `make test-relay-v4`, `make test-relay-v6`

### 2. Server/Broadcast Lab

This 3-actor topology places `dhcplb` as the first-hop relay for the client. It tests `dhcplb`'s ability to handle broadcast DHCPv4 packets and multicast DHCPv6 packets directly from a client. This is particularly important for testing features like raw socket replies for clients without an IP address.

**Topology:**
`[ ns-client ] <-> [ br-int ] <-> [ ns-dhcplb ] <-> [ br-ext ] <-> [ ns-server ]`

- **Use Case:** Tests handling of broadcast/multicast packets and the reply path to clients without an IP.
- **Setup Command:** `make setup-server`
- **Test Commands:** `make test-server-v4`, `make test-server-v6`

## How to Run the Lab

The `Makefile` provides a simple interface for managing the lab. All commands should be run from this `tests` directory.

1. **Start the Lima VM:**
    ```sh
    make start
    ```

2. **Set up a specific lab environment:**
    ```sh
    # For the relay scenario
    make setup-relay

    # For the server/broadcast scenario
    make setup-server
    ```

3. **Run tests for the configured lab:**
    ```sh
    # Run a v4 test in the relay lab
    make test-relay-v4

    # Run a v6 test in the server lab
    make test-server-v6
    ```

4. **Run all tests:**
    The `test-all` target will automatically set up each lab environment and run the corresponding v4 and v6 tests.
    ```sh
    make test-all
    ```

5. **Stop the VM and clean it:**
    ```sh
    make clean
    ```

## Debugging

- **`make shell`**: Get an interactive root shell inside the VM.
- **`make logs`**: Tail the logs from all running services in the lab.
- **`make delete-lease`**: Force-deletes the client's DHCP lease file to ensure a full DORA/SARR handshake on the next test run. The test targets run this automatically.
- **tcpdump**:  you can take a tcpdump of the network traffic with these commands should you need it (use `-w dhcp.pcap` to write to a file):

```sh
tcpdump -i any -l 'udp port 67 or udp port 68'
tcpdump -i any -l 'udp port 546 or udp port 547'
```

## GitHub Actions

The `test-lab` workflow runs the DHCP integration test on Linux but does not need to use `lima`.
The Github Actions workflow can be found in `.github/workflows/test-lab.yml` and run on Ubuntu runners.

