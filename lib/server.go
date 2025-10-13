/**
 * Copyright (c) Facebook, Inc. and its affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

package dhcplb

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/golang/glog"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

// dhcplbConn holds either a v4 or a v6 packet connection.
// This is necessary because their ReadFrom/WriteTo methods have different
// signatures for the control message, making them incompatible with a
// single generic net.PacketConn interface.
type dhcplbConn struct {
	v4 *ipv4.PacketConn
	v6 *ipv6.PacketConn
}

// UDP acceptor
type Server struct {
	server        bool
	conn          *dhcplbConn
	logger        *loggerHelper
	config        *Config
	stableServers []*DHCPServer
	rcServers     []*DHCPServer
	throttle      *Throttle
}

// returns a pointer to the current config struct, so that if it does get changed while being used,
// it shouldn't affect the caller and this copy struct should be GC'ed when it falls out of scope
func (s *Server) GetConfig() *Config {
	return s.config
}

// ListenAndServe starts the server
func (s *Server) ListenAndServe(ctx context.Context) error {
	if !s.server {
		s.startUpdatingServerList()
	}

	glog.Infof("Started server, processing DHCP requests...")

	// Start a version-specific listening loop.
	if s.conn.v6 != nil {
		return s.readLoop6(ctx)
	}
	return s.readLoop4(ctx)
}

// SetConfig updates the server config
func (s *Server) SetConfig(config *Config) {
	glog.Infof("Updating server config")
	// update server list because Algorithm instance was recreated
	config.Algorithm.UpdateStableServerList(s.stableServers)
	config.Algorithm.UpdateRCServerList(s.rcServers)
	atomic.SwapPointer((*unsafe.Pointer)(unsafe.Pointer(&s.config)), unsafe.Pointer(config))
	// update the throttle rate
	s.throttle.setRate(config.Rate)
	glog.Infof("Updated server config")
}

// HasServers checks if the list of backend servers is not empty
func (s *Server) HasServers() bool {
	return len(s.stableServers) > 0 || len(s.rcServers) > 0
}

// NewServer initialized a Server before returning it.
func NewServer(config *Config, serverMode bool, personalizedLogger PersonalizedLogger) (*Server, error) {
	conn := &dhcplbConn{}
	if config.Version == 6 {
		glog.Info("Starting DHCPv6 server")
		udpConn, err := net.ListenUDP("udp6", config.Addr)
		if err != nil {
			return nil, err
		}
		connV6 := ipv6.NewPacketConn(udpConn)
		// Enable receiving the interface index in control messages, mirroring the v4 logic.
		if err := connV6.SetControlMessage(ipv6.FlagInterface, true); err != nil {
			udpConn.Close()
			return nil, fmt.Errorf("failed to set control message for IPv6: %w", err)
		}

		// Join DHCPv6 multicast group based on listening address.
		go func() {
			mcastAddr := net.ParseIP("ff02::1:2")
			addr := config.Addr
			if addr.IP.IsUnspecified() {
				// Listen on all interfaces
				interfaces, err := net.Interfaces()
				if err != nil {
					glog.Errorf("Failed to get network interfaces: %v", err)
					return
				}
				for _, ifi := range interfaces {
					if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagMulticast == 0 {
						continue // Interface is down or doesn't support multicast
					}
					if err := connV6.JoinGroup(&ifi, &net.UDPAddr{IP: mcastAddr}); err != nil {
						glog.Warningf("Failed to join multicast group on interface %s: %v", ifi.Name, err)
					} else {
						glog.Infof("Joined multicast group on interface %s", ifi.Name)
					}
				}
			} else {
				// Listen on a specific interface
				ifi, err := findInterfaceForIP(addr.IP)
				if err != nil {
					glog.Errorf("Could not find interface for address %s: %v", addr.IP, err)
					return
				}
				if err := connV6.JoinGroup(ifi, &net.UDPAddr{IP: mcastAddr}); err != nil {
					glog.Warningf("Failed to join multicast group on interface %s: %v", ifi.Name, err)
				} else {
					glog.Infof("Joined multicast group on interface %s", ifi.Name)
				}
			}
		}()

		conn.v6 = connV6
	} else {
		glog.Info("Starting DHCPv4 server")
		lc := net.ListenConfig{
			Control: func(network, address string, c syscall.RawConn) error {
				var opErr error
				err := c.Control(func(fd uintptr) {
					opErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
				})
				if err != nil {
					return err
				}
				return opErr
			},
		}
		packetConn, err := lc.ListenPacket(context.Background(), "udp4", config.Addr.String())
		if err != nil {
			return nil, fmt.Errorf("failed to listen on UDPv4 socket: %w", err)
		}
		connV4 := ipv4.NewPacketConn(packetConn)
		// Enable receiving the interface index in control messages
		if err := connV4.SetControlMessage(ipv4.FlagInterface, true); err != nil {
			packetConn.Close()
			return nil, fmt.Errorf("failed to set control message for IPv4: %w", err)
		}
		conn.v4 = connV4
	}

	// setup logger
	var loggerHelper = &loggerHelper{
		version:            config.Version,
		personalizedLogger: personalizedLogger,
	}

	server := &Server{
		server: serverMode,
		conn:   conn,
		logger: loggerHelper,
		config: config,
	}

	glog.Infof("Setting up throttle: Cache Size: %d - Cache Rate: %d - Request Rate: %d",
		config.CacheSize, config.CacheRate, config.Rate)
	throttle, err := NewThrottle(config.CacheSize, config.CacheRate, config.Rate)
	if err != nil {
		return nil, err
	}
	server.throttle = throttle

	return server, nil
}

func findInterfaceForIP(ip net.IP) (*net.Interface, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, ifi := range interfaces {
		addrs, err := ifi.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.Equal(ip) {
				return &ifi, nil
			}
		}
	}
	return nil, fmt.Errorf("no interface found for IP %s", ip)
}
