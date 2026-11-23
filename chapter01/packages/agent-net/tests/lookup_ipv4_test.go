package tests

import (
	"context"
	"encoding/binary"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"agent-net/internal/services"
)

const lookupName = "endpoint.example.test."

// fakeDNS answers A and AAAA queries over UDP on 127.0.0.1. rcode is the response code; ip is the A answer and ip6 the
// AAAA answer, and a type whose address is empty gets an empty answer section (NOERROR with no records). queries counts
// the datagrams received.
type fakeDNS struct {
	addr    string
	queries atomic.Int64
}

func startFakeDNS(t *testing.T, rcode int, ip, ip6 string) *fakeDNS {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeDNS{addr: pc.LocalAddr().String()}
	t.Cleanup(func() { pc.Close() })
	go func() {
		buf := make([]byte, 512)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			f.queries.Add(1)
			if resp := dnsReply(buf[:n], rcode, ip, ip6); resp != nil {
				pc.WriteTo(resp, from)
			}
		}
	}()
	return f
}

// dnsReply builds the response to a single-question query, or nil when the query cannot be parsed.
func dnsReply(q []byte, rcode int, ip, ip6 string) []byte {
	if len(q) < 12 {
		return nil
	}
	i := 12
	for i < len(q) && q[i] != 0 {
		i += int(q[i]) + 1
	}
	end := i + 1 + 4 // root label, qtype, qclass
	if end > len(q) {
		return nil
	}
	qtype := binary.BigEndian.Uint16(q[i+1:])
	var rdata, rrHead []byte
	if rcode == 0 {
		switch {
		case qtype == 1 && ip != "":
			rdata = net.ParseIP(ip).To4()
			rrHead = []byte{0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4}
		case qtype == 28 && ip6 != "":
			rdata = net.ParseIP(ip6).To16()
			rrHead = []byte{0xc0, 0x0c, 0, 28, 0, 1, 0, 0, 0, 60, 0, 16}
		}
	}
	resp := make([]byte, 12, end+28)
	copy(resp[:2], q[:2]) // id
	binary.BigEndian.PutUint16(resp[2:], 0x8180|uint16(rcode))
	binary.BigEndian.PutUint16(resp[4:], 1) // qdcount
	if rdata != nil {
		binary.BigEndian.PutUint16(resp[6:], 1) // ancount
	}
	resp = append(resp, q[12:end]...)
	if rdata != nil {
		resp = append(resp, rrHead...)
		resp = append(resp, rdata...)
	}
	return resp
}

func TestLookupIPv4ViaFallsBackToSecondServer(t *testing.T) {
	first := startFakeDNS(t, 2, "", "")
	second := startFakeDNS(t, 0, "192.0.2.7", "")
	got, err := services.LookupIPv4Via(context.Background(), []string{first.addr, second.addr}, lookupName)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "192.0.2.7" {
		t.Fatalf("got %v, want [192.0.2.7]", got)
	}
	if first.queries.Load() == 0 {
		t.Fatal("first server was not asked")
	}
}

func TestLookupIPv4ViaStopsAtFirstSuccess(t *testing.T) {
	first := startFakeDNS(t, 0, "192.0.2.1", "")
	second := startFakeDNS(t, 0, "192.0.2.2", "")
	got, err := services.LookupIPv4Via(context.Background(), []string{first.addr, second.addr}, lookupName)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "192.0.2.1" {
		t.Fatalf("got %v, want [192.0.2.1]", got)
	}
	if n := second.queries.Load(); n != 0 {
		t.Fatalf("second server received %d queries, want 0", n)
	}
}

func TestLookupIPv4ViaAllFailSingleLineNamesEveryServer(t *testing.T) {
	first := startFakeDNS(t, 2, "", "")
	second := startFakeDNS(t, 2, "", "")
	_, err := services.LookupIPv4Via(context.Background(), []string{first.addr, second.addr}, lookupName)
	if err == nil {
		t.Fatal("want an error")
	}
	msg := err.Error()
	if strings.Contains(msg, "\n") {
		t.Fatalf("error spans several lines: %q", msg)
	}
	for _, s := range []string{first.addr, second.addr} {
		if !strings.Contains(msg, s) {
			t.Fatalf("error %q does not name %s", msg, s)
		}
	}
	if !strings.Contains(msg, "port 53") {
		t.Fatalf("error %q lacks the reachability hint", msg)
	}
	// Go's DNSError text "lookup <name> on <resolv.conf server>" would name the host's resolver, not the one asked.
	// The format's own "failed on every resolver" is the only legitimate " on ".
	if strings.Contains(strings.Replace(msg, "failed on every resolver", "", 1), " on ") {
		t.Fatalf("error %q carries Go's misleading \"on <server>\" text", msg)
	}
}

func TestLookupIPv4ViaNoRecordsIsAnError(t *testing.T) {
	srv := startFakeDNS(t, 0, "", "")
	_, err := services.LookupIPv4Via(context.Background(), []string{srv.addr}, lookupName)
	if err == nil {
		t.Fatal("want an error for NOERROR with no records")
	}
}

func TestLookupIPv4ViaOnlyAAAAGivesEmptyList(t *testing.T) {
	srv := startFakeDNS(t, 0, "", "2001:db8::1")
	got, err := services.LookupIPv4Via(context.Background(), []string{srv.addr}, lookupName)
	if err != nil {
		t.Fatalf("want nil error, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want empty", got)
	}
}

func TestLookupIPv4ViaCancelledContext(t *testing.T) {
	srv := startFakeDNS(t, 0, "192.0.2.1", "")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error, 1)
	go func() {
		_, err := services.LookupIPv4Via(ctx, []string{srv.addr}, lookupName)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("want an error")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not return promptly")
	}
}
