/**
 * Copyright (c) Facebook, Inc. and its affiliates.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE file in the root directory of this source tree.
 */

package dhcplb

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/golang/glog"
)

// ConfigProvider is an interface which provides methods to fetch the
// HostSourcer, parse extra configuration, provide additional load balancing
// implementations and how to handle dhcp requests in server mode
type ConfigProvider interface {
	NewHostSourcer(
		sourcerType, args string, version int) (DHCPServerSourcer, error)
	ParseExtras(extras json.RawMessage) (interface{}, error)
	NewDHCPBalancingAlgorithm(version int) (DHCPBalancingAlgorithm, error)
	NewHandler(extras interface{}, version int) (Handler, error)
}

// Config represents the server configuration.
type Config struct {
	Version              int
	Addr                 *net.UDPAddr
	Algorithm            DHCPBalancingAlgorithm
	ServerUpdateInterval time.Duration
	PacketBufSize        int
	Handler              Handler
	HostSourcer          DHCPServerSourcer
	RCRatio              uint32
	Overrides            map[string]Override
	Extras               interface{}
	CacheSize            int
	CacheRate            int
	Rate                 int
	ReplyAddr            *net.UDPAddr
}

// Override represents the dhcp server or the group of dhcp servers (tier) we
// want to send packets to.
type Override struct {
	// note that Host override takes precedence over Tier
	Host       string `json:"host"`
	Tier       string `json:"tier"`
	Expiration string `json:"expiration"`
}

// Overrides is a struct that holds v4 and v6 list of overrides.
// The keys of the map are mac addresses.
type Overrides struct {
	V4 map[string]Override `json:"v4"`
	V6 map[string]Override `json:"v6"`
}

// LoadConfig will take the path of the json file, the path of the override json
// file, an integer version and a ConfigProvider and will return a pointer to
// a Config object.
func LoadConfig(path, overridesPath string, version int, provider ConfigProvider) (*Config, error) {
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	overridesFile := []byte{}
	// path length of 0 means we aren't using overrides
	if len(overridesPath) != 0 {
		if overridesFile, err = os.ReadFile(overridesPath); err != nil {
			return nil, err
		}
	}
	return ParseConfig(file, overridesFile, version, provider)
}

// ParseConfig will take JSON config files, a version and a ConfigProvider,
// and return a pointer to a Config struct
func ParseConfig(jsonConfig, jsonOverrides []byte, version int, provider ConfigProvider) (*Config, error) {
	var combined combinedconfigSpec
	if err := json.Unmarshal(jsonConfig, &combined); err != nil {
		glog.Errorf("Failed to parse JSON: %s", err)
		return nil, err
	}
	var spec configSpec
	switch version {
	case 4:
		spec = combined.V4
	case 6:
		spec = combined.V6
	}

	var overrides map[string]Override
	if len(jsonOverrides) == 0 {
		overrides = make(map[string]Override)
	} else {
		var err error
		overrides, err = parseOverrides(jsonOverrides, version)
		if err != nil {
			glog.Errorf("Failed to load overrides: %s", err)
			return nil, err
		}
	}
	glog.Infof("Loaded %d override(s)", len(overrides))
	return newConfig(&spec, overrides, provider)
}

// WatchConfig will keep watching for changes to both config and override json
// files. It uses fsnotify library (it uses inotify in Linux), and call
// LoadConfig when it an inotify event signals the modification of the json
// files.
func WatchConfig(
	configPath, overridesPath string, version int, provider ConfigProvider,
) (chan *Config, error) {
	configChan := make(chan *Config)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	// strings containing the real path of a config files, if they are symlinks
	var realConfigPath string
	var realOverridesPath string

	err = watcher.Add(filepath.Dir(configPath))
	if err != nil {
		return nil, err
	}
	realConfigPath, err = filepath.EvalSymlinks(configPath)
	if err == nil {
		// configPath is a symlink, also watch the pointee
		err = watcher.Add(realConfigPath)
		if err != nil {
			return nil, err
		}
	}

	// setup watcher on overrides file if present
	if len(overridesPath) > 0 {
		err = watcher.Add(filepath.Dir(overridesPath))
		if err != nil {
			return nil, err
		}
		realOverridesPath, err = filepath.EvalSymlinks(overridesPath)
		if err == nil {
			// overridesPath is a symlink, also watch the pointee
			err = watcher.Add(realOverridesPath)
			if err != nil {
				return nil, err
			}
		}
	}

	// watch for fsnotify events
	go func() {
		for {
			select {
			case ev := <-watcher.Events:
				// ignore Remove events
				if ev.Op&fsnotify.Remove == fsnotify.Remove {
					continue
				}
				// only care about symlinks and target of symlinks
				if ev.Name == overridesPath || ev.Name == configPath ||
					ev.Name == realOverridesPath || ev.Name == realConfigPath {
					glog.Infof("Configuration file changed (%s), reloading", ev)
					config, err := LoadConfig(
						configPath, overridesPath, version, provider)
					if err != nil {
						glog.Fatalf("Failed to reload config: %s", err)
						panic(err) // fail hard
					}
					configChan <- config
				}
			case err := <-watcher.Errors:
				glog.Errorf("fsnotify error: %s", err)
			}
		}
	}()

	return configChan, nil
}

// configSpec holds the raw json configuration.
type configSpec struct {
	Path                 string
	Version              int             `json:"version"`
	ListenAddr           string          `json:"listen_addr"`
	Port                 int             `json:"port"`
	AlgorithmName        string          `json:"algorithm"`
	UpdateServerInterval int             `json:"update_server_interval"`
	PacketBufSize        int             `json:"packet_buf_size"`
	HostSourcer          string          `json:"host_sourcer"`
	RCRatio              uint32          `json:"rc_ratio"`
	Extras               json.RawMessage `json:"extras"`
	CacheSize            int             `json:"throttle_cache_size"`
	CacheRate            int             `json:"throttle_cache_rate"`
	Rate                 int             `json:"throttle_rate"`
	ReplyAddr            string          `json:"reply_addr"`
}

type combinedconfigSpec struct {
	V4 configSpec `json:"v4"`
	V6 configSpec `json:"v6"`
}

func (c *configSpec) sourcer(provider ConfigProvider) (DHCPServerSourcer, error) {
	// Load the DHCPServerSourcer implementation
	sourcerInfo := strings.Split(c.HostSourcer, ":")
	sourcerType := sourcerInfo[0]
	stable := sourcerInfo[1]
	rc := ""
	if strings.Contains(sourcerInfo[1], ",") {
		sourcerArgs := strings.Split(sourcerInfo[1], ",")
		stable = sourcerArgs[0]
		rc = sourcerArgs[1]
	}
	switch sourcerType {

	default:
		return provider.NewHostSourcer(sourcerType, sourcerInfo[1], c.Version)

	case "file":
		sourcer, err := NewFileSourcer(stable, rc, c.Version)
		if err != nil {
			glog.Fatalf("Can't load FileSourcer")
		}
		return sourcer, err
	}
}

func (c *configSpec) algorithm(provider ConfigProvider) (DHCPBalancingAlgorithm, error) {
	// Balancing algorithms coming with the dhcplb source code
	modulo := new(modulo)
	rr := new(roundRobin)
	algorithms := map[string]DHCPBalancingAlgorithm{
		modulo.Name(): modulo,
		rr.Name():     rr,
	}
	// load other non default algorithms from the ConfigProvider
	providedAlgo, err := provider.NewDHCPBalancingAlgorithm(c.Version)
	if err != nil {
		glog.Fatalf("Provided load balancing implementation error: %s", err)
	}
	if providedAlgo != nil {
		if _, exists := algorithms[providedAlgo.Name()]; exists {
			glog.Fatalf("Algorithm name %s exists already, pick another name.", providedAlgo.Name())

		}
		algorithms[providedAlgo.Name()] = providedAlgo
	}
	lb, ok := algorithms[c.AlgorithmName]
	if !ok {
		supported := []string{}
		for k := range algorithms {
			supported = append(supported, k)
		}
		glog.Fatalf(
			"'%s' is not a supported balancing algorithm. "+
				"Supported balancing algorithms are: %v",
			c.AlgorithmName, supported)
		return nil, fmt.Errorf(
			"'%s' is not a supported balancing algorithm", c.AlgorithmName)
	}
	lb.SetRCRatio(c.RCRatio)
	return lb, nil
}

func newConfig(spec *configSpec, overrides map[string]Override, provider ConfigProvider) (*Config, error) {
	if spec.Version != 4 && spec.Version != 6 {
		return nil, fmt.Errorf("supported version: 4, 6 - not %d", spec.Version)
	}

	targetIP := net.ParseIP(spec.ListenAddr)
	if targetIP == nil {
		return nil, fmt.Errorf("unable to parse IP %s", targetIP)
	}
	addr := &net.UDPAddr{
		IP:   targetIP,
		Port: spec.Port,
		Zone: "",
	}

	algo, err := spec.algorithm(provider)
	if err != nil {
		return nil, err
	}
	sourcer, err := spec.sourcer(provider)
	if err != nil {
		return nil, err
	}

	// extras
	extras, err := provider.ParseExtras(spec.Extras)
	if err != nil {
		return nil, err
	}
	handler, err := provider.NewHandler(extras, spec.Version)
	if err != nil {
		return nil, err
	}

	return &Config{
		Version:   spec.Version,
		Addr:      addr,
		Algorithm: algo,
		ServerUpdateInterval: time.Duration(
			spec.UpdateServerInterval) * time.Second,
		PacketBufSize: spec.PacketBufSize,
		Handler:       handler,
		HostSourcer:   sourcer,
		RCRatio:       spec.RCRatio,
		Overrides:     overrides,
		Extras:        extras,
		CacheSize:     spec.CacheSize,
		CacheRate:     spec.CacheRate,
		Rate:          spec.Rate,
		ReplyAddr:     &net.UDPAddr{IP: net.ParseIP(spec.ReplyAddr)},
	}, nil
}

func parseOverrides(file []byte, version int) (map[string]Override, error) {
	overrides := Overrides{}
	err := json.Unmarshal(file, &overrides)
	if err != nil {
		glog.Errorf("Failed to parse JSON: %s", err)
		return nil, err
	}
	switch version {
	case 4:
		return overrides.V4, nil
	case 6:
		return overrides.V6, nil
	}
	return nil, fmt.Errorf("unsupported version %d, must be 4|6", version)
}
