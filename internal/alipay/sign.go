package alipay

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"sort"
	"strings"
	"time"
)

// SignContent is the Alipay RSA2 string-to-sign: keys sorted by ASCII,
// empty values omitted, the sign parameter itself omitted. sign_type is included.
func SignContent(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k, v := range params {
		if k == "sign" || v == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte('&')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(params[k])
	}
	return b.String()
}

// BuildGatewayParams is the public request map a live client would POST to
// gateway.do. Sign is applied separately so tests can lock the field set.
func BuildGatewayParams(appID, method, bizJSON, timestamp, appAuthToken string) map[string]string {
	if timestamp == "" {
		timestamp = time.Now().Format("2006-01-02 15:04:05")
	}
	p := map[string]string{
		"app_id":      appID,
		"method":      method,
		"format":      "JSON",
		"charset":     "UTF-8",
		"sign_type":   "RSA2",
		"timestamp":   timestamp,
		"version":     "1.0",
		"biz_content": bizJSON,
	}
	if appAuthToken != "" {
		p["app_auth_token"] = appAuthToken
	}
	return p
}

// SignRSA2 signs content with SHA256withRSA and returns standard Base64.
// PKCS#1 and PKCS#8 private keys are both accepted.
func SignRSA2(content string, privateKeyPEM string) (string, error) {
	key, err := parsePrivateKey(privateKeyPEM)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(content))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

// VerifyRSA2 checks a gateway response signature with the Alipay public key.
func VerifyRSA2(content, signatureB64, publicKeyPEM string) error {
	pub, err := parsePublicKey(publicKeyPEM)
	if err != nil {
		return err
	}
	sig, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(content))
	return rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig)
}

func parsePrivateKey(pemText string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(pemText)))
	if block == nil {
		return nil, errors.New("支付宝应用私钥不是 PEM")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rk, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("支付宝应用私钥不是 RSA")
	}
	return rk, nil
}

func parsePublicKey(pemText string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(strings.TrimSpace(pemText)))
	if block == nil {
		return nil, errors.New("支付宝公钥不是 PEM")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		if pkcs1, err2 := x509.ParsePKCS1PublicKey(block.Bytes); err2 == nil {
			return pkcs1, nil
		}
		return nil, err
	}
	rk, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("支付宝公钥不是 RSA")
	}
	return rk, nil
}
