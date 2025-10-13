//go:build !linux

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

	"github.com/insomniacslk/dhcp/dhcpv4"
)

// sendEthernet is a stub for non-Linux systems. Raw socket packet sending
// is not supported on this platform.
func sendEthernet(_ net.Interface, _ *dhcpv4.DHCPv4) error {
	return fmt.Errorf("raw socket DHCP packet sending not supported on this OS")
}
