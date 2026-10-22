package main

import (
	"log"
	"testing"

	"file-converter/internal/auth"
)

func TestSMTPCredentialsWork(t *testing.T) {
	cfg, err := auth.LoadConfig("config.json")
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if !cfg.IsSMTPConfigured() {
		t.Fatalf("SMTP not configured — check config.json")
	}
	t.Logf("using sender <%s> via %s:%s", cfg.SMTPEmail, cfg.SMTPHost, cfg.SMTPPort)

	sender := auth.NewEmailSender(cfg)
	err = sender.SendTemporaryPassword(cfg.SMTPEmail, "SMTP-Test-User", "TestOnly123!")
	if err != nil {
		t.Fatalf("SendTemporaryPassword failed: %v", err)
	}
	log.Println("test email sent — check your Gmail inbox")
}
