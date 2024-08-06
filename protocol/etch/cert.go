package etch

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"math/rand/v2"
	"net"
	"time"

	"github.com/mohanson/daze"
	"github.com/mohanson/daze/lib/doa"
)

func NewCert() tls.Certificate {
	priv := doa.Try(ecdsa.GenerateKey(elliptic.P256(), &daze.RandomReader{}))
	temp := x509.Certificate{
		SerialNumber: big.NewInt(rand.Int64()),
		Subject: pkix.Name{
			Organization: []string{"Acme Co"},
		},
		NotBefore:   doa.Try(time.Parse(time.DateOnly, "1970-01-01")),
		NotAfter:    doa.Try(time.Parse(time.DateOnly, "9999-12-31")),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	cert := doa.Try(x509.CreateCertificate(&daze.RandomReader{}, &temp, &temp, &priv.PublicKey, priv))
	certFile := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert})
	pkcs := doa.Try(x509.MarshalPKCS8PrivateKey(priv))
	pkcsFile := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs})
	return doa.Try(tls.X509KeyPair(certFile, pkcsFile))
}
