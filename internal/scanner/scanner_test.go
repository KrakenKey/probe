package scanner

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"strconv"
	"testing"
	"time"
)

// newTestTLSServer starts a TLS listener with a self-signed certificate and
// returns the listener, the leaf certificate, and the port.
func newTestTLSServer(t *testing.T, opts ...func(*x509.Certificate)) (net.Listener, *x509.Certificate, int) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "test.example.com"},
		DNSNames:     []string{"test.example.com", "www.test.example.com"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(90 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IsCA:         true, // self-signed acts as its own root
		BasicConstraintsValid: true,
	}

	for _, opt := range opts {
		opt(tmpl)
	}

	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatal(err)
	}

	tlsCert := tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}

	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	// Accept connections in the background
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				tlsConn := c.(*tls.Conn)
				_ = tlsConn.Handshake()
			}(conn)
		}
	}()

	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)

	return ln, cert, port
}

func TestScanEndpointSuccess(t *testing.T) {
	_, cert, port := newTestTLSServer(t)

	ep := EndpointResult{
		Host: "127.0.0.1",
		Port: port,
		SNI:  "test.example.com",
	}

	result := ScanEndpoint(context.Background(), ep, 5*time.Second)

	if !result.Connection.Success {
		t.Fatalf("expected success, got error: %v", result.Connection.Error)
	}
	if result.Connection.Error != nil {
		t.Errorf("expected nil error, got %q", *result.Connection.Error)
	}
	if result.Connection.LatencyMs < 0 {
		t.Errorf("expected non-negative latency, got %d", result.Connection.LatencyMs)
	}
	if result.Connection.TLSVersion == "" {
		t.Error("expected TLS version, got empty")
	}
	if result.Connection.CipherSuite == "" {
		t.Error("expected cipher suite, got empty")
	}

	if result.Certificate == nil {
		t.Fatal("expected certificate result, got nil")
	}

	c := result.Certificate
	if c.Subject == "" {
		t.Error("expected subject, got empty")
	}
	if len(c.SANs) != 2 {
		t.Errorf("expected 2 SANs, got %d: %v", len(c.SANs), c.SANs)
	}
	if c.Issuer == "" {
		t.Error("expected issuer, got empty")
	}
	if c.SerialNumber == "" {
		t.Error("expected serial number, got empty")
	}
	if c.NotBefore.IsZero() || c.NotAfter.IsZero() {
		t.Error("expected valid notBefore/notAfter")
	}
	if c.DaysUntilExpiry < 89 || c.DaysUntilExpiry > 91 {
		t.Errorf("expected ~90 days until expiry, got %d", c.DaysUntilExpiry)
	}
	if c.KeyType != "ECDSA" {
		t.Errorf("expected ECDSA key type, got %q", c.KeyType)
	}
	if c.KeySize != 256 {
		t.Errorf("expected 256-bit key, got %d", c.KeySize)
	}
	if c.SignatureAlgorithm == "" {
		t.Error("expected signature algorithm, got empty")
	}
	if c.Fingerprint == "" || c.Fingerprint[:7] != "SHA256:" {
		t.Errorf("expected SHA256: fingerprint, got %q", c.Fingerprint)
	}
	if c.ChainDepth != 1 {
		t.Errorf("expected chain depth 1, got %d", c.ChainDepth)
	}

	_ = cert
}

func TestScanEndpointExpiredCert(t *testing.T) {
	_, _, port := newTestTLSServer(t, func(tmpl *x509.Certificate) {
		tmpl.NotBefore = time.Now().Add(-365 * 24 * time.Hour)
		tmpl.NotAfter = time.Now().Add(-1 * 24 * time.Hour)
	})

	ep := EndpointResult{
		Host: "127.0.0.1",
		Port: port,
		SNI:  "test.example.com",
	}

	result := ScanEndpoint(context.Background(), ep, 5*time.Second)

	// Should still connect (InsecureSkipVerify=true)
	if !result.Connection.Success {
		t.Fatalf("expected success even for expired cert, got error: %v", result.Connection.Error)
	}

	if result.Certificate == nil {
		t.Fatal("expected certificate result, got nil")
	}

	if result.Certificate.DaysUntilExpiry >= 0 {
		t.Errorf("expected negative days until expiry for expired cert, got %d", result.Certificate.DaysUntilExpiry)
	}

	// Self-signed expired cert should not be trusted
	if result.Certificate.Trusted {
		t.Error("expected expired cert to not be trusted")
	}
}

func TestScanEndpointConnectionRefused(t *testing.T) {
	// Use a port that (almost certainly) has nothing listening
	ep := EndpointResult{
		Host: "127.0.0.1",
		Port: 19999,
		SNI:  "test.example.com",
	}

	result := ScanEndpoint(context.Background(), ep, 2*time.Second)

	if result.Connection.Success {
		t.Fatal("expected connection failure")
	}
	if result.Connection.Error == nil {
		t.Fatal("expected error message")
	}
	if result.Certificate != nil {
		t.Error("expected nil certificate on connection failure")
	}
}

func TestScanEndpointTimeout(t *testing.T) {
	// Create a TCP listener that accepts but never completes TLS handshake
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			// Hold connection open, never respond
			go func(c net.Conn) {
				buf := make([]byte, 1024)
				_, _ = c.Read(buf) // block until closed
				c.Close()
			}(conn)
		}
	}()

	_, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)

	ep := EndpointResult{
		Host: "127.0.0.1",
		Port: port,
		SNI:  "test.example.com",
	}

	start := time.Now()
	result := ScanEndpoint(context.Background(), ep, 1*time.Second)
	elapsed := time.Since(start)

	if result.Connection.Success {
		t.Fatal("expected timeout failure")
	}
	if result.Connection.Error == nil {
		t.Fatal("expected error message")
	}
	if elapsed > 3*time.Second {
		t.Errorf("timeout took too long: %v", elapsed)
	}
}

func TestScanEndpointTLSVersion(t *testing.T) {
	_, _, port := newTestTLSServer(t)

	ep := EndpointResult{
		Host: "127.0.0.1",
		Port: port,
		SNI:  "test.example.com",
	}

	result := ScanEndpoint(context.Background(), ep, 5*time.Second)

	if !result.Connection.Success {
		t.Fatalf("expected success, got error: %v", result.Connection.Error)
	}

	// Go's TLS implementation should negotiate TLS 1.3 by default
	if result.Connection.TLSVersion != "TLS 1.3" {
		t.Errorf("expected TLS 1.3, got %q", result.Connection.TLSVersion)
	}
}

func TestScanEndpointSNIDefault(t *testing.T) {
	ep := EndpointResult{
		Host: "127.0.0.1",
		Port: 443,
		SNI:  "",
	}

	// Just verify EndpointFromConfig sets SNI
	from := EndpointFromConfig(struct {
		Host string `yaml:"host" json:"host"`
		Port int    `yaml:"port" json:"port"`
		SNI  string `yaml:"sni"  json:"sni,omitempty"`
	}{Host: "example.com", Port: 443})
	if from.SNI != "example.com" {
		t.Errorf("expected SNI to default to host, got %q", from.SNI)
	}

	_ = ep
}

func TestScanAllConcurrent(t *testing.T) {
	_, _, port1 := newTestTLSServer(t)
	_, _, port2 := newTestTLSServer(t)

	endpoints := []EndpointResult{
		{Host: "127.0.0.1", Port: port1, SNI: "test.example.com"},
		{Host: "127.0.0.1", Port: port2, SNI: "test.example.com"},
		{Host: "127.0.0.1", Port: 19998, SNI: "unreachable.example.com"}, // will fail
	}

	results := ScanAll(context.Background(), endpoints, 2*time.Second)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// First two should succeed
	if !results[0].Connection.Success {
		t.Errorf("results[0] expected success, got error: %v", results[0].Connection.Error)
	}
	if !results[1].Connection.Success {
		t.Errorf("results[1] expected success, got error: %v", results[1].Connection.Error)
	}

	// Third should fail
	if results[2].Connection.Success {
		t.Error("results[2] expected failure for unreachable endpoint")
	}

	// Results should be in order
	if results[0].Endpoint.Port != port1 {
		t.Errorf("results[0] port = %d, want %d", results[0].Endpoint.Port, port1)
	}
	if results[1].Endpoint.Port != port2 {
		t.Errorf("results[1] port = %d, want %d", results[1].Endpoint.Port, port2)
	}
}

func TestTLSVersionString(t *testing.T) {
	tests := []struct {
		version uint16
		want    string
	}{
		{tls.VersionTLS10, "TLS 1.0"},
		{tls.VersionTLS11, "TLS 1.1"},
		{tls.VersionTLS12, "TLS 1.2"},
		{tls.VersionTLS13, "TLS 1.3"},
		{0x0000, "unknown (0x0000)"},
	}

	for _, tt := range tests {
		got := tlsVersionString(tt.version)
		if got != tt.want {
			t.Errorf("tlsVersionString(0x%04x) = %q, want %q", tt.version, got, tt.want)
		}
	}
}

func TestFormatFingerprint(t *testing.T) {
	got := formatFingerprint([]byte{0xab, 0xcd, 0xef, 0x01})
	want := "ab:cd:ef:01"
	if got != want {
		t.Errorf("formatFingerprint = %q, want %q", got, want)
	}
}

func TestFormatSerial(t *testing.T) {
	got := formatSerial([]byte{0x04, 0xab, 0xcd})
	want := "04:ab:cd"
	if got != want {
		t.Errorf("formatSerial = %q, want %q", got, want)
	}
}
