package scanner

import (
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"math"
	"net"
	"strings"
	"time"
)

// ScanEndpoint connects to host:port via TLS and extracts certificate and
// connection metadata. It uses crypto/tls from the standard library.
func ScanEndpoint(ctx context.Context, ep EndpointResult, timeout time.Duration) *ScanResult {
	result := &ScanResult{Endpoint: ep}

	addr := net.JoinHostPort(ep.Host, fmt.Sprintf("%d", ep.Port))

	dialer := &net.Dialer{Timeout: timeout}

	tlsCfg := &tls.Config{
		ServerName:         ep.SNI,
		InsecureSkipVerify: true, // we verify trust separately
	}

	start := time.Now()
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, tlsCfg)
	latency := time.Since(start)

	if err != nil {
		errStr := err.Error()
		result.Connection = ConnectionResult{
			Success: false,
			Error:   &errStr,
		}
		return result
	}
	defer conn.Close()

	state := conn.ConnectionState()

	result.Connection = ConnectionResult{
		Success:     true,
		LatencyMs:   latency.Milliseconds(),
		TLSVersion:  tlsVersionString(state.Version),
		CipherSuite: tls.CipherSuiteName(state.CipherSuite),
		OCSPStapled: len(state.OCSPResponse) > 0,
	}

	if len(state.PeerCertificates) > 0 {
		leaf := state.PeerCertificates[0]
		result.Certificate = extractCertInfo(leaf, state)
	}

	return result
}

// ScanAll scans all endpoints concurrently and returns results in order.
func ScanAll(ctx context.Context, endpoints []EndpointResult, timeout time.Duration) []ScanResult {
	results := make([]ScanResult, len(endpoints))
	done := make(chan int, len(endpoints))

	for i, ep := range endpoints {
		go func(idx int, endpoint EndpointResult) {
			r := ScanEndpoint(ctx, endpoint, timeout)
			results[idx] = *r
			done <- idx
		}(i, ep)
	}

	for range endpoints {
		<-done
	}

	return results
}

func extractCertInfo(leaf *x509.Certificate, state tls.ConnectionState) *CertificateResult {
	daysUntilExpiry := int(math.Floor(time.Until(leaf.NotAfter).Hours() / 24))

	fingerprint := sha256.Sum256(leaf.Raw)
	fpStr := formatFingerprint(fingerprint[:])

	keyType, keySize := keyInfo(leaf)

	chainDepth := len(state.PeerCertificates)
	chainComplete := isChainComplete(state.PeerCertificates)

	trusted := verifyTrust(leaf, state.PeerCertificates)

	return &CertificateResult{
		Subject:            leaf.Subject.String(),
		SANs:               leaf.DNSNames,
		Issuer:             leaf.Issuer.String(),
		SerialNumber:       formatSerial(leaf.SerialNumber.Bytes()),
		NotBefore:          leaf.NotBefore,
		NotAfter:           leaf.NotAfter,
		DaysUntilExpiry:    daysUntilExpiry,
		KeyType:            keyType,
		KeySize:            keySize,
		SignatureAlgorithm: leaf.SignatureAlgorithm.String(),
		Fingerprint:        "SHA256:" + fpStr,
		ChainDepth:         chainDepth,
		ChainComplete:      chainComplete,
		Trusted:            trusted,
	}
}

func verifyTrust(leaf *x509.Certificate, chain []*x509.Certificate) bool {
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}

	intermediates := x509.NewCertPool()
	for _, cert := range chain[1:] {
		intermediates.AddCert(cert)
	}

	_, err = leaf.Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: intermediates,
	})
	return err == nil
}

func isChainComplete(chain []*x509.Certificate) bool {
	if len(chain) == 0 {
		return false
	}
	last := chain[len(chain)-1]
	return last.IsCA && last.CheckSignatureFrom(last) == nil
}

func keyInfo(cert *x509.Certificate) (string, int) {
	switch pub := cert.PublicKey.(type) {
	case *rsa.PublicKey:
		return "RSA", pub.N.BitLen()
	case *ecdsa.PublicKey:
		return "ECDSA", pub.Curve.Params().BitSize
	default:
		if cert.PublicKeyAlgorithm == x509.Ed25519 {
			return "Ed25519", 256
		}
		return cert.PublicKeyAlgorithm.String(), 0
	}
}

func tlsVersionString(v uint16) string {
	switch v {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return fmt.Sprintf("unknown (0x%04x)", v)
	}
}

func formatFingerprint(b []byte) string {
	parts := make([]string, len(b))
	for i, v := range b {
		parts[i] = fmt.Sprintf("%02x", v)
	}
	return strings.Join(parts, ":")
}

func formatSerial(b []byte) string {
	parts := make([]string, len(b))
	for i, v := range b {
		parts[i] = fmt.Sprintf("%02x", v)
	}
	return strings.Join(parts, ":")
}
