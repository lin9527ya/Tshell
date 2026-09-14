package certutil

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// GenerateSelfSigned 为 https 监听器生成自签名证书（兜底：生产请配置真实证书）。
// SAN 包含 hosts 中的域名/IP 以及 127.0.0.1/localhost。证书有效期 1 年。
// 写入 dir 下 cert.pem / key.pem，幂等（已存在且为 PEM 则直接复用）。
func GenerateSelfSigned(hosts []string, dir string) (certFile, keyFile string, err error) {
	if dir == "" {
		dir = "certs"
	}
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")

	// 已存在则复用
	if fi, e := os.Stat(certFile); e == nil && !fi.IsDir() {
		if _, e2 := os.Stat(keyFile); e2 == nil {
			return certFile, keyFile, nil
		}
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", fmt.Errorf("生成RSA密钥失败: %v", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", fmt.Errorf("生成序列号失败: %v", err)
	}

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "Tshell HTTPS Listener",
			Organization: []string{"Tshell"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	// SAN：hosts + 回环
	seen := map[string]bool{}
	addHost := func(h string) {
		if h == "" || seen[h] {
			return
		}
		seen[h] = true
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}
	for _, h := range hosts {
		addHost(h)
	}
	addHost("127.0.0.1")
	addHost("localhost")

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return "", "", fmt.Errorf("创建证书失败: %v", err)
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", fmt.Errorf("创建证书目录失败: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		return "", "", fmt.Errorf("写入证书失败: %v", err)
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		return "", "", fmt.Errorf("写入私钥失败: %v", err)
	}
	return certFile, keyFile, nil
}
