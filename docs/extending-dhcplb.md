# Extending DHCPLB

It's possible to extend `dhcplb` to modify the way it fetches the list of
DHCP servers, or have a different logging implementation, or add different
balancing algorithm, or make it behave as a server, replying to requests
directly.
At the moment this is a bit complex but we will work on ways to make it easier.

## Adding a new balancing algorithm.

Adding a new algorithm can be done by implementing something that matches
the `DHCPBalancingAlgorithm` interface:

```go
type DHCPBalancingAlgorithm interface {
  selectServerFromList(list []*DHCPServer, message *DHCPMessage) (*DHCPServer, error)
  selectRatioBasedDhcpServer(message *DHCPMessage) (*DHCPServer, error)
  updateStableServerList(list []*DHCPServer) error
  updateRCServerList(list []*DHCPServer) error
  setRCRatio(ratio uint32)
  Name() string
}
```

Then add it to the `algorithms` map in the `configSpec.algorithm` function, in
the `config.go` file.
Do that if you want to share the algorithm with the community.

If, however, you need to implement something that you can't share, because, for
example, it's internal and specific to your infra, you can write something that
implements the `ConfigProvider` interface, in particular the
`NewDHCPBalancingAlgorithm` function.

## Adding more configuration options.

More configuration options can be added to the config JSON file using the
`ConfigProvider` interface:

```go
type ConfigProvider interface {
  NewHostSourcer(sourcerType, args string, version int) (DHCPServerSourcer, error)
  ParseExtras(extras json.RawMessage) (interface{}, error)
  NewDHCPBalancingAlgorithm(version int) (DHCPBalancingAlgorithm, error)
  NewHandler(extras interface{}, version int) (Handler, error)
}
```

The `NewHostSourcer` function is passed values from the `host_sourcer` config option
with the `sourcerType` being the part of the string before the `:` and `args` the
remaining portion. ex: `file:hosts-v4.txt,hosts-v4-rc.txt` will have `sourcerType="file"`
and `args="hosts-v4.txt,hosts-v4-rc-txt"`.
The default `Config` loader is able to instantiate a `FileSourcer` by itself, so
`NewHostSourcer` can simply return `nil, nil` unless you are using a custom sourcer
implementation.

Any struct can be returned from the `ParseExtras` function and used elsewhere in
the code via the `Extras` member of a `Config` struct.

As mentioned in the section before `NewDHCPBalancingAlgorithm` can be used
to return your own specific load balancing implementation.

## Write your own logic to source list of DHCP servers

If you want to change the way `dhcplb` sources the list of DHCP servers (for
example you want to source them from a backend system like a database) you can
have something implementing the `DHCPServerSourcer` interface:

```go
type DHCPServerSourcer interface {
  GetStableServers() ([]*DHCPServer, error)
  GetRCServers() ([]*DHCPServer, error)
  // get servers from a specific named group (this is used with overrides)
  GetServersFromTier(tier string) ([]*DHCPServer, error)
}
```

Then implement your own `ConfigProvider` interface and make it return a
`DHCPServerSourcer`. Then in the main you can replace `NewDefaultConfigProvider`
with your own `ConfigProvider` implementation.

## Write your own server handler

If you want to make `dhcplb` responsible for serving dhcp requests you can implement
the `Handler` interface. The methods of the interface take an incoming packet and
return the crafted response the server is going to reply with.

```go
type Handler interface {
  ServeDHCPv4(packet *dhcpv4.DHCPv4) (*dhcpv4.DHCPv4, error)
  ServeDHCPv6(packet dhcpv6.DHCPv6) (dhcpv6.DHCPv6, error)
}
```

Then implement your own `ConfigProvider` interface and make it return a `Handler`
interface. `dhcplb` should be started in server mode using the `-server` flag.

When creating a `Handler`, the `Extra` configuration options are passed to it, so
things such as DNS, NTP servers or lease time can be defined there.

When `dhcplb` is used to serve requests directly, the `DHCPServerSourcer` and
`DHCPBalancingAlgorithm` interfaces are not used.

### Example

Define a config provider with its methods and the extra configuration options.

```go
// MyConfigProvider implements the ConfigProvider interface
type MyConfigProvider struct {
}

// MyConfigExtras represents extra configuration options
type MyConfigExtras struct {
  NameServers []string `json:"name_servers"`
  LeaseTime   uint32   `json:"lease_time_s"`
}

// ParseExtras is responsible for parsing extra configuration options
func (p MyConfigProvider) ParseExtras(extrasJSON json.RawMessage) (interface{}, error) {
  var extras MyConfigExtras
  if err := json.Unmarshal(extrasJSON, &extras); err != nil {
    return nil, fmt.Errorf("Error parsing extras JSON: %s", err)
  }
  return extras, nil
}

// NewHandler returns the handler for serving DHCP requests
func (p MyConfigProvider) NewHandler(extras interface{}, version int) (Handler, error) {
  config, ok := extras.(MyConfigExtras)
  if !ok {
    return nil, fmt.Errorf("MyConfigExtras type assertion error")
  }
  return &MyHandler{config: config}, nil
}
```

Define a server handler and its methods.

```go
// MyHandler contains data needed to handle DHCP requests
type MyHandler struct {
  config MyConfigExtras
}

// ServeDHCPv6 handles DHCPv6 requests
func (h MyHandler) ServeDHCPv6(packet dhcpv6.DHCPv6) (dhcpv6.DHCPv6, error) {
  msg, err := packet.GetInnerMessage()
  if err != nil {
    return nil, err
  }

  mac, err := dhcpv6.ExtractMAC(packet)
  if err != nil {
    return nil, err
  }

  reply, err := buildReply(msg)
  if err != nil {
    return nil, err
  }

  ...

  var nameservers []net.IP
  for _, ns := range h.config.NameServers {
    nameservers = append(nameservers, net.ParseIP(ns))
  }
  reply.AddOption(&dhcpv6.OptDNSRecursiveNameServer{NameServers: nameservers})

  ...

  return reply, nil
}
```

Create a configuration file and watch the config changes.

```
{
  "v6": {
    "version": 6,
    "listen_addr": "::",
    "port": 547,
    "packet_buf_size": 1024,
    "update_server_interval": 30,
    "algorithm": "xid",
    "host_sourcer": "file:hosts-v6.txt",
    "rc_ratio": 0,
    "throttle_cache_size": 1024,
    "throttle_cache_rate": 128,
    "throttle_rate": 256,
    "extras": {
      "name_servers": [
        "2001:4860:4860::8888",
        "2001:4860:4860::8844"
      ],
      "lease_time_s": 43200
    }
  }
}
```

```go
  configChan, err := dhcplb.WatchConfig(
    *configPath, *overridesPath, *version, &MyConfigProvider{})
```
## Concrete example

The project root directory contains a concrete example that can be used as a
starting point.

- `main.go`: main entry point where all objects are instantiated
- `config_provider.go`: config provider implementation
- `lib/range_handler.go`: an example implementation of the `Handler` interface, it implements DHCPv4 and DHCPv6 handlers using JSON lease files to assign and revoke IPs.