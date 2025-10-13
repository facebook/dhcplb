package dhcplb

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	"github.com/insomniacslk/dhcp/dhcpv6"
	"github.com/stretchr/testify/assert"
)

func TestServeDHCPv6(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "leases.json")
	assert.NoError(t, err)
	defer os.Remove(tmpfile.Name())

	h, err := NewRangeHandler(
		net.ParseIP("::1"),
		net.ParseIP("::10"),
		time.Hour,
		map[string]interface{}{
			"dns-server": []interface{}{"::2"},
		},
		tmpfile.Name(),
	)
	assert.NoError(t, err)

	// Solicit
	solicit, err := dhcpv6.NewSolicit(net.HardwareAddr{0x01, 0x02, 0x03, 0x04, 0x05, 0x06})
	assert.NoError(t, err)
	solicit.AddOption(&dhcpv6.OptIANA{})

	adv, err := h.ServeDHCPv6(context.Background(), solicit)
	assert.NoError(t, err)
	assert.Equal(t, dhcpv6.MessageTypeAdvertise, adv.Type())

	// Check important fields in Advertise
	advMsg := adv.(*dhcpv6.Message)
	assert.NotNil(t, advMsg.Options.ServerID())
	assert.NotNil(t, advMsg.Options.ClientID())
	iana := advMsg.Options.IANA()
	assert.NotNil(t, iana)
	assert.Len(t, iana, 1)
	addrs := iana[0].Options.Addresses()
	assert.Len(t, addrs, 1)
	assert.True(t, addrs[0].IPv6Addr.Equal(net.ParseIP("::1")))
	dns := advMsg.Options.DNS()
	assert.Len(t, dns, 1)
	assert.True(t, dns[0].Equal(net.ParseIP("::2")))

	// Request
	req, err := dhcpv6.NewRequestFromAdvertise(adv.(*dhcpv6.Message))
	assert.NoError(t, err)
	req.AddOption(advMsg.Options.IANA()[0])

	reply, err := h.ServeDHCPv6(context.Background(), req)
	assert.NoError(t, err)
	assert.Equal(t, dhcpv6.MessageTypeReply, reply.Type())

	// Check important fields in Reply
	replyMsg := reply.(*dhcpv6.Message)
	assert.NotNil(t, replyMsg.Options.ServerID())
	assert.NotNil(t, replyMsg.Options.ClientID())
	iana = replyMsg.Options.IANA()
	assert.NotNil(t, iana)
	assert.Len(t, iana, 1)
	addrs = iana[0].Options.Addresses()
	assert.Len(t, addrs, 1)
	assert.True(t, addrs[0].IPv6Addr.Equal(net.ParseIP("::1")))
	dns = replyMsg.Options.DNS()
	assert.Len(t, dns, 1)
	assert.True(t, dns[0].Equal(net.ParseIP("::2")))
}
