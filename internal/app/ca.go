package app

import (
	"crypto/tls"
	"crypto/x509"
	"log/slog"
	"os"
)

// extraCAs trusts the certificate authorities in PETREL_CA_FILE (PEM) on
// top of the system's: for mail servers with certificates from a
// company's own CA, or a local test server's self-signed one.
func extraCAs(logger *slog.Logger) *tls.Config {
	path := os.Getenv("PETREL_CA_FILE")
	if path == "" {
		return nil
	}
	pool, err := x509.SystemCertPool()
	if err != nil {
		pool = x509.NewCertPool()
	}
	pem, err := os.ReadFile(path)
	if err != nil || !pool.AppendCertsFromPEM(pem) {
		logger.Error("petrel: PETREL_CA_FILE has no certificates Petrel can read", "path", path, "err", err)
		return nil
	}
	return &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
}
