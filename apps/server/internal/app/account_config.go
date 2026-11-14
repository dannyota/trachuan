package app

import (
	"errors"
	"os"

	"danny.vn/trachuan/apps/server/internal/authmail"
)

type mailConfig struct {
	smtp    authmail.SMTPOptions
	capture string
}

func loadMailConfig(publicURL, listen string) (mailConfig, error) {
	c := mailConfig{smtp: authmail.SMTPOptions{Host: os.Getenv("SERVER_SMTP_HOST"), Port: os.Getenv("SERVER_SMTP_PORT"), Username: os.Getenv("SERVER_SMTP_USERNAME"), Password: os.Getenv("SERVER_SMTP_PASSWORD"), From: os.Getenv("SERVER_MAIL_FROM")}, capture: os.Getenv("SERVER_MAIL_CAPTURE_DIR")}
	if c.smtp.Port == "" {
		c.smtp.Port = "587"
	}
	if c.smtp.From == "" {
		c.smtp.From = "no-reply@trachuan.vn"
	}
	if c.capture != "" && (c.smtp.Host != "" || !loopbackDeployment(publicURL, listen)) {
		return mailConfig{}, errors.New("SERVER_MAIL_CAPTURE_DIR requires loopback deployment and no SERVER_SMTP_HOST")
	}
	if c.smtp.Host != "" || !loopbackDeployment(publicURL, listen) {
		if _, err := authmail.NewSMTP(c.smtp); err != nil {
			return mailConfig{}, errors.New("invalid SERVER_SMTP_HOST, SERVER_SMTP_PORT, SERVER_SMTP_USERNAME, SERVER_SMTP_PASSWORD or SERVER_MAIL_FROM")
		}
	}
	return c, nil
}
