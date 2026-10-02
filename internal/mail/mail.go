package mail

import (
	"crypto/tls"
	"errors"
	"fmt"
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
	if c.Port != 465 && c.Port != 587 {
		return fmt.Errorf("SMTP_PORT must be 465 or 587, got %d", c.Port)
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
	if c.Port == 465 && c.Secure != "ssl" {
		return errors.New("SMTP_PORT 465 requires SMTP_SECURE=ssl")
	}
	if c.Port == 587 && c.Secure != "starttls" {
		return errors.New("SMTP_PORT 587 requires SMTP_SECURE=starttls")
	}
	return nil
}

func SendTest(to string) error {
	return SendTestWithConfig(ConfigFromEnv(), to)
}

func SendTestWithConfig(cfg Config, to string) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	address, err := mail.ParseAddress(to)
	if err != nil || address.Address != to {
		return errors.New("recipient email is invalid")
	}
	if strings.ContainsAny(to, "\r\n") {
		return errors.New("recipient contains invalid header characters")
	}
	from, _ := mail.ParseAddress(cfg.From)
	subject := "Kaoqy Shop SMTP 测试邮件"
	body := "Kaoqy Shop SMTP 测试成功。\r\n\r\n这是一封由管理员测试接口发送的邮件。"
	message := strings.Join([]string{"From: " + from.String(), "To: " + to, "Subject: " + subject, "MIME-Version: 1.0", "Content-Type: text/plain; charset=UTF-8", "", body}, "\r\n")
	server := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	if cfg.Port == 465 {
		conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}, "tcp", server, &tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12})
		if err != nil {
			return fmt.Errorf("connect: %w", err)
		}
		client, err := smtp.NewClient(conn, cfg.Host)
		if err != nil {
			return fmt.Errorf("client: %w", err)
		}
		defer client.Close()
		return send(client, cfg, to, []byte(message))
	}
	client, err := smtp.Dial(server)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer client.Close()
	if err := client.Hello("localhost"); err != nil {
		return fmt.Errorf("hello: %w", err)
	}
	if ok, _ := client.Extension("STARTTLS"); !ok {
		return errors.New("server does not support STARTTLS")
	}
	if err := client.StartTLS(&tls.Config{ServerName: cfg.Host, MinVersion: tls.VersionTLS12}); err != nil {
		return fmt.Errorf("starttls: %w", err)
	}
	return send(client, cfg, to, []byte(message))
}

func send(client *smtp.Client, cfg Config, to string, message []byte) error {
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
	if _, err = writer.Write(message); err != nil {
		_ = writer.Close()
		return fmt.Errorf("write: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("send: %w", err)
	}
	return client.Quit()
}
