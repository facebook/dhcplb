# DHCPLB Lima Test Lab

This directory contains a self-contained lab environment for testing `dhcplb` using [Lima](https://github.com/lima-vm/lima). The lab uses Linux network namespaces to simulate a realistic DHCP environment with four key actors: a client, a relay, the `dhcplb` load balancer, and a backend DHCP server.

## Files

-   `Makefile`: Simplifies the entire lab workflow (start, stop, setup, etc.).
-   `dhcplb-vm.yaml`: The Lima VM configuration file. It sets up an Ubuntu VM with the necessary tools (`dnsmasq`, `iproute2`, Go).
-   `setup_lab.sh`: The main script that builds and runs the test environment inside the Lima VM.

## How to Run the Lab

The `Makefile` provides a simple interface for managing the lab. All commands should be run from this `tests` directory.

1.  **Start the Lima VM:**
    ```sh
    make start
    ```
    The first time you run this, Lima will download the VM image and run the provisioning scripts, which includes installing Go.

2.  **Set up the network environment:**
    ```sh
    make setup
    ```
    This command executes the `setup_lab.sh` script inside the VM to create the network namespaces and start the services.

3.  **Run a DHCP client test:**
    ```sh
    make test-client
    ```

4.  **Stop the VM:**
    ```sh
    make stop
    ```

## Debugging in the VM

To get an interactive shell inside the VM for debugging, use the Makefile target:
```sh
make shell
```
**Important:** When you get a shell, you are in with your current user. To get a root shell with the correct `PATH` that includes Go, you **must** use `sudo su -`. The hyphen is critical as it starts a proper login shell.

```sh
# Inside the Lima shell
sudo su -

# Now the go command will be in your PATH
go version
```

## Network Topology

The lab creates a segmented network to simulate a real-world scenario where `dhcplb` sits between a relay and a server.

```
[ ns-client ] <--> [ br-int ] <--> [ ns-relay ] <--> [ br-ext ] <--> [ ns-dhcplb ]
(192.168.100.0/24)      |               |          (192.168.200.0/24)      |
                        |               +----------------------------------+
                        |                                                  |
                        +---------------------> [ ns-server ] <-------------+
```

-   **`ns-client`**: Simulates a DHCP client requesting an IP.
-   **`ns-relay`**: Simulates a DHCP relay agent (`dnsmasq`). It listens for broadcasts on the internal network and forwards them as unicast packets to `dhcplb`.
-   **`ns-dhcplb`**: The `dhcplb` instance being tested. It forwards requests to the backend server.
-   **`ns-server`**: A backend DHCP server (`dnsmasq`) that assigns IP addresses.
-   **`br-int`**: The "internal" network bridge (`192.168.100.0/24`).
-   **`br-ext`**: The "external" network bridge (`192.168.200.0/24`).

## The Routing Challenge and Solution

A key challenge in this topology is handling the DHCP reply (the `DHCPOFFER`) from the server.

### The Problem

1.  The client broadcasts a `DHCPDISCOVER`.
2.  The relay forwards it, setting the `giaddr` (Gateway IP Address) field to its internal IP (`192.168.100.1`).
3.  `dhcplb` forwards the request to the server.
4.  The server correctly sends the `DHCPOFFER` back to the `giaddr` (`192.168.100.1`).
5.  However, this reply is sent over the **external network** (`192.168.200.0/24`). The relay's interface on this network has the IP `192.168.200.1`.
6.  When the relay's kernel sees a packet destined for `192.168.100.1` arrive on its `192.168.200.1` interface, it drops the packet because the destination IP does not match.

This indicates that for this topology to work, `dhcplb` must rewrite the destination IP of the reply packet to match the relay's external IP.

### The Workaround

To get the lab functional and test the forward path, the `setup_lab.sh` script implements a workaround: it connects the `ns-server` to **both** the internal and external bridges.

This "multi-homed" server now has a direct route to the internal network. When it sends the `DHCPOFFER` to `192.168.100.1`, its kernel knows to send it out of its internal interface directly onto `br-int`, where the relay can receive it. This bypasses `dhcplb` on the return path but allows for successful end-to-end testing of the DHCP lease process.

## Forcing a Full DHCP Handshake (DORA)

To consistently test the full Discover, Offer, Request, Acknowledge (DORA) cycle, you must force `dhclient` to forget its previous lease.

You can do this by deleting the lease file before running the client:

```sh
# Delete the old lease
make delete-lease

# Run the client to start a new DORA handshake
make test-client
```
