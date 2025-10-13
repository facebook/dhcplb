/**
 * Copyright (c) Facebook, Inc. and its affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

package dhcplb

import (
	"context"
	"errors"
	"fmt"
	"net"
	"runtime/debug"
	"time"

	"github.com/golang/glog"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/ztpv4"
	"github.com/insomniacslk/dhcp/dhcpv6"
	"github.com/insomniacslk/dhcp/dhcpv6/ztpv6"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// List of possible errors.
const (
	ErrUnknown   = "E_UNKNOWN"
	ErrPanic     = "E_PANIC"
	ErrRead      = "E_READ"
	ErrConnect   = "E_CONN"
	ErrWrite     = "E_WRITE"
	ErrGi0       = "E_GI_0"
	ErrParse     = "E_PARSE"
	ErrNoServer  = "E_NO_SERVER"
	ErrConnRate  = "E_CONN_RATE"
	ErrNoHandler = "E_NO_HANDLER"
)

func (s *Server) readLoop4(ctx context.Context) error {
	glog.V(2).Info("Starting IPv4 read loop...")
	if s.config.PacketBufSize == 0 {
		glog.Warning("packet_buf_size not configured, settingt default of 1024")
		s.config.PacketBufSize = 1024
	}
	buffer := make([]byte, s.config.PacketBufSize)
	for {
		bytesRead, cm, peer, err := s.conn.v4.ReadFrom(buffer)
		if err != nil {
			if opErr, ok := err.(*net.OpError); ok && opErr.Err.Error() == "use of closed network connection" {
				return nil // Server is closing.
			}
			glog.Errorf("error reading from v4 connection: %v", err)
			s.logger.LogErr(time.Now(), nil, nil, peer.(*net.UDPAddr), ErrRead, err)
			continue
		}
		if bytesRead == 0 {
			glog.Errorf(
				"0 bytes read even if we have packet_buf_size set to %d... something is going on",
				s.config.PacketBufSize,
			)
			continue
		}

		// Handle the packet in a new goroutine.
		go func(b []byte, p *net.UDPAddr, c *ipv4.ControlMessage) {
			defer func() {
				if r := recover(); r != nil {
					glog.Errorf("Panicked handling v4 packet from %s: %s", p, r)
					glog.Errorf("Offending packet: %x", b)
					err, _ := r.(error)
					s.logger.LogErr(time.Now(), nil, nil, p, ErrPanic, err)
					glog.Errorf("%s: %s", r, debug.Stack())
				}
			}()
			s.handleRawPacketV4(ctx, b, p, c)
		}(buffer[:bytesRead], peer.(*net.UDPAddr), cm)
	}
}

func (s *Server) readLoop6(ctx context.Context) error {
	glog.V(2).Info("Starting IPv6 read loop...")
	if s.config.PacketBufSize == 0 {
		glog.Warning("packet_buf_size not configured, settingt default of 1024")
		s.config.PacketBufSize = 1024
	}
	buffer := make([]byte, s.config.PacketBufSize)
	for {
		bytesRead, cm, peer, err := s.conn.v6.ReadFrom(buffer)
		glog.V(2).Infof("Read %d bytes from %s", bytesRead, peer)
		if err != nil {
			if opErr, ok := err.(*net.OpError); ok && opErr.Err.Error() == "use of closed network connection" {
				return nil // Server is closing.
			}
			glog.Errorf("error reading from v6 connection: %v", err)
			s.logger.LogErr(time.Now(), nil, nil, peer.(*net.UDPAddr), ErrRead, err)
			continue
		}
		if bytesRead == 0 {
			glog.Errorf(
				"0 bytes read even if we have packet_buf_size set to %d... something is going on",
				s.config.PacketBufSize,
			)
			continue
		}

		// Handle the packet in a new goroutine.
		go func(b []byte, p *net.UDPAddr, c *ipv6.ControlMessage) {
			defer func() {
				if r := recover(); r != nil {
					glog.Errorf("Panicked handling v6 packet from %s: %s", p, r)
					glog.Errorf("Offending packet: %x", b)
					err, _ := r.(error)
					s.logger.LogErr(time.Now(), nil, nil, p, ErrPanic, err)
					glog.Errorf("%s: %s", r, debug.Stack())
				}
			}()
			s.handleRawPacketV6(ctx, b, p, c)
		}(buffer[:bytesRead], peer.(*net.UDPAddr), cm)
	}
}

func selectDestinationServer(config *Config, message *DHCPMessage) (*DHCPServer, error) {
	server, err := handleOverride(config, message)
	if err != nil {
		glog.Errorf("Error handling override, drop due to: %s", err)
		return nil, err
	}
	if server == nil {
		server, err = config.Algorithm.SelectRatioBasedDhcpServer(message)
	}
	return server, err
}

func handleOverride(config *Config, message *DHCPMessage) (*DHCPServer, error) {
	if override, ok := config.Overrides[message.Mac.String()]; ok {
		// Checking if override is expired. If so, ignore it. Expiration field should
		// be a timestamp in the following format "2006/01/02 15:04 -0700".
		// For example, a timestamp in UTC would look as follows: "2017/05/06 14:00 +0000".
		var err error
		var expiration time.Time
		if override.Expiration != "" {
			expiration, err = time.Parse("2006/01/02 15:04 -0700", override.Expiration)
			if err != nil {
				glog.Errorf("Could not parse override expiration for MAC %s: %s", message.Mac.String(), err.Error())
				return nil, nil
			}
			if time.Now().After(expiration) {
				glog.Errorf("Override rule for MAC %s expired on %s, ignoring", message.Mac.String(), expiration.Local())
				return nil, nil
			}
		}
		if override.Expiration == "" {
			glog.Infof("Found override rule for %s without expiration", message.Mac.String())
		} else {
			glog.Infof("Found override rule for %s, it will expire on %s", message.Mac.String(), expiration.Local())
		}

		var server *DHCPServer
		if len(override.Host) > 0 {
			server, err = handleHostOverride(config, override.Host)
		} else if len(override.Tier) > 0 {
			server, err = handleTierOverride(config, override.Tier, message)
		}
		if err != nil {
			return nil, err
		}
		if server != nil {
			return server, nil
		}
		glog.Infof("Override didn't have host or tier, this shouldn't happen, proceeding with normal server selection")
	}
	return nil, nil
}

func handleHostOverride(config *Config, host string) (*DHCPServer, error) {
	addr := net.ParseIP(host)
	if addr == nil {
		return nil, fmt.Errorf("failed to get IP for overridden host %s", host)
	}
	port := 67
	if config.Version == 6 {
		port = 547
	}
	server := NewDHCPServer(host, addr, port)
	return server, nil
}

func handleTierOverride(config *Config, tier string, message *DHCPMessage) (*DHCPServer, error) {
	servers, err := config.HostSourcer.GetServersFromTier(tier)
	if err != nil {
		return nil, fmt.Errorf("failed to get servers from tier: %w", err)
	}
	if len(servers) == 0 {
		return nil, fmt.Errorf("sourcer returned no servers")
	}
	// pick server according to the configured algorithm
	server, err := config.Algorithm.SelectServerFromList(servers, message)
	if err != nil {
		return nil, fmt.Errorf("failed to select server: %w", err)
	}
	return server, nil
}

// sendToServer forwards a DHCP packet to a selected backend server.
// It applies rate limiting and uses the control message (`cm`) to ensure
// the packet is sent from the correct source IP and interface.
func (s *Server) sendToServer(start time.Time, server *DHCPServer, packet []byte, peer *net.UDPAddr, cm interface{}) error {
	// Check for connection rate before forwarding.
	ok, err := s.throttle.OK(server.Address.String())
	if !ok {
		glog.Errorf("error writing to server %s, drop due to throttling", server.Hostname)
		s.logger.LogErr(time.Now(), server, packet, peer, ErrConnRate, err)
		return err
	}

	if s.conn.v4 != nil {
		v4cm, ok := cm.(*ipv4.ControlMessage)
		// By setting the source IP (`Src`) to the destination IP (`Dst`) of the
		// original packet, we ensure that the kernel sends the relayed packet
		// from the same IP address that received the request. This is crucial
		// in multi-homed setups.
		if ok && v4cm.Dst != nil && !v4cm.Dst.Equal(net.IPv4bcast) {
			v4cm.Src = v4cm.Dst
		}
		_, err = s.conn.v4.WriteTo(packet, v4cm, server.udpAddr())
	} else if s.conn.v6 != nil {
		v6cm, ok := cm.(*ipv6.ControlMessage)
		// By setting the source IP (`Src`) to the destination IP (`Dst`) of the
		// original packet, we ensure that the kernel sends the relayed packet
		// from the same IP address that received the request. This is crucial
		// in multi-homed setups.
		if ok && v6cm.Dst != nil && !v6cm.Dst.IsMulticast() {
			v6cm.Src = v6cm.Dst
		}
		_, err = s.conn.v6.WriteTo(packet, v6cm, server.udpAddr())
	} else {
		return fmt.Errorf("no valid connection available to send packet")
	}

	if err != nil {
		glog.Errorf("Error writing to server %s, drop due to %s", server.Hostname, err)
		s.logger.LogErr(start, server, packet, peer, ErrWrite, err)
		return err
	}

	s.logger.LogSuccess(start, server, packet, peer)
	return nil
}

func (s *Server) handleRawPacketV4(ctx context.Context, buffer []byte, peer *net.UDPAddr, cm *ipv4.ControlMessage) {
	// runs in a separate go routine
	start := time.Now()
	var message DHCPMessage
	packet, err := dhcpv4.FromBytes(buffer)
	if err != nil {
		glog.Errorf("Error encoding DHCPv4 packet: %s", err)
		s.logger.LogErr(start, nil, nil, peer, ErrParse, err)
		return
	}

	if s.server {
		s.handleV4Server(ctx, start, packet, peer, cm)
		return
	}

	message.XID = packet.TransactionID[:]
	message.Peer = peer
	message.ClientID = packet.ClientHWAddr
	message.Mac = packet.ClientHWAddr
	if vd, err := ztpv4.ParseVendorData(packet); err != nil {
		glog.V(2).Infof("error parsing vendor data: %s", err)
	} else {
		message.Serial = vd.Serial
	}

	packet.HopCount++

	server, err := selectDestinationServer(s.config, &message)
	if err != nil {
		glog.Errorf("%s, Drop due to %s", packet.Summary(), err)
		s.logger.LogErr(start, nil, packet.ToBytes(), peer, ErrNoServer, err)
		return
	}

	s.sendToServer(start, server, packet.ToBytes(), peer, cm)
}

func (s *Server) handleV4Server(ctx context.Context, start time.Time, packet *dhcpv4.DHCPv4, peer *net.UDPAddr, cm *ipv4.ControlMessage) {
	if s.config.Handler == nil {
		glog.Errorf("No handler configured. Ignoring packet")
		s.logger.LogErr(start, nil, packet.ToBytes(), peer, ErrNoHandler, nil)
		return
	}
	reply, err := s.config.Handler.ServeDHCPv4(ctx, packet)
	if err != nil {
		glog.Errorf("Error creating reply %s", err)
		s.logger.LogErr(start, nil, packet.ToBytes(), peer, fmt.Sprintf("%T", err), err)
		return
	}
	if reply == nil {
		glog.Errorf("No reply from handler. Ignoring request")
		return
	}
	s.logger.LogSuccess(start, nil, packet.ToBytes(), peer)

	useEthernet := false
	var addr *net.UDPAddr
	if !packet.GatewayIPAddr.IsUnspecified() {
		// The request came from a relay, send the reply back to the relay
		addr = &net.UDPAddr{
			IP:   packet.GatewayIPAddr,
			Port: dhcpv4.ServerPort,
		}
	} else if packet.ClientIPAddr.IsUnspecified() && packet.Flags&0x8000 != 0 {
		// Client doesn't have an IP and requests a broadcast reply
		addr = &net.UDPAddr{
			IP:   net.IPv4bcast,
			Port: dhcpv4.ClientPort,
		}
	} else if !packet.ClientIPAddr.IsUnspecified() {
		// Client has an IP, send unicast to that IP
		addr = &net.UDPAddr{
			IP:   packet.ClientIPAddr,
			Port: dhcpv4.ClientPort,
		}
	} else {
		// Client doesn't have an IP and didn't request broadcast.
		// This is the edge case where we send a unicast frame to the client's MAC address.
		addr = &net.UDPAddr{
			IP:   reply.YourIPAddr,
			Port: dhcpv4.ClientPort,
		}
		useEthernet = true
	}

	// Set ServerIdentifier to the IP that received the request, which is in cm.Dst.
	// Fall back to the globally configured ReplyAddr if Dst is not available (e.g., older kernel).
	optServerIdentifier := net.IP(nil)
	if cm != nil && cm.Dst != nil && !cm.Dst.Equal(net.IPv4bcast) {
		optServerIdentifier = cm.Dst
	} else if s.config.ReplyAddr != nil && s.config.ReplyAddr.IP != nil {
		optServerIdentifier = s.config.ReplyAddr.IP
	}

	// The control message determines which interface the packet is sent on.
	// cm is the control message from the inbound packet. We use it to send the
	// reply on the same interface.
	if useEthernet {
		if cm != nil && cm.IfIndex != 0 {
			iface, err := net.InterfaceByIndex(cm.IfIndex)
			if err == nil {
				if optServerIdentifier == nil {
					// When sending a raw Ethernet frame, we should use the IP of the
					// sending interface as the Server Identifier.
					addrs, err := iface.Addrs()
					if err != nil {
						glog.Warningf("Could not get addresses for interface %s: %v. Server identifier may be incorrect.", iface.Name, err)
					} else {
						ipFound := false
						for _, addr := range addrs {
							if ip, ok := addr.(*net.IPNet); ok && ip.IP.To4() != nil {
								optServerIdentifier = ip.IP
								reply.UpdateOption(dhcpv4.OptServerIdentifier(optServerIdentifier))
								ipFound = true
								break
							}
						}
						if !ipFound {
							glog.Warningf("Could not find IPv4 for interface %s. Server identifier may be incorrect.", iface.Name)
						}
					}
				}
				// Attempt to send via raw socket. If it succeeds, we are done.
				if err := sendEthernet(*iface, reply); err == nil {
					s.logger.LogSuccess(start, nil, reply.ToBytes(), peer)
					return // Exit successfully
				}
				// Log the error but continue, to fall back to broadcast.
				glog.V(2).Infof("Raw socket send failed (expected on non-Linux): %v. Falling back to broadcast.", err)
			} else {
				glog.Warningf("Could not get interface for index %d: %v. Falling back to broadcast.", cm.IfIndex, err)
			}
		} else {
			glog.Warningf("Cannot send raw Ethernet packet: missing interface information. Falling back to broadcast.")
		}

		// --- FALLBACK LOGIC ---
		// If we got here, it means the raw socket send was either not possible or failed.
		// We now force a broadcast reply as the best alternative.
		addr = &net.UDPAddr{
			IP:   net.IPv4bcast,
			Port: dhcpv4.ClientPort,
		}
	}

	if !optServerIdentifier.IsUnspecified() {
		reply.UpdateOption(dhcpv4.OptServerIdentifier(optServerIdentifier))
	}

	// This single block now handles all UDP sends:
	// 1. Normal unicast/broadcast determined by the initial logic.
	// 2. Fallback broadcast if the raw socket path was triggered but failed (or we are on non-Linux systems).
	if cm != nil && cm.Dst != nil && !cm.Dst.Equal(net.IPv4bcast) {
		cm.Src = cm.Dst
	}
	_, err = s.conn.v4.WriteTo(reply.ToBytes(), cm, addr)
	if err != nil {
		glog.Errorf("Error writing to %s: %s", addr, err)
		s.logger.LogErr(start, nil, reply.ToBytes(), addr, ErrWrite, err)
		return
	}
	s.logger.LogSuccess(start, nil, reply.ToBytes(), peer)
}
func (s *Server) handleRawPacketV6(ctx context.Context, buffer []byte, peer *net.UDPAddr, cm *ipv6.ControlMessage) {
	// runs in a separate go routine
	start := time.Now()
	packet, err := dhcpv6.FromBytes(buffer)
	if err != nil {
		glog.Errorf("Error decoding DHCPv6 packet: %s", err)
		s.logger.LogErr(start, nil, buffer, peer, ErrParse, err)
		return
	}

	// In server mode, we act as a proper DHCPv6 server.
	// In relay mode, we load balance between backend DHCPv6 servers.
	if s.server {
		s.handleV6Server(ctx, start, packet, peer, cm)
		return
	}

	switch packet.Type() {
	case dhcpv6.MessageTypeSolicit, dhcpv6.MessageTypeRequest, dhcpv6.MessageTypeConfirm, dhcpv6.MessageTypeRenew, dhcpv6.MessageTypeRebind, dhcpv6.MessageTypeInformationRequest:
		// This is a direct client request received via multicast.
		// When dhcplb is in relay mode but listening on a multicast address,
		// it should still be able to forward these to a backend.
		s.handleV6RelayForward(start, packet, peer, cm)
	case dhcpv6.MessageTypeRelayForward:
		// This is a relayed request from another relay agent.
		s.handleV6RelayForward(start, packet, peer, cm)
	case dhcpv6.MessageTypeRelayReply:
		// This is a reply from a backend server to us (the relay).
		s.handleV6RelayRepl(start, packet, peer, cm)
	default:
		glog.Warningf("Received unhandled DHCPv6 message type: %s from %s", packet.Type(), peer)
		s.logger.LogErr(start, nil, buffer, peer, ErrUnknown, fmt.Errorf("unhandled message type: %s", packet.Type()))
	}
}

func (s *Server) handleV6RelayForward(start time.Time, packet dhcpv6.DHCPv6, peer *net.UDPAddr, cm *ipv6.ControlMessage) {
	var message DHCPMessage

	msg, err := packet.GetInnerMessage()
	if err != nil {
		glog.Errorf("Error getting inner message: %s", err)
		s.logger.LogErr(start, nil, packet.ToBytes(), peer, ErrParse, err)
		return
	}
	message.XID = msg.TransactionID[:]
	message.Peer = peer

	duid := msg.Options.ClientID()
	if duid == nil {
		errMsg := errors.New("failed to extract Client ID")
		glog.Errorf("%v", errMsg)
		s.logger.LogErr(start, nil, packet.ToBytes(), peer, ErrParse, errMsg)
		return
	}
	message.ClientID = duid.ToBytes()
	mac, err := dhcpv6.ExtractMAC(packet)
	if err != nil {
		glog.Errorf("Failed to extract MAC, drop due to %s", err)
		s.logger.LogErr(start, nil, packet.ToBytes(), peer, ErrParse, err)
		return
	}
	message.Mac = mac
	if vendorData, err := ztpv6.ParseVendorData(msg); err != nil {
		glog.V(2).Infof("Failed to extract vendor data: %s", err)
	} else {
		message.Serial = vendorData.Serial
	}

	server, err := selectDestinationServer(s.config, &message)
	if err != nil {
		glog.Errorf("%s, Drop due to %s", packet.Summary(), err)
		s.logger.LogErr(start, nil, packet.ToBytes(), peer, ErrNoServer, err)
		return
	}

	// If the packet was already relayed, we need to preserve the LinkAddr.
	// Otherwise, we are the first relay and LinkAddr should be zero.
	linkAddr := net.IPv6zero
	if relay, ok := packet.(*dhcpv6.RelayMessage); ok {
		linkAddr = relay.LinkAddr
	}

	relayMsg, err := dhcpv6.EncapsulateRelay(packet, dhcpv6.MessageTypeRelayForward, linkAddr, peer.IP)
	if err != nil {
		glog.Errorf("Failed to encapsulate relay message: %s", err)
		s.logger.LogErr(start, server, packet.ToBytes(), peer, ErrParse, err)
		return
	}
	s.sendToServer(start, server, relayMsg.ToBytes(), peer, cm)
}

func (s *Server) handleV6RelayRepl(start time.Time, packet dhcpv6.DHCPv6, peer *net.UDPAddr, cm *ipv6.ControlMessage) {
	// when we get a relay-reply, we need to unwind the message, removing the top
	// relay-reply info and passing on the inner part of the message
	msg, err := dhcpv6.DecapsulateRelay(packet)
	if err != nil {
		glog.Errorf("Failed to decapsulate packet, drop due to %s", err)
		s.logger.LogErr(start, nil, packet.ToBytes(), peer, ErrParse, err)
		return
	}
	peerAddr := packet.(*dhcpv6.RelayMessage).PeerAddr
	// send the packet to the peer addr
	addr := &net.UDPAddr{
		IP:   peerAddr,
		Port: dhcpv6.DefaultServerPort,
		Zone: "",
	}

	// Use the control message to send the reply on the correct interface.
	if cm != nil && cm.Dst != nil && !cm.Dst.IsMulticast() {
		cm.Src = cm.Dst
	}
	_, err = s.conn.v6.WriteTo(msg.ToBytes(), cm, addr)
	if err != nil {
		glog.Errorf("Error writing to %s: %s", addr, err)
		s.logger.LogErr(start, nil, msg.ToBytes(), addr, ErrWrite, err)
		return
	}
	s.logger.LogSuccess(start, nil, packet.ToBytes(), peer)
}

func (s *Server) handleV6Server(ctx context.Context, start time.Time, packet dhcpv6.DHCPv6, peer *net.UDPAddr, cm *ipv6.ControlMessage) {
	reply, err := s.config.Handler.ServeDHCPv6(ctx, packet)
	s.logger.LogSuccess(start, nil, packet.ToBytes(), peer)
	if err != nil {
		glog.Errorf("Error creating reply %s", err)
		s.logger.LogErr(start, nil, packet.ToBytes(), peer, fmt.Sprintf("%T", err), err)
		return
	}
	addr := &net.UDPAddr{
		IP:   peer.IP,
		Port: peer.Port,
		Zone: peer.Zone,
	}

	// Use the control message to send the reply on the correct interface.
	var outCm *ipv6.ControlMessage
	if cm != nil {
		outCm = &ipv6.ControlMessage{IfIndex: cm.IfIndex}
		// If the destination was a unicast address, use that as the source.
		// This mirrors the logic in the working DHCPv4 handler.
		if cm.Dst != nil && !cm.Dst.IsMulticast() {
			outCm.Src = cm.Dst
		} else if cm.IfIndex > 0 {
			// If the destination was multicast, we must find a suitable unicast
			// source address from the receiving interface.
			iface, err := net.InterfaceByIndex(cm.IfIndex)
			if err == nil {
				addrs, err := iface.Addrs()
				if err == nil {
					for _, addr := range addrs {
						if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To16() != nil && !ipnet.IP.IsLinkLocalUnicast() {
							// Found a global unicast address on the interface. Use it as the source.
							outCm.Src = ipnet.IP
							break
						}
					}
				}
			}
		}
	}
	_, err = s.conn.v6.WriteTo(reply.ToBytes(), outCm, addr)
	if err != nil {
		glog.Errorf("Error writing to %s: %s", addr, err)
		s.logger.LogErr(start, nil, reply.ToBytes(), addr, ErrWrite, err)
		return
	}
	s.logger.LogSuccess(start, nil, reply.ToBytes(), peer)
}
