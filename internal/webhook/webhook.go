package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"time"
)

type Result struct {
	Status    string
	Attempts  int
	LastError string
}

func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// Deliver posts a placeholder callback. Empty targets are skipped. Redirects
// are not followed. Failure is recorded and does not unwind the payment.
func Deliver(ctx context.Context, secret, target, event string, body []byte) Result {
	if strings.TrimSpace(target) == "" {
		return Result{Status: "skipped", Attempts: 0, LastError: "项目未填写异步通知地址"}
	}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return Result{Status: "failed", Attempts: 1, LastError: err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Yin-Event", event)
	if secret == "" {
		req.Header.Set("X-Yin-Signature", "unsigned")
	} else {
		req.Header.Set("X-Yin-Signature", Sign(secret, body))
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	res, err := client.Do(req)
	if err != nil {
		return Result{Status: "failed", Attempts: 1, LastError: err.Error()}
	}
	defer res.Body.Close()
	snippet, _ := io.ReadAll(io.LimitReader(res.Body, 512))
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return Result{Status: "delivered", Attempts: 1}
	}
	msg := res.Status
	if len(snippet) > 0 {
		msg += ": " + string(snippet)
	}
	return Result{Status: "failed", Attempts: 1, LastError: msg}
}
