# How to setup your test environment with Vagrant

The instruction below will help you bringing up a virtual lab containing VMs
sharing their own private network(s).
This assumes you are somewhat familiar with
[`vagrant`](https://www.vagrantup.com/).
This has been tested under OSX but it should work find on Linux too.
Please provide feedback or PRs/patches if you find problems.
This instructions are for DHCPv4 only, DHCPv6 will follow soon.

## Install dependencies

First, install `chef-dk` from https://downloads.chef.io/chef-dk/ .
On OSX you can use `brew`:

```
$ brew cask install chef/chef/chefdk
```

Install `vagrant-berkshelf` plugin:

```
$ vagrant plugin install vagrant-berkshelf
$ cd ${PROJECT_ROOT}/vagrant/chef/cookbooks
$ berks install
```

You might need to disable dhcpserver for `vboxnet0` in VirtualBox:

```
$ VBoxManage dhcpserver remove --netname HostInterfaceNetworking-vboxnet0
```

## Start VMs

To start all the vms:

```
$ cd ${PROJECT_ROOT}/vagrant/
$ vagrant up
```

This will bring up the following VMs:

* `dhcpserver`: a VM running ISC `dhcpd` (both v4 and v6) configured with a
  subnet in the private network space. You can start as many as you want by
  changing the variable on top of the `Vagrantfile`.
* `dhcplb`: a VM running the `dhcplb` itself, configured to foward traffic to
  the above;
* `dhcprelay`: a VM running ISC `dhcrelay`, it intercepts broadcast/multicast
  traffic from the client below and relays traffic to the above;
* `dhcpclient`: a VM you can use to run `dhclient`, or `perfdhcp` manually to
  test things. It's DISCOVER/SOLICIT messages will be picked up by the
  `dhcprelay` instance

You can ssh into VMs using `vagrant ssh ${vm_name}`. Destroy them with
`vagrant destrory ${vm_name}`. If you find bugs in the `chef` cookbooks or you
want to change something there you can test your `chef` changes using
`vagrant provision ${vm_name}` on a running VM.

## Development cycle

Just edit `dhcplb`'s code on your host machine (the machine running VirtualBox
or whatever VM solution you are using). The root directory of your github
checkout will be mounted into the `dhcplb` VM at
`~/go/src/github.com/facebook/dhcplb`.

You can compile the binary using:

```
$ cd ~/go/src/github.com/facebook/dhcplb
$ go build
$ sudo mv dhcplb $GOBIN
```

And restart it with:

```
# initctl restart dhcplb
```

Logs will be in `/var/log/upstart/dhcplb.log` (becuase the current Vagrant image
uses a version of Ubuntu using Upstart init replacement).

On the `dhcpclient` you can initiate dhcp requests using these commands:

```
# perfdhcp -R 1 -4 -r 1200 -p 30 -t 1 -i 192.168.51.104
# dhclient -d -1 -v -pf /run/dhclient.eth1.pid -lf /var/lib/dhcp/dhclient.eth1.leases eth1
```

You will see:

```
root@dhcpclient:~# dhclient -d -1 -v -pf /run/dhclient.eth1.pid -lf
/var/lib/dhcp/dhclient.eth1.leases eth1
Internet Systems Consortium DHCP Client 4.2.4
Copyright 2004-2012 Internet Systems Consortium.
All rights reserved.
For info, please visit https://www.isc.org/software/dhcp/

Listening on LPF/eth1/08:00:27:7b:79:94
Sending on   LPF/eth1/08:00:27:7b:79:94
Sending on   Socket/fallback
DHCPDISCOVER on eth1 to 255.255.255.255 port 67 interval 3 (xid=0xcd1fdb2d)
DHCPREQUEST of 192.168.51.152 on eth1 to 255.255.255.255 port 67
(xid=0x2ddb1fcd)
DHCPOFFER of 192.168.51.152 from 192.168.51.104
DHCPACK of 192.168.51.152 from 192.168.51.104
RTNETLINK answers: File exists
bound to 192.168.51.152 -- renewal in 227 seconds.
^C
```

And something in the dhcplb logs:

```
I1125 15:54:11.985895   12190 modulo.go:65] List of available stable servers:
I1125 15:54:11.985943   12190 modulo.go:67] 192.168.50.104:67
I1125 15:54:11.985953   12190 modulo.go:67] 192.168.50.105:67
I1125 15:54:16.532833   12190 glog_logger.go:91] client_mac: 08:00:27:7b:79:94, dhcp_server: 192.168.50.104, giaddr: 192.168.51.101, latency_us: 112, server_is_rc: false, source_ip: 192.168.50.101, success: true, type: Discover, version: 4, xid: 0xcd1fdb2d
I1125 15:54:16.534310   12190 glog_logger.go:91] client_mac: 08:00:27:7b:79:94, dhcp_server: 192.168.50.104, giaddr: 192.168.51.101, latency_us: 117, server_is_rc: false, source_ip: 192.168.50.101, success: true, type: Request, version: 4, xid: 0xcd1fdb2d
```

[ISC KEA's
perfdhcp](https://kea.isc.org/wiki/DhcpBenchmarking) utility comes handy so it's
installed for your convenience.

Should you need to change something in the `dhcprelay` here are some useful
commands:

```
# initctl list
# initctl (stop|start|restart) isc-dhcp-relay
# /usr/sbin/dhcrelay -d -4 -i eth1 -i eth2 192.168.50.104
```

The relay config is in `/etc/default/isc-dhcp-relay`.

In general you don't need to touch the `dhcpserver` but you need to restart it
you can use:

```
# /etc/init.d/isc-dhcp-server restart
```

The main config is in `/etc/dhcp/dhcpd.conf`.
Subnets are configured like this should you need to change them:

```
subnet 192.168.50.0 netmask 255.255.255.0 {}
subnet 192.168.51.0 netmask 255.255.255.0 {range 192.168.51.220 192.168.51.230;}
```
