/**
 * Copyright (c) Facebook, Inc. and its affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

package main

import (
	"fmt"
	"net"
	"sort"
	"strings"

	dhcplb "github.com/facebook/dhcplb/lib"
	"github.com/golang/glog"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv6"
)

type glogLogger struct{}

// NewGlogLogger returns a glogLogger struct based on the
// dhcplb.PersonalizedLogger interface.
func NewGlogLogger() dhcplb.PersonalizedLogger {
	return glogLogger{}
}

// Log takes a dhcplb.LogMessage, creates a sample map[string] containing
// information about the served request and prints it to stdout/err.
func (l glogLogger) Log(msg dhcplb.LogMessage) error {
	sample := map[string]interface{}{
		"version":      msg.Version,
		"dhcp_server":  msg.Server,
		"server_is_rc": msg.ServerIsRC,
		"source_ip":    msg.Peer.IP.String(),
		"success":      msg.Success,
		"latency_us":   msg.Latency.Nanoseconds() / 1000,
	}
	if msg.ErrorName != "" {
		sample["error_name"] = msg.ErrorName
		sample["error_details"] = fmt.Sprintf("%s", msg.ErrorDetails)
	}

	if msg.Packet != nil {
		if msg.Version == 4 {
			packet, err := dhcpv4.FromBytes(msg.Packet)
			if err != nil {
				glog.Errorf("Error decoding DHCPv4 packet: %s", err)
				return err
			}
			sample["type"] = packet.MessageType().String()
			sample["xid"] = packet.TransactionID.String()
			sample["giaddr"] = packet.GatewayIPAddr.String()
			sample["client_mac"] = packet.ClientHWAddr.String()
		} else if msg.Version == 6 {
			packet, err := dhcpv6.FromBytes(msg.Packet)
			if err != nil {
				glog.Errorf("Error decoding DHCPv6 packet: %s", err)
				return err
			}
			sample["type"] = packet.Type().String()
			msg, err := packet.GetInnerMessage()
			if err != nil {
				glog.Errorf("Failed to get inner packet: %s", err)
				return err
			}
			sample["xid"] = msg.TransactionID.String()
			if duid := msg.Options.ClientID(); duid != nil {
				sample["duid"] = net.HardwareAddr(duid.ToBytes()).String()
				sample["duid_type"] = duid.DUIDType().String()
			}
			if mac, err := dhcpv6.ExtractMAC(packet); err != nil {
				glog.Errorf("error getting mac: %s", err)
			} else {
				sample["client_mac"] = mac.String()
			}
			if packet.IsRelay() {
				relay := packet.(*dhcpv6.RelayMessage)
				sample["link-addr"] = relay.LinkAddr.String()
				sample["peer-addr"] = relay.PeerAddr.String()
			}
		}
	}

	// Order samples by key, store them into logline slice
	keys := make([]string, len(sample))
	i := 0
	for key := range sample {
		keys[i] = key
		i++
	}
	sort.Strings(keys)
	logline := make([]string, len(sample))
	i = 0
	for k := range keys {
		logline[i] = fmt.Sprintf("%s: %+v", keys[k], sample[keys[k]])
		i++
	}

	if msg.Success {
		glog.Infof("%s", strings.Join(logline, ", "))
	} else {
		glog.Errorf("%s", strings.Join(logline, ", "))
	}
	return nil
}
