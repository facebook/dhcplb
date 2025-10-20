//go:build linux

/**
 * Copyright (c) Facebook, Inc. and its affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

package dhcplb

import (
	"fmt"
	"net"
	"syscall"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/insomniacslk/dhcp/dhcpv4"
)

// sendEthernet crafts and sends a DHCPv4 packet over a raw socket.
// This is used to send a unicast reply to a client that does not yet have an IP address.
func sendEthernet(iface net.Interface, dhcp *dhcpv4.DHCPv4) error {

	// Get the source IP address from the interface
	addrs, err := iface.Addrs()
	if err != nil {
		return fmt.Errorf("could not get addresses for interface %s: %w", iface.Name, err)
	}
	var srcIP net.IP
	for _, addr := range addrs {
		if ip, ok := addr.(*net.IPNet); ok && ip.IP.To4() != nil {
			srcIP = ip.IP
			break
		}
	}
	if srcIP == nil {
		return fmt.Errorf("could not find a usable IPv4 address for interface %s", iface.Name)
	}

	// Set up all the layers' headers
	eth := layers.Ethernet{
		SrcMAC:       iface.HardwareAddr,
		DstMAC:       dhcp.ClientHWAddr,
		EthernetType: layers.EthernetTypeIPv4,
	}
	ipv4 := layers.IPv4{
		Version:  4,
		IHL:      5, // IHL: Internet Header Length, 5 means no options
		TTL:      64,
		Protocol: layers.IPProtocolUDP,
		SrcIP:    srcIP,
		DstIP:    dhcp.YourIPAddr,
	}
	udp := layers.UDP{
		SrcPort: layers.UDPPort(dhcpv4.ServerPort),
		DstPort: layers.UDPPort(dhcpv4.ClientPort),
	}
	udp.SetNetworkLayerForChecksum(&ipv4)

	// Serialize the packet
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{
		ComputeChecksums: true,
		FixLengths:       true,
	}
	if err := gopacket.SerializeLayers(buf, opts, &eth, &ipv4, &udp, gopacket.Payload(dhcp.ToBytes())); err != nil {
		return fmt.Errorf("failed to serialize packet: %w", err)
	}

	// Open a raw socket and send the packet
	// Open a raw socket to send the packet. We use AF_PACKET to send the packet
	// at the link layer, and SOCK_RAW to send the packet without any additional
	// headers. We use ETH_P_ALL to allow the kernel to pick the best interface
	// to send the packet on.
	fd, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW, int(htons(syscall.ETH_P_ALL)))
	if err != nil {
		return fmt.Errorf("failed to open raw socket: %w", err)
	}
	defer syscall.Close(fd)

	addr := syscall.SockaddrLinklayer{
		Ifindex: iface.Index,
	}
	if err := syscall.Sendto(fd, buf.Bytes(), 0, &addr); err != nil {
		return fmt.Errorf("failed to send on raw socket: %w", err)
	}

	return nil
}

// htons converts a short (uint16) from host-to-network byte order.
func htons(i uint16) uint16 {
	return (i<<8)&0xff00 | i>>8
}
