package mail

import (
	"context"
	"errors"
	"testing"
)

func TestExtraPorts(t *testing.T) {
	d := dialer(false)
	// 8443 isn't a mail port: refused before anything is sent.
	if _, err := d.DialContext(context.Background(), "tcp", "192.0.2.1:8443"); !errors.Is(err, ErrBlocked) {
		t.Fatalf("8443 before AllowPorts: %v", err)
	}
	AllowPorts(8443, 0, 70000)
	defer delete(extraPorts, 8443)
	if !portAllowed(8443) || portAllowed(0) || portAllowed(70000) || portAllowed(8080) {
		t.Fatal("AllowPorts allowed the wrong ports")
	}
	// Still only public addresses.
	if _, err := d.DialContext(context.Background(), "tcp", "127.0.0.1:8443"); !errors.Is(err, ErrBlocked) {
		t.Fatalf("loopback on an extra port: %v", err)
	}
}
