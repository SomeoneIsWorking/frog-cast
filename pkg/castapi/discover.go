package castapi

import (
	"context"
	"fmt"
	"net"
	"slices"
	"strings"
	"time"

	"github.com/miekg/dns"
)

const (
	service      = "_frogcast._tcp.local."
	browseWindow = 3 * time.Second
)

var mdnsGroup = &net.UDPAddr{IP: net.IPv4(224, 0, 0, 251), Port: 5353}

// Discover finds frog-cast instances on the LAN. It sends one-shot mDNS
// queries from an ephemeral port, which responders answer by unicast
// (RFC 6762 §6.7). That needs no interface list and no multicast lock, which
// Android denies to apps.
func Discover(ctx context.Context) ([]CastInstance, error) {
	return discover(ctx, localIPSet())
}

// discover browses, leaving out instances at the skip addresses.
func discover(ctx context.Context, skip map[string]bool) ([]CastInstance, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return nil, fmt.Errorf("mDNS socket: %w", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(ctx, browseWindow)
	defer cancel()
	deadline, _ := ctx.Deadline()
	conn.SetReadDeadline(deadline)
	go func() {
		<-ctx.Done()
		conn.SetReadDeadline(time.Now())
	}()

	if err := sendQuery(conn, dns.TypePTR, service); err != nil {
		return nil, fmt.Errorf("mDNS query: %w", err)
	}

	answers := newAnswerSet()
	asked := map[string]bool{}
	buf := make([]byte, 65536)
	for {
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			break
		}
		var msg dns.Msg
		if msg.Unpack(buf[:n]) != nil {
			continue
		}
		answers.add(&msg, from.IP)
		// A responder that left the service record out is asked for it.
		for _, instance := range answers.missingSRV() {
			if !asked[instance] {
				asked[instance] = true
				sendQuery(conn, dns.TypeSRV, instance)
			}
		}
	}
	return answers.instances(skip), nil
}

func sendQuery(conn *net.UDPConn, qtype uint16, name string) error {
	var msg dns.Msg
	msg.SetQuestion(name, qtype)
	msg.RecursionDesired = false
	// QU: ask for a unicast answer to this socket.
	msg.Question[0].Qclass |= 1 << 15
	packet, err := msg.Pack()
	if err != nil {
		return err
	}
	_, err = conn.WriteToUDP(packet, mdnsGroup)
	return err
}

// answerSet gathers the records of every response to one browse.
type answerSet struct {
	order  []string            // PTR targets, in arrival order
	srv    map[string]*dns.SRV // instance -> service record
	addrs  map[string][]net.IP // host name -> IPv4 addresses
	sender map[string]net.IP   // instance -> address it was announced from
}

func newAnswerSet() *answerSet {
	return &answerSet{
		srv:    map[string]*dns.SRV{},
		addrs:  map[string][]net.IP{},
		sender: map[string]net.IP{},
	}
}

func (s *answerSet) add(msg *dns.Msg, from net.IP) {
	records := append(append([]dns.RR{}, msg.Answer...), msg.Extra...)
	for _, rr := range records {
		switch r := rr.(type) {
		case *dns.PTR:
			if !strings.EqualFold(r.Hdr.Name, service) {
				continue
			}
			if _, seen := s.sender[r.Ptr]; !seen {
				s.order = append(s.order, r.Ptr)
			}
			s.sender[r.Ptr] = from
		case *dns.SRV:
			s.srv[r.Hdr.Name] = r
		case *dns.A:
			host := strings.ToLower(r.Hdr.Name)
			s.addrs[host] = append(s.addrs[host], r.A)
		}
	}
}

func (s *answerSet) missingSRV() []string {
	var missing []string
	for _, instance := range s.order {
		if s.srv[instance] == nil {
			missing = append(missing, instance)
		}
	}
	return missing
}

func (s *answerSet) instances(local map[string]bool) []CastInstance {
	found := make([]CastInstance, 0)
	seen := map[string]bool{}
	for _, instance := range s.order {
		srv := s.srv[instance]
		if srv == nil {
			continue
		}
		ip := reachable(s.addrs[strings.ToLower(srv.Target)], s.sender[instance])
		if ip == nil || local[ip.String()] {
			continue
		}
		key := fmt.Sprintf("%s:%d", ip, srv.Port)
		if seen[key] {
			continue
		}
		seen[key] = true
		found = append(found, CastInstance{
			Name: instanceName(instance),
			Host: ip.String(),
			Port: int(srv.Port),
			URL:  fmt.Sprintf("http://%s:%d", ip, srv.Port),
		})
	}
	return found
}

// reachable picks the host's address to connect to. A host advertises every
// interface's address (a container bridge too); the one its answer came from
// is known to reach us.
func reachable(addrs []net.IP, sender net.IP) net.IP {
	if sender != nil && (len(addrs) == 0 || slices.ContainsFunc(addrs, sender.Equal)) {
		return sender
	}
	if len(addrs) > 0 {
		return addrs[0]
	}
	return nil
}

// instanceName is the human part of "<name>._frogcast._tcp.local.", with
// DNS escapes undone.
func instanceName(instance string) string {
	name := strings.TrimSuffix(instance, "."+service)
	return strings.NewReplacer(`\ `, " ", `\.`, ".", `\\`, `\`).Replace(name)
}

// localIPSet lists this machine's addresses, to leave itself out. Where the
// platform refuses to list them (Android), nothing is left out.
func localIPSet() map[string]bool {
	set := map[string]bool{}
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return set
	}
	for _, a := range addrs {
		if ipNet, ok := a.(*net.IPNet); ok {
			set[ipNet.IP.String()] = true
		}
	}
	return set
}
