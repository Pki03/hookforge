package notifier

import (
	"fmt"
	"log/slog"
	"net/smtp"
)

type EmailConfig struct {
	SMTPHost     string
	SMTPPort     string
	SMTPUser     string
	SMTPPassword string
	FromAddr     string
}

func SendEmailAlert(cfg EmailConfig, to, eventID, endpointID, status string, attempts, maxRetries int) {
	if to == "" || cfg.SMTPHost == "" {
		return
	}

	subject := fmt.Sprintf("HookForge Alert: Event %s -> %s", eventID[:min(8, len(eventID))], status)
	body := fmt.Sprintf(
		"HookForge Alert\r\n"+
			"Event: %s\r\n"+
			"Endpoint: %s\r\n"+
			"Status: %s\r\n"+
			"Attempts: %d/%d\r\n",
		eventID, endpointID, status, attempts, maxRetries,
	)

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s", cfg.FromAddr, to, subject, body)
	addr := fmt.Sprintf("%s:%s", cfg.SMTPHost, cfg.SMTPPort)

	auth := smtp.PlainAuth("", cfg.SMTPUser, cfg.SMTPPassword, cfg.SMTPHost)

	if err := smtp.SendMail(addr, auth, cfg.FromAddr, []string{to}, []byte(msg)); err != nil {
		slog.Error("email notify error", "error", err)
		return
	}
	slog.Info("email alert sent", "event_id", eventID[:min(len(eventID), 8)], "to", to)
}
