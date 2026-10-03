package mail

import (
	"strings"
	"testing"
)

// TestNormalizeFallsBackToUserAndStartTLS 锁定店家最常用到的两个默认值：
// 发件人不填就复用 SMTP 账号；SECURE 不填就按 starttls 走，而不是明文。
func TestNormalizeFallsBackToUserAndStartTLS(t *testing.T) {
	cfg := Config{Host: " smtp.example.com ", Port: 587, User: " shop@example.com ", Secure: " SSL "}
	cfg.Normalize()
	if cfg.Host != "smtp.example.com" {
		t.Fatalf("host = %q", cfg.Host)
	}
	if cfg.From != "shop@example.com" {
		t.Fatalf("from fallback = %q", cfg.From)
	}
	if cfg.Secure != "ssl" {
		t.Fatalf("secure = %q", cfg.Secure)
	}

	blank := Config{Host: "smtp.example.com", Port: 587, User: "shop@example.com"}
	blank.Normalize()
	if blank.Secure != "starttls" {
		t.Fatalf("default secure = %q", blank.Secure)
	}
	// 显式填写的发件人不能被账号覆盖。
	explicit := Config{User: "shop@example.com", From: " hello@example.com "}
	explicit.Normalize()
	if explicit.From != "hello@example.com" {
		t.Fatalf("explicit from = %q", explicit.From)
	}
}

func TestValidateRejectsBadConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{"missing host", Config{Port: 587, User: "a@example.com"}, "SMTP_HOST"},
		{"bad port", Config{Host: "h", Port: 0, User: "a@example.com"}, "SMTP_PORT"},
		{"no sender", Config{Host: "h", Port: 587}, "MAIL_FROM"},
		{"invalid sender", Config{Host: "h", Port: 587, From: "not-an-address"}, "invalid"},
		{"unknown secure", Config{Host: "h", Port: 587, User: "a@example.com", Secure: "tls"}, "SMTP_SECURE"},
	}
	for _, tc := range cases {
		err := tc.cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want containing %q", tc.name, err, tc.want)
		}
	}
}

// TestValidateAcceptsDefaults 确认只填账号也能通过：From 回退与 Secure 默认
// 都在 Validate 内部补齐，而不是要求店家把同一份配置写两遍。
func TestValidateAcceptsDefaults(t *testing.T) {
	cfg := Config{Host: "smtp.example.com", Port: 587, User: "shop@example.com", Pass: "secret"}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate = %v", err)
	}
	if cfg.From != "shop@example.com" || cfg.Secure != "starttls" {
		t.Fatalf("normalized cfg = %+v", cfg)
	}
}

// TestSendAppliesNormalizeBeforeDial 是这次 SMTP 修复的回归点：Send 的值接收者
// 过去会丢掉 Validate 写回的默认值，导致 From 为空时报配置错误。现在 Send 内部
// 用指针校验，空 From 会先回退到用户再进入连接阶段。
func TestSendAppliesNormalizeBeforeDial(t *testing.T) {
	cfg := Config{Host: "127.0.0.1", Port: 1, User: "shop@example.com"}
	err := cfg.Send(Message{To: "buyer@example.com", Subject: "hi", Body: "body"})
	if err == nil {
		t.Fatal("expected a connection error, got nil")
	}
	if strings.Contains(err.Error(), "config:") {
		t.Fatalf("normalize did not reach deliver: %v", err)
	}
}

func TestSendRejectsInvalidRecipient(t *testing.T) {
	cfg := Config{Host: "127.0.0.1", Port: 1, User: "shop@example.com"}
	if err := cfg.Send(Message{To: "bad address"}); err == nil || !strings.Contains(err.Error(), "recipient") {
		t.Fatalf("err = %v", err)
	}
}
