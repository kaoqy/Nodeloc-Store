package mail

import (
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Host   string
	Port   int
	User   string
	Pass   string
	Secure string
	From   string
}

func ConfigFromEnv() Config {
	port, _ := strconv.Atoi(strings.TrimSpace(os.Getenv("SMTP_PORT")))
	if port == 0 {
		port = 587
	}
	return Config{Host: strings.TrimSpace(os.Getenv("SMTP_HOST")), Port: port, User: os.Getenv("SMTP_USER"), Pass: os.Getenv("SMTP_PASS"), Secure: strings.ToLower(strings.TrimSpace(os.Getenv("SMTP_SECURE"))), From: strings.TrimSpace(os.Getenv("MAIL_FROM"))}
}

func (c Config) Validate() error {
	if c.Host == "" {
		return errors.New("SMTP_HOST is required")
	}
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("SMTP_PORT must be a real port, got %d", c.Port)
	}
	if c.From == "" {
		return errors.New("MAIL_FROM is required")
	}
	if _, err := mail.ParseAddress(c.From); err != nil {
		return fmt.Errorf("MAIL_FROM is invalid: %w", err)
	}
	if c.Secure == "" {
		c.Secure = "starttls"
	}
	if c.Secure != "ssl" && c.Secure != "starttls" && c.Secure != "plain" {
		return fmt.Errorf("SMTP_SECURE must be ssl, starttls or plain, got %q", c.Secure)
	}
	return nil
}

// Message is one outgoing email.
type Message struct {
	To      string
	Subject string
	Body    string
}

// Send delivers one message through the configured SMTP server. It is the one
// path every outbound mail takes — the settings-page test and the notification
// fan-out both end up here, so an SMTP problem looks the same everywhere.
func (c Config) Send(message Message) error {
	if err := c.Validate(); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	to := strings.TrimSpace(message.To)
	address, err := mail.ParseAddress(to)
	if err != nil || address.Address != to {
		return errors.New("recipient email is invalid")
	}
	if strings.ContainsAny(to, "\r\n") || strings.ContainsAny(message.Subject, "\r\n") {
		return errors.New("recipient or subject contains invalid header characters")
	}
	from, _ := mail.ParseAddress(c.From)
	subject := strings.TrimSpace(message.Subject)
	if subject == "" {
		subject = "通知"
	}
	// The subject is MIME-encoded: a Chinese subject sent raw is mangled by every
	// client that reads it as ASCII, which is most of them for a header this long.
	encoded := mime.QEncoding.Encode("UTF-8", subject)
	lines := []string{
		"From: " + from.String(),
		"To: " + to,
		"Subject: " + encoded,
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: 8bit",
		"",
		message.Body,
	}
	return c.deliver(to, []byte(strings.Join(lines, "\r\n")))
}

// deliver opens the connection and hands the finished bytes to the server.
func (c Config) deliver(to string, payload []byte) error {
	server := net.JoinHostPort(c.Host, strconv.Itoa(c.Port))
	if c.Secure == "ssl" {
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}, "tcp", server, &tls.Config{ServerName: c.Host, MinVersion: tls.VersionTLS12})
		if err != nil {
			return fmt.Errorf("connect: %w", err)
		}
		client, err := smtp.NewClient(conn, c.Host)
		if err != nil {
			return fmt.Errorf("client: %w", err)
		}
		defer client.Close()
		return send(client, c, to, payload)
	}
	client, err := smtp.Dial(server)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer client.Close()
	if err := client.Hello("localhost"); err != nil {
		return fmt.Errorf("hello: %w", err)
	}
	if c.Secure == "starttls" {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return errors.New("server does not support STARTTLS")
		}
		if err := client.StartTLS(&tls.Config{ServerName: c.Host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("starttls: %w", err)
		}
	}
	return send(client, c, to, payload)
}

func SendTest(to string) error {
	return SendTestWithConfig(ConfigFromEnv(), to)
}

func SendTestWithConfig(cfg Config, to string) error {
	return cfg.Send(Message{
		To:      to,
		Subject: "NodeLoc Store SMTP 测试邮件",
		Body:    "SMTP 配置可用，这是一封由管理后台发出的测试邮件。\r\n\r\n如果你收到它，说明订单与站内通知的邮件提醒也能发出去了。",
	})
}

// send runs the SMTP envelope and writes the finished message. The recipient is
// passed in rather than re-read from the payload, so a body line that happens to
// look like a header cannot redirect the envelope.
func send(client *smtp.Client, cfg Config, to string, payload []byte) error {
	if cfg.User != "" {
		auth := smtp.PlainAuth("", cfg.User, cfg.Pass, cfg.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("auth: %w", err)
		}
	}
	from, _ := mail.ParseAddress(cfg.From)
	if err := client.Mail(from.Address); err != nil {
		return fmt.Errorf("mail_from: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("recipient: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}
	if _, err = writer.Write(payload); err != nil {
		_ = writer.Close()
		return fmt.Errorf("write: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("send: %w", err)
	}
	return client.Quit()
}
