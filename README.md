# What is dhcplb?

`dhcplb` is Facebook's implementation of:
  * a DHCP v4/v6 relayer with load balancing capabilities
  * a DHCP v4/v6 server framework

Both modes currently only support handling messages sent by a relayer which is
unicast traffic. It doesn't support broadcast (v4) and multicast (v6) requests.
Facebook currently uses it in production, and it's deployed at global scale
across all of our data centers.
It is based on [@insomniacslk](https://github.com/insomniacslk) [dhcp library](https://github.com/insomniacslk/dhcp).

# Why did you do that?

Facebook uses DHCP to provide network configuration to bare-metal machines at
provisioning phase and to assign IPs to out-of-band interfaces.

`dhcplb` was created because the previous infrastructure surrounding DHCP led
to very unbalanced load across the DHCP servers in a region when simply using
Anycast+ECMP alone (for example 1 server out of 10 would receive >65% of
requests).

Facebook's DHCP infrastructure was [presented at SRECon15 Ireland](https://www.usenix.org/conference/srecon15europe/program/presentation/failla).

Later, support for making it responsible for serving dhcp requests (server mode)
was added. This was done because having a single threaded application (ISC KEA)
queuing up packets while doing backend calls to another services wasn't scaling
well for us.

# Why not use an existing load balancer?

* All the relayer implementations available on the internet lack the load
balancing functionality.
* Having control of the code gives you the ability to:
  * perform A/B testing on new builds of our DHCP server
  * implement override mechanism
  * implement anything additional you need

# Why not use an existing server?

We needed a server implementation which allow us to have both:
* Multithreaded design, to avoid blocking requests when doing backend calls
* An interface to be able to call other services for getting the IP assignment,
boot file url, etc.

# How do you use `dhcplb` at Facebook?

This picture shows how we have deployed `dhcplb` in our production
infrastructure:

![DHCPLB deployed at Facebook](/docs/dhcplb-fb-deployment.jpg)

TORs (Top of Rack switch) at Facebook run DHCP relayers, these relayers are
responsible for relaying broadcast DHCP traffic (DISCOVERY and SOLICIT
messages) originating within their racks to anycast VIPs, one DHCPv4 and one
for DHCPv6.

In a Cisco switch the configuration would look like this:

```
ip helper-address 10.127.255.67
ipv6 dhcp relay destination 2401:db00:eef0:a67::
```

We have a bunch of `dhcplb` [Tupperware](https://blog.docker.com/2014/07/dockercon-video-containerized-deployment-at-facebook/) instances in every region listening on
those VIPs.
They are responsible for received traffic relayed by TORs agents and load
balancing them amongst the actual `dhcplb` servers distributed across clusters
in that same region.

Having 2 layers allows us to A/B test changes of the server implementation.

The configuration for `dhcplb` consists of 3 files:

* json config file: contains the main configuration for the server as explained in the [Getting Started](docs/getting-started.md) section
* host lists file: contains a list of dhcp servers, one per line, those are the servers `dhcplb` will try to balance on
* overrides file: a file containing per mac overrides. See the [Getting Started](docs/getting-started.md) section.

# TODOs / future improvements

`dhcplb` does not support relaying/responding broadcasted DHCPv4 DISCOVERY
packets or DHCPv6 SOLICIT packets sent to `ff02::1:2` multicast address. We
don't need this in our production environment but adding that support should be
trivial though.

TODOs and improvements are tracked [here](https://github.com/facebook/dhcplb/issues?q=is%3Aissue+is%3Aopen+label%3Aenhancement)

PRs are welcome!

# How does the packet path looks like?

When operating in v4 `dhcplb` will relay relayed messages coming from other
relayers (in our production network those are rack switches), the response from
the server will be relayed back to the rack switches:

```
dhcp client <---> rsw relayer ---> dhcplb (relay) ---> dhcplb (server)
                      ^                                      |
                      |                                      |
                      +--------------------------------------+
```

In DHCPv6 responses by the dhcp server will traverse the load balancer.

# Installation

To install `dhcplb` into `$GOPATH/bin/dhcplb`, simply run:

```
$ go install github.com/facebook/dhcplb@latest
```

# Cloning

If you wish to clone the repo you can do the following:

```
$ mkdir -p $GOPATH/src/github.com/facebook
$ cd $_
$ git clone https://github.com/facebook/dhcplb
$ go install github.com/facebook/dhcplb
```

# Run unit tests

You can run tests with:

```
$ cd $GOPATH/src/github.com/facebook/dhcplb/lib
$ go test
```

# Getting Started and extending `dhcplb`

`dhcplb` can be run out of the box after compilation.

To start immediately, you can run
`sudo dhcplb -config config.json -version 6`.
That will start the relay in v6 mode using the default configuration.

Should you need to integrate `dhcplb` with your infrastructure please
see [Extending DHCPLB](docs/extending-dhcplb.md).

# Virtual lab for development and testing

You can bring up a virtual lab using vagrant. This will replicate our production
environment, you can spawn VMs containing various components like:

* N instances of `ISC dhcpd`
* An instance of `dhcplb`
* An instance of `dhcrelay`, simulating a top of rack switch.
* a VM where you can run `dhclient` or `ISC perfdhcp`

All of that is managed by `vagrant` and `chef-solo` cookbooks.
You can use this lab to test your `dhcplb` changes.
For more information have a look at the [vagrant directory](vagrant/README.md).

# Who wrote it?

`dhcplb` started in April 2016 during a 3 days hackathon in the Facebook
Dublin office, the hackathon project proved the feasibility of the tool.
In June we were joined by Vinnie Magro (@vmagro) for a 3 months internship in
which he worked with two production engineers on turning the hack into a
production ready system.

Original Hackathon project members:

* Angelo Failla ([@pallotron](https://github.com/pallotron)), Production Engineer
* Roman Gushchin ([@rgushchin](https://github.com/rgushchin)), Production Engineer
* Mateusz Kaczanowski ([@mkaczanowski](https://github.com/mkaczanowski)), Production Engineer
* Jake Bunce, Network Engineer

Internship project members:

* Vinnie Magro ([@vmagro](https://github.com/vmagro)), Production Engineer intern
* Angelo Failla (@pallotron), Intern mentor, Production Engineer
* Mateusz Kaczanowski (@mkaczanowski), Production Engineer

Other contributors:

* Emre Cantimur, Production Engineer, Facebook, Throttling support
* Andrea Barberio, Production Engineer, Facebook
* Pablo Mazzini, Production Engineer, Facebook

# License

BSD License. See the LICENSE file.
