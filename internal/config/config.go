package config

import (
	"os"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr         string
	DatabaseURL      string
	AdminEmail       string
	AdminPassword    string
	AdminName        string
	AlipayMode       string
	AlipayAppID      string
	AlipayPrivateKey string
	AlipayPublicKey  string
	AlipayGatewayURL string
	AlipayAppAuth    string
	WebhookSecret    string
	PublicBaseURL    string
	AllowedOrigins   []string
	CookieSecure     bool
	AutoCommission   string
	SessionTTL       time.Duration
}

func Load() Config {
	base := env("PUBLIC_BASE_URL", "http://127.0.0.1:43123")
	origins := splitCSV(env("ALLOWED_ORIGINS", base+",http://localhost:43123"))
	return Config{
		HTTPAddr:         env("HTTP_ADDR", ":8080"),
		DatabaseURL:      env("DATABASE_URL", "postgres://yin:yin@127.0.0.1:54329/nowpay?sslmode=disable"),
		AdminEmail:       strings.ToLower(strings.TrimSpace(env("ADMIN_EMAIL", "admin@yin.local"))),
		AdminPassword:    env("ADMIN_PASSWORD", "changeme-yin"),
		AdminName:        env("ADMIN_NAME", "YIN"),
		AlipayMode:       strings.ToLower(env("ALIPAY_MODE", "mock")),
		AlipayAppID:      env("ALIPAY_APP_ID", ""),
		AlipayPrivateKey: keyMaterial("ALIPAY_PRIVATE_KEY", "ALIPAY_PRIVATE_KEY_FILE"),
		AlipayPublicKey:  keyMaterial("ALIPAY_PUBLIC_KEY", "ALIPAY_PUBLIC_KEY_FILE"),
		AlipayGatewayURL: env("ALIPAY_GATEWAY_URL", "https://openapi.alipay.com/gateway.do"),
		AlipayAppAuth:    env("ALIPAY_APP_AUTH_TOKEN", ""),
		WebhookSecret:    env("WEBHOOK_SECRET", "dev-webhook-secret"),
		PublicBaseURL:    strings.TrimRight(base, "/"),
		AllowedOrigins:   origins,
		CookieSecure:     strings.HasPrefix(base, "https://"),
		AutoCommission:   env("AUTO_COMMISSION", "mock_book_then_settle"),
		SessionTTL:       7 * 24 * time.Hour,
	}
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func keyMaterial(inlineKey, fileKey string) string {
	if path := strings.TrimSpace(os.Getenv(fileKey)); path != "" {
		b, err := os.ReadFile(path)
		if err == nil {
			return string(b)
		}
	}
	return strings.ReplaceAll(os.Getenv(inlineKey), `\n`, "\n")
}
