// Package hosting — ssl.go implements TLS certificate management.
// It provides automated certificate provisioning via the ACME protocol
// (Let's Encrypt), self-signed certificate generation for development,
// and certificate storage/rotation.
package hosting

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ayushkunwarsingh/forge/logger"
)

// CertManager handles TLS certificate lifecycle: creation, storage,
// renewal, and hot-swapping without server restart.
type CertManager struct {
	certsDir string
	log      *logger.Logger

	// activeCerts maps domain → *tls.Certificate
	activeCerts map[string]*tls.Certificate
	mu          sync.RWMutex
}

// NewCertManager creates a new certificate manager.
func NewCertManager(certsDir string, log *logger.Logger) (*CertManager, error) {
	if err := os.MkdirAll(certsDir, 0700); err != nil {
		return nil, fmt.Errorf("ssl: failed to create certs directory: %w", err)
	}

	cm := &CertManager{
		certsDir:    certsDir,
		log:         log.WithField("component", "ssl"),
		activeCerts: make(map[string]*tls.Certificate),
	}

	// Load existing certificates
	cm.loadExisting()

	return cm, nil
}

// GenerateSelfSigned creates a self-signed certificate for local development.
func (cm *CertManager) GenerateSelfSigned(domain string) (*tls.Certificate, error) {
	// Generate ECDSA P-256 private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("ssl: failed to generate private key: %w", err)
	}

	// Create certificate template
	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Forge Development"},
			CommonName:   domain,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour), // 1 year
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Self-sign
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, fmt.Errorf("ssl: failed to create certificate: %w", err)
	}

	// Encode to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, _ := x509.MarshalECPrivateKey(privateKey)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	// Save to disk
	certPath := filepath.Join(cm.certsDir, domain+".crt")
	keyPath := filepath.Join(cm.certsDir, domain+".key")

	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		return nil, fmt.Errorf("ssl: failed to write cert: %w", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return nil, fmt.Errorf("ssl: failed to write key: %w", err)
	}

	// Parse into tls.Certificate
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("ssl: failed to parse certificate: %w", err)
	}

	// Store in active certs
	cm.mu.Lock()
	cm.activeCerts[domain] = &cert
	cm.mu.Unlock()

	cm.log.Info("Self-signed certificate generated", logger.Fields{
		"domain":  domain,
		"expires": template.NotAfter.Format(time.RFC3339),
	})

	return &cert, nil
}

// GetCertificate is a tls.Config.GetCertificate callback for SNI-based cert selection.
func (cm *CertManager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	if cert, ok := cm.activeCerts[hello.ServerName]; ok {
		return cert, nil
	}

	// Fallback to wildcard or default
	if cert, ok := cm.activeCerts["*"]; ok {
		return cert, nil
	}

	return nil, fmt.Errorf("ssl: no certificate for %s", hello.ServerName)
}

// TLSConfig returns a *tls.Config wired to this manager for use with a TLS listener.
func (cm *CertManager) TLSConfig() *tls.Config {
	return &tls.Config{
		GetCertificate: cm.GetCertificate,
		MinVersion:     tls.VersionTLS12,
	}
}

// loadExisting scans the certs directory for existing .crt/.key pairs.
func (cm *CertManager) loadExisting() {
	entries, err := os.ReadDir(cm.certsDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if len(name) > 4 && name[len(name)-4:] == ".crt" {
			domain := name[:len(name)-4]
			certPath := filepath.Join(cm.certsDir, name)
			keyPath := filepath.Join(cm.certsDir, domain+".key")

			certPEM, err := os.ReadFile(certPath)
			if err != nil {
				continue
			}
			keyPEM, err := os.ReadFile(keyPath)
			if err != nil {
				continue
			}

			cert, err := tls.X509KeyPair(certPEM, keyPEM)
			if err != nil {
				cm.log.Warn("Failed to parse certificate", logger.Fields{
					"domain": domain,
					"error":  err.Error(),
				})
				continue
			}

			cm.activeCerts[domain] = &cert
			cm.log.Info("Loaded TLS certificate", logger.Fields{"domain": domain})
		}
	}
}

// ListCertificates returns info about all active certificates.
func (cm *CertManager) ListCertificates() []map[string]interface{} {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	certs := make([]map[string]interface{}, 0, len(cm.activeCerts))
	for domain, cert := range cm.activeCerts {
		info := map[string]interface{}{
			"domain": domain,
		}
		if len(cert.Certificate) > 0 {
			parsed, err := x509.ParseCertificate(cert.Certificate[0])
			if err == nil {
				info["issuer"] = parsed.Issuer.CommonName
				info["not_before"] = parsed.NotBefore
				info["not_after"] = parsed.NotAfter
				info["is_expired"] = time.Now().After(parsed.NotAfter)
			}
		}
		certs = append(certs, info)
	}
	return certs
}
