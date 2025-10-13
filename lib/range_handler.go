/**
 * Copyright (c) Facebook, Inc. and its affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

package dhcplb

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv6"
	"github.com/insomniacslk/dhcp/rfc1035label"
)

// Lease represents a DHCP lease record.
type Lease struct {
	IP         net.IP    `json:"ip"`
	Identifier string    `json:"identifier"` // MAC for v4, DUID for v6
	Expiration time.Time `json:"expiration"`
}

// RangeHandler is a simple handler that assigns IPs from a given range and manages leases from a JSON file.
type RangeHandler struct {
	StartIP    net.IP
	EndIP      net.IP
	LeaseTime  time.Duration
	Options    []dhcpv4.Option
	OptionsV6  []dhcpv6.Option
	LeaseFile  string
	leases     map[string]Lease // identifier -> Lease
	leaseLock  sync.Mutex
	ServerDUID *dhcpv6.DUIDLL
}

// NewRangeHandler creates a new RangeHandler.
func NewRangeHandler(startIP, endIP net.IP, leaseTime time.Duration, options map[string]any, leaseFile string) (*RangeHandler, error) {
	var opts []dhcpv4.Option
	var optsV6 []dhcpv6.Option

	for key, value := range options {
		switch key {
		case "subnet-mask":
			ip := net.ParseIP(value.(string))
			if ip != nil {
				// The dhcpv4.OptSubnetMask expects a net.IPMask, which is a []byte.
				// We need to convert the net.IP to a 4-byte representation for IPv4.
				mask := ip.To4()
				if mask != nil {
					opts = append(opts, dhcpv4.OptSubnetMask(net.IPMask(mask)))
				}
			}
		case "router":
			router := net.ParseIP(value.(string))
			if router != nil {
				opts = append(opts, dhcpv4.OptRouter(router))
			}
		case "dns-server":
			var dnsServers []net.IP
			for _, v := range value.([]any) {
				dns := net.ParseIP(v.(string))
				if dns != nil {
					dnsServers = append(dnsServers, dns)
				}
			}
			opts = append(opts, dhcpv4.OptDNS(dnsServers...))
			optsV6 = append(optsV6, dhcpv6.OptDNS(dnsServers...))
		case "domain-search":
			var domains []string
			for _, v := range value.([]any) {
				domains = append(domains, v.(string))
			}
			labels := &rfc1035label.Labels{
				Labels: domains,
			}
			opts = append(opts, dhcpv4.OptDomainSearch(labels))
			optsV6 = append(optsV6, dhcpv6.OptDomainSearchList(labels))
		}
	}

	duid, err := dhcpv6.GetDUIDLL()
	if err != nil {
		return nil, fmt.Errorf("failed to generate DUID: %w", err)
	}

	handler := &RangeHandler{
		StartIP:    startIP,
		EndIP:      endIP,
		LeaseTime:  leaseTime,
		Options:    opts,
		OptionsV6:  optsV6,
		LeaseFile:  leaseFile,
		leases:     make(map[string]Lease),
		ServerDUID: duid,
	}

	if err := handler.loadLeases(); err != nil {
		return nil, fmt.Errorf("failed to load leases: %w", err)
	}

	return handler, nil
}

func (h *RangeHandler) loadLeases() error {
	h.leaseLock.Lock()
	defer h.leaseLock.Unlock()

	data, err := os.ReadFile(h.LeaseFile)
	if err != nil {
		if os.IsNotExist(err) {
			glog.V(2).Infof("Lease file %s not found, starting with empty lease DB", h.LeaseFile)
			return nil
		}
		return err
	}

	if len(data) == 0 {
		glog.V(2).Infof("Lease file %s is empty, starting with empty lease DB", h.LeaseFile)
		return nil
	}

	return json.Unmarshal(data, &h.leases)
}

func (h *RangeHandler) saveLeases() error {
	h.leaseLock.Lock()
	defer h.leaseLock.Unlock()

	data, err := json.MarshalIndent(h.leases, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(h.LeaseFile, data, 0644)
}

func (h *RangeHandler) getFreeIP(identifier string) (net.IP, error) {
	h.leaseLock.Lock()
	defer h.leaseLock.Unlock()

	if lease, ok := h.leases[identifier]; ok && time.Now().Before(lease.Expiration) {
		return lease.IP, nil
	}

	leasedIPs := make(map[string]bool)
	for _, lease := range h.leases {
		if time.Now().Before(lease.Expiration) {
			leasedIPs[lease.IP.String()] = true
		}
	}

	for ip := h.StartIP; !ip.Equal(h.EndIP); ip = nextIP(ip) {
		if !leasedIPs[ip.String()] {
			return ip, nil
		}
	}

	return nil, fmt.Errorf("no free IP in range")
}

// ServeDHCPv4 handles DHCPv4 requests.
func (h *RangeHandler) ServeDHCPv4(ctx context.Context, req *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, error) {
	var reply *dhcpv4.DHCPv4
	var err error
	identifier := req.ClientHWAddr.String()

	switch req.MessageType() {
	case dhcpv4.MessageTypeDiscover:
		ip, err := h.getFreeIP(identifier)
		if err != nil {
			return nil, err
		}
		reply, err = dhcpv4.NewReplyFromRequest(req)
		if err != nil {
			return nil, err
		}
		reply.YourIPAddr = ip
		reply.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeOffer))
		reply.UpdateOption(dhcpv4.OptIPAddressLeaseTime(h.LeaseTime))

	case dhcpv4.MessageTypeRequest:
		ip := req.RequestedIPAddress()
		if ip.IsUnspecified() {
			ip, err = h.getFreeIP(identifier)
			if err != nil {
				return nil, err
			}
		}

		h.leases[identifier] = Lease{
			IP:         ip,
			Identifier: identifier,
			Expiration: time.Now().Add(h.LeaseTime),
		}
		if err := h.saveLeases(); err != nil {
			glog.Errorf("Failed to save leases: %v", err)
		}

		reply, err = dhcpv4.NewReplyFromRequest(req)
		if err != nil {
			return nil, err
		}
		reply.YourIPAddr = ip
		reply.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeAck))
		reply.UpdateOption(dhcpv4.OptIPAddressLeaseTime(h.LeaseTime))
	default:
		return nil, fmt.Errorf("unhandled message type: %v", req.MessageType())
	}

	for _, opt := range h.Options {
		reply.UpdateOption(opt)
	}

	return reply, nil
}

// ServeDHCPv6 handles DHCPv6 requests.
func (h *RangeHandler) ServeDHCPv6(ctx context.Context, req dhcpv6.DHCPv6) (dhcpv6.DHCPv6, error) {
	// Per RFC 8415, a server needs to be able to handle relayed messages.
	// GetInnerMessage will decapsulate any relay messages and return the
	// innermost DHCPv6 message, which is what we want to handle here.
	msg, err := req.GetInnerMessage()
	if err != nil {
		return nil, fmt.Errorf("failed to get inner message: %w", err)
	}
	identifier := msg.Options.ClientID().String()

	switch msg.Type() {
	case dhcpv6.MessageTypeSolicit:
		ip, err := h.getFreeIP(identifier)
		if err != nil {
			return nil, err
		}

		reply, err := dhcpv6.NewAdvertiseFromSolicit(msg)
		if err != nil {
			return nil, err
		}

		// Add IA_NA option
		iana := &dhcpv6.OptIANA{
			IaId: msg.Options.IANA()[0].IaId,
			T1:   h.LeaseTime / 2,                           // T1 is typically half the lease time
			T2:   time.Duration(float64(h.LeaseTime) * 0.8), // T2 is typically 80% of the lease time
		}

		iana.Options.Add(&dhcpv6.OptIAAddress{
			IPv6Addr:          ip,
			PreferredLifetime: h.LeaseTime,
			ValidLifetime:     h.LeaseTime,
		})

		reply.AddOption(iana)

		// Add Server ID
		reply.AddOption(dhcpv6.OptServerID(h.ServerDUID))

		for _, opt := range h.OptionsV6 {
			reply.AddOption(opt)
		}
		return reply, nil

	case dhcpv6.MessageTypeRequest:
		// For simplicity, this example ACKs the first address requested by the client.
		// A real server should validate that the requested address is appropriate.
		reqIA := msg.Options.IANA()[0]
		if len(reqIA.Options.Addresses()) == 0 {
			return nil, fmt.Errorf("no address requested in IANA")
		}
		ip := reqIA.Options.Addresses()[0].IPv6Addr

		h.leases[identifier] = Lease{
			IP:         ip,
			Identifier: identifier,
			Expiration: time.Now().Add(h.LeaseTime),
		}

		if err := h.saveLeases(); err != nil {
			glog.Errorf("Failed to save leases: %v", err)
		}

		reply, err := dhcpv6.NewReplyFromMessage(msg)
		if err != nil {
			return nil, err
		}

		// Add IA_NA option
		iana := &dhcpv6.OptIANA{
			IaId: msg.Options.IANA()[0].IaId,
			T1:   h.LeaseTime / 2,
			T2:   time.Duration(float64(h.LeaseTime) * 0.8),
		}

		iana.Options.Add(&dhcpv6.OptIAAddress{
			IPv6Addr:          ip,
			PreferredLifetime: h.LeaseTime,
			ValidLifetime:     h.LeaseTime,
		})

		reply.AddOption(iana)

		// Add Server ID
		reply.AddOption(dhcpv6.OptServerID(h.ServerDUID))

		for _, opt := range h.OptionsV6 {
			reply.AddOption(opt)
		}

		return reply, nil

	default:
		return nil, fmt.Errorf("unhandled message type: %v", msg.Type())
	}
}

func nextIP(ip net.IP) net.IP {
	next := make(net.IP, len(ip))
	copy(next, ip)
	for i := len(next) - 1; i >= 0; i-- {
		next[i]++
		if next[i] > 0 {
			break
		}
	}
	return next
}
