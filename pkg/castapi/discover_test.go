package castapi

import (
	"context"
	"net"
	"testing"

	"github.com/grandcat/zeroconf"
	"github.com/miekg/dns"
)

// The desktop advertises with zeroconf; a unicast browse must find it.
func TestDiscoverFindsAZeroconfAdvertisement(t *testing.T) {
	server, err := zeroconf.Register("test instance", "_frogcast._tcp", "local.", 45123, []string{"app=frog-cast"}, nil)
	if err != nil {
		t.Skipf("can't advertise here: %v", err)
	}
	defer server.Shutdown()

	found, err := discover(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, inst := range found {
		if inst.Name == "test instance" {
			if inst.Port != 45123 || inst.Host == "" {
				t.Fatalf("instance = %+v", inst)
			}
			return
		}
	}
	t.Fatalf("test instance not found in %+v", found)
}

func TestInstanceAddressIsTheOneItAnsweredFrom(t *testing.T) {
	s := newAnswerSet()
	instance := "fedora." + service
	msg := new(dns.Msg)
	msg.Answer = []dns.RR{
		&dns.PTR{Hdr: dns.RR_Header{Name: service, Rrtype: dns.TypePTR}, Ptr: instance},
		&dns.SRV{Hdr: dns.RR_Header{Name: instance, Rrtype: dns.TypeSRV}, Target: "fedora.local.", Port: 9999},
	}
	msg.Extra = []dns.RR{
		&dns.A{Hdr: dns.RR_Header{Name: "fedora.local.", Rrtype: dns.TypeA}, A: net.IPv4(192, 168, 1, 107)},
		&dns.A{Hdr: dns.RR_Header{Name: "fedora.local.", Rrtype: dns.TypeA}, A: net.IPv4(172, 17, 0, 1)},
	}
	s.add(msg, net.IPv4(192, 168, 1, 107))
	found := s.instances(map[string]bool{})
	if len(found) != 1 || found[0].Host != "192.168.1.107" {
		t.Fatalf("found %+v, want the LAN address", found)
	}
}
