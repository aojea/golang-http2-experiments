package resolver

import (
	"fmt"

	"golang.org/x/net/dns/dnsmessage"
)

func (r *DNSShim) ProcessDNSRequest(b []byte) []byte {

	// process DNS query
	var p dnsmessage.Parser
	hdr, err := p.Start(b)
	if err != nil {
		// return dns error
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
			// return dns error
			return []byte{}

		}

		switch q.Type {
		case dnsmessage.TypeA, dnsmessage.TypeAAAA:
			//_, err = r.lookupAddr(context.Background(), q.Name.String())
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
