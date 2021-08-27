package resolver

import (
	"context"
	"net"
	"sync"
)

// InMemoryDNS implements PacketConn and allows to override the
// golang net.DefaultResolver DNS functions
type InMemoryDNS struct {
	// Connection parameters
	readCh chan []byte

	once sync.Once
	done chan struct{}

	readDeadline  connDeadline
	writeDeadline connDeadline

	// DNS overrides
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

var _ net.PacketConn = &InMemoryDNS{}

// NewInMemoryResolver receives a InMemoryDNS object with the override functions
func NewInMemoryResolver(dialDns *InMemoryDNS) *net.Resolver {
	if dialDns == nil {
		dialDns = &InMemoryDNS{}
	}

	// Initialize connection
	dialDns.readCh = make(chan []byte)
	dialDns.done = make(chan struct{})
	dialDns.readDeadline = makeConnDeadline()
	dialDns.writeDeadline = makeConnDeadline()

	return &net.Resolver{
		PreferGo: true,
		Dial:     dialDns.Dial,
	}
}

func (r *InMemoryDNS) lookupAddr(ctx context.Context, addr string) (names []string, err error) {
	if r != nil && r.LookupAddr != nil {
		return r.LookupAddr(ctx, addr)
	}
	return net.DefaultResolver.LookupAddr(ctx, addr)
}
func (r *InMemoryDNS) lookupCNAME(ctx context.Context, host string) (cname string, err error) {
	if r != nil && r.LookupCNAME != nil {
		return r.LookupCNAME(ctx, host)
	}
	return net.DefaultResolver.LookupCNAME(ctx, host)

}
func (r *InMemoryDNS) lookupHost(ctx context.Context, host string) (addrs []string, err error) {
	if r != nil && r.LookupHost != nil {
		return r.LookupHost(ctx, host)
	}
	return net.DefaultResolver.LookupHost(ctx, host)

}
func (r *InMemoryDNS) lookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	if r != nil && r.LookupIPAddr != nil {
		return r.LookupIPAddr(ctx, host)
	}
	return net.DefaultResolver.LookupIPAddr(ctx, host)

}
func (r *InMemoryDNS) lookupMX(ctx context.Context, name string) ([]*net.MX, error) {
	if r != nil && r.LookupMX != nil {
		return r.LookupMX(ctx, name)
	}
	return net.DefaultResolver.LookupMX(ctx, name)

}
func (r *InMemoryDNS) lookupNS(ctx context.Context, name string) ([]*net.NS, error) {
	if r != nil && r.LookupNS != nil {
		return r.LookupNS(ctx, name)
	}
	return net.DefaultResolver.LookupNS(ctx, name)

}
func (r *InMemoryDNS) lookupPort(ctx context.Context, network, service string) (port int, err error) {
	if r != nil && r.LookupPort != nil {
		return r.LookupPort(ctx, network, service)
	}
	return net.DefaultResolver.LookupPort(ctx, network, service)

}
func (r *InMemoryDNS) lookupSRV(ctx context.Context, service, proto, name string) (cname string, addrs []*net.SRV, err error) {
	if r != nil && r.LookupSRV != nil {
		return r.LookupSRV(ctx, service, proto, name)
	}
	return net.DefaultResolver.LookupSRV(ctx, service, proto, name)
}
func (r *InMemoryDNS) lookupTXT(ctx context.Context, name string) ([]string, error) {
	if r != nil && r.LookupTXT != nil {
		return r.LookupTXT(ctx, name)
	}
	return net.DefaultResolver.LookupTXT(ctx, name)

}
