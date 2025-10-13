/**
 * Copyright (c) Facebook, Inc. and its affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

package main

import (
	"encoding/json"
	"fmt"
	"net"
	"time"

	dhcplb "github.com/facebookincubator/dhcplb/lib"
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
// in server mode. Here we return the RangeHandler implementation as an example.
func (h DefaultConfigProvider) NewHandler(extras interface{}, version int) (dhcplb.Handler, error) {
	if extras == nil {
		return nil, nil
	}

	// Define a struct to parse the handler config from the main JSON
	type HandlerConfig struct {
		Type      string                 `json:"type"`
		StartIP   string                 `json:"start_ip"`
		EndIP     string                 `json:"end_ip"`
		LeaseTime string                 `json:"lease_time"`
		LeaseFile string                 `json:"lease_file"`
		Options   map[string]interface{} `json:"options"`
	}

	var handlerConf HandlerConfig
	configBytes, err := json.Marshal(extras)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal handler config: %w", err)
	}
	if err := json.Unmarshal(configBytes, &handlerConf); err != nil {
		return nil, fmt.Errorf("failed to unmarshal handler config: %w", err)
	}

	if handlerConf.Type == "range" {
		startIP := net.ParseIP(handlerConf.StartIP)
		if startIP == nil {
			return nil, fmt.Errorf("invalid start_ip: %s", handlerConf.StartIP)
		}
		endIP := net.ParseIP(handlerConf.EndIP)
		if endIP == nil {
			return nil, fmt.Errorf("invalid end_ip: %s", handlerConf.EndIP)
		}
		leaseTime, err := time.ParseDuration(handlerConf.LeaseTime)
		if err != nil {
			return nil, fmt.Errorf("invalid lease_time: %w", err)
		}

		return dhcplb.NewRangeHandler(startIP, endIP, leaseTime, handlerConf.Options, handlerConf.LeaseFile)
	}

	return nil, nil
}
