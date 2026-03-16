package scanner

import (
	"time"

	"github.com/krakenkey/probe/internal/config"
)

type ScanResult struct {
	Endpoint    EndpointResult    `json:"endpoint"`
	Connection  ConnectionResult  `json:"connection"`
	Certificate *CertificateResult `json:"certificate,omitempty"`
}

type EndpointResult struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	SNI  string `json:"sni"`
}

type ConnectionResult struct {
	Success     bool    `json:"success"`
	Error       *string `json:"error"`
	LatencyMs   int64   `json:"latencyMs,omitempty"`
	TLSVersion  string  `json:"tlsVersion,omitempty"`
	CipherSuite string  `json:"cipherSuite,omitempty"`
	OCSPStapled bool    `json:"ocspStapled,omitempty"`
}

type CertificateResult struct {
	Subject            string    `json:"subject"`
	SANs               []string  `json:"sans"`
	Issuer             string    `json:"issuer"`
	SerialNumber       string    `json:"serialNumber"`
	NotBefore          time.Time `json:"notBefore"`
	NotAfter           time.Time `json:"notAfter"`
	DaysUntilExpiry    int       `json:"daysUntilExpiry"`
	KeyType            string    `json:"keyType"`
	KeySize            int       `json:"keySize"`
	SignatureAlgorithm string    `json:"signatureAlgorithm"`
	Fingerprint        string    `json:"fingerprint"`
	ChainDepth         int       `json:"chainDepth"`
	ChainComplete      bool      `json:"chainComplete"`
	Trusted            bool      `json:"trusted"`
}

func EndpointFromConfig(ep config.Endpoint) EndpointResult {
	sni := ep.SNI
	if sni == "" {
		sni = ep.Host
	}
	return EndpointResult{
		Host: ep.Host,
		Port: ep.Port,
		SNI:  sni,
	}
}
