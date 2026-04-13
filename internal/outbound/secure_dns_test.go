package outbound

import (
	"net"
	"testing"

	mDNS "github.com/miekg/dns"
)

func TestShouldAcceptSecureDNSResponseRejectsLoopbackOnlyA(t *testing.T) {
	msg := &mDNS.Msg{
		MsgHdr: mDNS.MsgHdr{Rcode: mDNS.RcodeSuccess},
		Answer: []mDNS.RR{
			&mDNS.A{
				Hdr: mDNS.RR_Header{
					Name:   "example.com.",
					Rrtype: mDNS.TypeA,
					Class:  mDNS.ClassINET,
					Ttl:    60,
				},
				A: net.ParseIP("127.0.0.1").To4(),
			},
		},
	}

	if shouldAcceptSecureDNSResponse(msg) {
		t.Fatalf("expected loopback-only answer to be rejected")
	}
}

func TestShouldAcceptSecureDNSResponseAcceptsNonLoopbackA(t *testing.T) {
	msg := &mDNS.Msg{
		MsgHdr: mDNS.MsgHdr{Rcode: mDNS.RcodeSuccess},
		Answer: []mDNS.RR{
			&mDNS.A{
				Hdr: mDNS.RR_Header{
					Name:   "example.com.",
					Rrtype: mDNS.TypeA,
					Class:  mDNS.ClassINET,
					Ttl:    60,
				},
				A: net.ParseIP("203.0.113.10").To4(),
			},
		},
	}

	if !shouldAcceptSecureDNSResponse(msg) {
		t.Fatalf("expected non-loopback answer to be accepted")
	}
}

func TestShouldAcceptSecureDNSResponseAcceptsNoIPAnswer(t *testing.T) {
	msg := &mDNS.Msg{
		MsgHdr: mDNS.MsgHdr{Rcode: mDNS.RcodeSuccess},
		Answer: []mDNS.RR{
			&mDNS.CNAME{
				Hdr: mDNS.RR_Header{
					Name:   "example.com.",
					Rrtype: mDNS.TypeCNAME,
					Class:  mDNS.ClassINET,
					Ttl:    60,
				},
				Target: "alias.example.com.",
			},
		},
	}

	if !shouldAcceptSecureDNSResponse(msg) {
		t.Fatalf("expected non-IP success answer to be accepted")
	}
}

