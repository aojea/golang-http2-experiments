package resolver

import (
	"context"
	"net"
)

// DNSShim process dns packets and executes the corresponding functions if set
// otherwise it uses the stdlib DefaultResolver.
type DNSShim struct {
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

var defaultResolver = net.DefaultResolver

// NewInMemoryResolver receives a DNSShim object with the override functions
func NewInMemoryResolver(d *DNSShim) *net.Resolver {
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

func (r *DNSShim) lookupAddr(ctx context.Context, addr string) (names []string, err error) {
	if r != nil && r.LookupAddr != nil {
		return r.LookupAddr(ctx, addr)
	}
	return defaultResolver.LookupAddr(ctx, addr)
}
func (r *DNSShim) lookupCNAME(ctx context.Context, host string) (cname string, err error) {
	if r != nil && r.LookupCNAME != nil {
		return r.LookupCNAME(ctx, host)
	}
	return defaultResolver.LookupCNAME(ctx, host)

}
func (r *DNSShim) lookupHost(ctx context.Context, host string) (addrs []string, err error) {
	if r != nil && r.LookupHost != nil {
		return r.LookupHost(ctx, host)
	}
	return defaultResolver.LookupHost(ctx, host)

}
func (r *DNSShim) lookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	if r != nil && r.LookupIPAddr != nil {
		return r.LookupIPAddr(ctx, host)
	}
	return defaultResolver.LookupIPAddr(ctx, host)

}
func (r *DNSShim) lookupMX(ctx context.Context, name string) ([]*net.MX, error) {
	if r != nil && r.LookupMX != nil {
		return r.LookupMX(ctx, name)
	}
	return defaultResolver.LookupMX(ctx, name)

}
func (r *DNSShim) lookupNS(ctx context.Context, name string) ([]*net.NS, error) {
	if r != nil && r.LookupNS != nil {
		return r.LookupNS(ctx, name)
	}
	return defaultResolver.LookupNS(ctx, name)

}
func (r *DNSShim) lookupPort(ctx context.Context, network, service string) (port int, err error) {
	if r != nil && r.LookupPort != nil {
		return r.LookupPort(ctx, network, service)
	}
	return defaultResolver.LookupPort(ctx, network, service)

}
func (r *DNSShim) lookupSRV(ctx context.Context, service, proto, name string) (cname string, addrs []*net.SRV, err error) {
	if r != nil && r.LookupSRV != nil {
		return r.LookupSRV(ctx, service, proto, name)
	}
	return defaultResolver.LookupSRV(ctx, service, proto, name)
}
func (r *DNSShim) lookupTXT(ctx context.Context, name string) ([]string, error) {
	if r != nil && r.LookupTXT != nil {
		return r.LookupTXT(ctx, name)
	}
	return defaultResolver.LookupTXT(ctx, name)

}
