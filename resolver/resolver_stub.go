package resolver

import (
	"context"
	"net"

	"golang.org/x/net/dns/dnsmessage"
)

// ResolverStub process dns packets and executes the corresponding functions if set
type ResolverStub struct {
	LookupAddr  func(ctx context.Context, addr string) (names []string, err error)
	LookupCNAME func(ctx context.Context, host string) (cname string, err error)
	LookupHost  func(ctx context.Context, host string) (addrs []string, err error)
	LookupIP    func(ctx context.Context, network, host string) ([]net.IP, error)
	LookupMX    func(ctx context.Context, name string) ([]*net.MX, error)
	LookupNS    func(ctx context.Context, name string) ([]*net.NS, error)
	LookupPort  func(ctx context.Context, network, service string) (port int, err error)
	LookupSRV   func(ctx context.Context, service, proto, name string) (cname string, addrs []*net.SRV, err error)
	LookupTXT   func(ctx context.Context, name string) ([]string, error)
}

// ProcessDNSRequest is used by the MemoryConn to process the packets
// Transform a DNS request to the corresponding Golang Lookup function
func (r *ResolverStub) ProcessDNSRequest(b []byte) []byte {
	// process DNS query
	var p dnsmessage.Parser
	hdr, err := p.Start(b)
	if err != nil {
		return dnsErrorMessage(dnsmessage.RCodeFormatError)
	}

	// Only support 1 question, the code in dnsmessage says
	// https://cs.opensource.google/go/x/net/+/e898025e:dns/dnsmessage/message.go
	// Multiple questions are valid according to the spec,
	// but servers don't actually support them. There will
	// be at most one question here.
	questions, err := p.AllQuestions()
	if err != nil {
		return dnsErrorMessage(dnsmessage.RCodeFormatError)
	}
	if len(questions) > 1 {
		return dnsErrorMessage(dnsmessage.RCodeNotImplemented)
	} else if len(questions) == 0 {
		return dnsErrorMessage(dnsmessage.RCodeFormatError)
	}
	q := questions[0]

	// Create the answer restricted to 512 bytes (Section 4.2.1 RFC 1035)
	buf := make([]byte, 2, 514)
	answer := dnsmessage.NewBuilder(buf,
		dnsmessage.Header{
			ID:            hdr.ID,
			Response:      true,
			Authoritative: true,
		})
	answer.EnableCompression()

	err = answer.StartQuestions()
	if err != nil {
		return dnsErrorMessage(dnsmessage.RCodeServerFailure)
	}
	answer.Question(q)

	err = answer.StartAnswers()
	if err != nil {
		return dnsErrorMessage(dnsmessage.RCodeServerFailure)
	}
	switch q.Type {
	case dnsmessage.TypeA:
		if r.LookupIP == nil {
			return dnsErrorMessage(dnsmessage.RCodeNotImplemented)
		}
		addrs, err := r.LookupIP(context.Background(), "ip4", q.Name.String())
		if err != nil {
			return dnsErrorMessage(dnsmessage.RCodeServerFailure)
		}

		for _, ip := range addrs {
			a := ip.To4()
			if a == nil {
				continue
			}
			err = answer.AResource(
				dnsmessage.ResourceHeader{
					Name:  q.Name,
					Class: q.Class,
					TTL:   86400,
				},
				dnsmessage.AResource{
					A: [4]byte{a[0], a[1], a[2], a[3]},
				},
			)
			if err != nil {
				return dnsErrorMessage(dnsmessage.RCodeServerFailure)
			}
		}

	case dnsmessage.TypeAAAA:
		if r.LookupIP == nil {
			return dnsErrorMessage(dnsmessage.RCodeNotImplemented)
		}
		addrs, err := r.LookupIP(context.Background(), "ip6", q.Name.String())
		if err != nil {
			return dnsErrorMessage(dnsmessage.RCodeServerFailure)
		}

		for _, ip := range addrs {
			if ip.To16() == nil || ip.To4() != nil {
				continue
			}
			var aaaa [16]byte
			copy(aaaa[:], ip.To16())
			err = answer.AAAAResource(
				dnsmessage.ResourceHeader{
					Name:  q.Name,
					Class: q.Class,
					TTL:   86400,
				},
				dnsmessage.AAAAResource{
					AAAA: aaaa,
				},
			)
			if err != nil {
				return dnsErrorMessage(dnsmessage.RCodeServerFailure)
			}
		}

	case dnsmessage.TypeNS:
		if r.LookupNS == nil {
			return dnsErrorMessage(dnsmessage.RCodeNotImplemented)
		}
	case dnsmessage.TypeCNAME:
		if r.LookupCNAME == nil {
			return dnsErrorMessage(dnsmessage.RCodeNotImplemented)
		}
	case dnsmessage.TypeSOA:
		return dnsErrorMessage(dnsmessage.RCodeNotImplemented)
	case dnsmessage.TypePTR:
		if r.LookupAddr == nil {
			return dnsErrorMessage(dnsmessage.RCodeNotImplemented)
		}
	case dnsmessage.TypeMX:
		if r.LookupMX == nil {
			return dnsErrorMessage(dnsmessage.RCodeNotImplemented)
		}
	case dnsmessage.TypeTXT:
		if r.LookupTXT == nil {
			return dnsErrorMessage(dnsmessage.RCodeNotImplemented)
		}
	case dnsmessage.TypeSRV:
		if r.LookupSRV == nil {
			return dnsErrorMessage(dnsmessage.RCodeNotImplemented)
		}
	case dnsmessage.TypeOPT:
		return dnsErrorMessage(dnsmessage.RCodeNotImplemented)
	default:
		return dnsErrorMessage(dnsmessage.RCodeNotImplemented)
	}
	if err != nil {
		// return dns error
	}
	buf, err = answer.Finish()
	if err != nil {
		return dnsErrorMessage(dnsmessage.RCodeServerFailure)
	}
	// omit first 2 byes
	return buf[2:]
}

// dnsErrorMessage return an encoded dns error message
func dnsErrorMessage(rcode dnsmessage.RCode) []byte {
	msg := dnsmessage.Message{
		Header: dnsmessage.Header{
			Response:      true,
			Authoritative: true,
			RCode:         rcode,
		},
	}
	buf, err := msg.Pack()
	if err != nil {
		panic(err)
	}
	return buf
}

// NewInMemoryResolver receives a ResolverStub object with the override functions
// and returns a resolver that can be used as custom Resolver implementation
func NewInMemoryResolver(d *ResolverStub) *net.Resolver {
	if d == nil {
		return net.DefaultResolver
	}
	localDNSDialer := &HairpinDialer{
		PacketHandler: d.ProcessDNSRequest,
	}

	return &net.Resolver{
		PreferGo: true,
		Dial:     localDNSDialer.Dial,
	}
}
