/**
 * Copyright (c) Facebook, Inc. and its affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

package main

import (
	"encoding/json"

	"github.com/facebook/dhcplb/lib"
)

// DefaultConfigProvider holds configuration for the server.
type DefaultConfigProvider struct{}

// NewDefaultConfigProvider returns a new DefaultConfigProvider
func NewDefaultConfigProvider() *DefaultConfigProvider {
	return &DefaultConfigProvider{}
}

// NewHostSourcer returns a dhcplb.DHCPServerSourcer interface.
// The default config loader is able to instantiate a FileSourcer by itself, so
// NewHostSourcer here will simply return (nil, nil).
// The FileSourcer implemments dhcplb.DHCPServerSourcer interface.
// If you are writing your own implementation of dhcplb you could write your
// custom sourcer implementation here.
// sourcerType
// The NewHostSourcer function is passed values from the host_sourcer json
// config option with the sourcerType being the part of the string before
// the : and args the remaining portion.
// ex: file:hosts-v4.txt,hosts-v4-rc.txt in the json config file will have
// sourcerType="file" and args="hosts-v4.txt,hosts-v4-rc.txt".
func (h DefaultConfigProvider) NewHostSourcer(sourcerType, args string, version int) (dhcplb.DHCPServerSourcer, error) {
	return nil, nil
}

// ParseExtras is used to return extra config. Here we return nil because we
// don't need any extra configuration in the opensource version of dhcplb.
func (h DefaultConfigProvider) ParseExtras(data json.RawMessage) (interface{}, error) {
	return nil, nil
}

// NewDHCPBalancingAlgorithm returns a DHCPBalancingAlgorithm implementation.
// This can be used if you need to create your own balancing algorithm and
// integrate it with your infra without necesarily having to realase your code
// to github.
func (h DefaultConfigProvider) NewDHCPBalancingAlgorithm(version int) (dhcplb.DHCPBalancingAlgorithm, error) {
	return nil, nil
}

// NewHandler takes an interface with extra configurations and returns a
// Handler used for serving DHCP requests. It is only needed when using dhcplb
// in server mode.
func (h DefaultConfigProvider) NewHandler(extras interface{}, version int) (dhcplb.Handler, error) {
	return nil, nil
}
