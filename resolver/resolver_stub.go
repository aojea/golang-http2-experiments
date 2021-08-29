package resolver

import (
	"context"
	"fmt"
	"net"

	"golang.org/x/net/dns/dnsmessage"
)

// ResolverStub process dns packets and executes the corresponding functions if set
type ResolverStub struct {
	LookupAddr   func(ctx context.Context, addr string) (names []string, err error)
	LookupCNAME  func(ctx context.Context, host string) (cname string, err error)
	LookupHost   func(ctx context.Context, host string) (addrs []string, err error)
	LookupIPAddr func(ctx context.Context, host string) ([]net.IPAddr, error)
	LookupMX     func(ctx context.Context, name string) ([]*net.MX, error)
	LookupNS     func(ctx context.Context, name string) ([]*net.NS, error)
	LookupPort   func(ctx context.Context, network, service string) (port int, err error)
	LookupSRV    func(ctx context.Context, service, proto, name string) (cname string, addrs []*net.SRV, err error)
	LookupTXT    func(ctx context.Context, name string) ([]string, error)
}

// NewInMemoryResolver receives a ResolverStub object with the override functions
func NewInMemoryResolver(d *ResolverStub) *net.Resolver {
	if d == nil {
		return net.DefaultResolver
	}
	localDNSDialer := &MemoryDialer{
		PacketHandler: d.ProcessDNSRequest,
	}

	return &net.Resolver{
		PreferGo: true,
		Dial:     localDNSDialer.Dial,
	}
}

// ProcessDNSRequest is used by the MemoryConn to process the packets
// Transform a DNS request to the corresponding Golang Lookup function
// DNS https://courses.cs.duke.edu//fall16/compsci356/DNS/DNS-primer.pdf
func (r *ResolverStub) ProcessDNSRequest(b []byte) []byte {
	// process DNS query
	var p dnsmessage.Parser
	hdr, err := p.Start(b)
	if err != nil {
		buf, err := msgFormatError.Pack()
		if err != nil {
			panic(err)
		}
		return buf
	}

	buf := make([]byte, 2, 514)
	answer := dnsmessage.NewBuilder(buf, hdr)
	answer.EnableCompression()

	for {
		q, err := p.Question()
		fmt.Println("DEBUG RCV DNS q", q)

		if err == dnsmessage.ErrSectionDone {
			break
		}
		if err != nil {
			buf, err := msgFormatError.Pack()
			if err != nil {
				panic(err)
			}
			return buf
		}

		switch q.Type {
		case dnsmessage.TypeA, dnsmessage.TypeAAAA:
			if r.LookupIPAddr == nil {
				buf, err := msgNotImplemented.Pack()
				if err != nil {
					panic(err)
				}
				return buf
			}
			// addrs, err := r.lookupIPAddr(context.Background(), q.Name.String())
			// if err != nil {

			//}
		case dnsmessage.TypeNS:
			// TODO
		case dnsmessage.TypeCNAME:
			// TODO
		case dnsmessage.TypeSOA:
			// TODO
		case dnsmessage.TypePTR:
			// TODO
		case dnsmessage.TypeMX:
			// TODO
		case dnsmessage.TypeTXT:
			// TODO
		case dnsmessage.TypeSRV:
		// TODO
		case dnsmessage.TypeOPT:
			// TODO
		default:
		}
		if err != nil {
			// return dns error
		}
	}
	out, err := answer.Finish()
	if err != nil {
		return []byte{}
	}
	return out
}

var (
	msgFormatError = dnsmessage.Message{
		Header: dnsmessage.Header{
			Response:      true,
			Authoritative: true,
			RCode:         dnsmessage.RCodeFormatError,
		},
	}
	msgNotImplemented = dnsmessage.Message{
		Header: dnsmessage.Header{
			Response:      true,
			Authoritative: true,
			RCode:         dnsmessage.RCodeNotImplemented,
		},
	}
)
