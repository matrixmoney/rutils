package auth

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"html"
	"log"
	"math/rand"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// EmailSender handles email dispatching via SMTP
type EmailSender struct {
	cfg *Config
}

// NewEmailSender initializes an EmailSender
func NewEmailSender(cfg *Config) *EmailSender {
	return &EmailSender{cfg: cfg}
}

// IsConfigured returns true if SMTP credentials are fully provided
func (e *EmailSender) IsConfigured() bool {
	return e.cfg != nil &&
		strings.TrimSpace(e.cfg.SMTPEmail) != "" &&
		strings.TrimSpace(e.cfg.SMTPAppPassword) != ""
}

// SendTemporaryPassword sends an elegant, responsive HTML and plain text email with a temporary password.
// Accepts: (toEmail, tempPassword) or (toEmail, userName, tempPassword)
func (e *EmailSender) SendTemporaryPassword(toEmail string, args ...string) error {
	var userName, tempPassword string
	if len(args) == 1 {
		tempPassword = args[0]
	} else if len(args) >= 2 {
		userName = strings.TrimSpace(args[0])
		tempPassword = args[1]
	} else {
		return fmt.Errorf("temporary password is required")
	}

	if !e.IsConfigured() {
		log.Printf("[AUTH-DEV] SMTP not configured in config.json. Temporary password for <%s> is: %s", toEmail, tempPassword)
		return nil
	}

	greeting := "Hello,"
	if userName != "" {
		greeting = fmt.Sprintf("Hello %s,", userName)
	}

	subject := "🔐 Your Temporary Password for Rutils - Action Required"

	textBody := fmt.Sprintf(
`%s

We received a request to recover access to your Rutils account. To help you get back in safely, we've generated a temporary password for you:

--------------------------------------------------
YOUR TEMPORARY PASSWORD:
  %s
--------------------------------------------------
(Valid for 24 hours)

🌸 A KIND REMINDER TO CHANGE YOUR PASSWORD:
For your privacy and security, this temporary password provides one-time access only.
When you log in with it, our system will kindly prompt you to set a fresh permanent password of your choice.
Please take a moment to choose a strong, unique password that only you know to keep your account safe and your data protected.

SIMPLE STEPS:
1. Copy the temporary password shown above.
2. Sign in to Rutils with your registered email and this temporary password.
3. When prompted, set your new permanent password.

Didn't request this? If you did not ask for a password reset, you can safely ignore this email. Your current password remains secure, and no changes have been made to your credentials.

Warm regards,
The Rutils Team
`, greeting, tempPassword)

	htmlBody := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Your Temporary Password</title>
  <style>
    body {
      margin: 0;
      padding: 0;
      background-color: #f1f5f9;
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif;
      color: #334155;
      -webkit-font-smoothing: antialiased;
    }
    table { border-collapse: collapse; }
    .wrapper {
      width: 100%%;
      background-color: #f1f5f9;
      padding: 40px 12px;
    }
    .card {
      max-width: 580px;
      margin: 0 auto;
      background-color: #ffffff;
      border-radius: 16px;
      overflow: hidden;
      box-shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.05), 0 8px 10px -6px rgba(0, 0, 0, 0.03);
      border: 1px solid #e2e8f0;
    }
    .banner {
      background: linear-gradient(135deg, #4f46e5 0%%, #7c3aed 50%%, #06b6d4 100%%);
      padding: 36px 30px;
      text-align: center;
      color: #ffffff;
    }
    .brand-title {
      font-size: 28px;
      font-weight: 800;
      letter-spacing: -0.5px;
      margin: 0;
      text-transform: uppercase;
    }
    .brand-sub {
      font-size: 14px;
      opacity: 0.92;
      margin-top: 6px;
      font-weight: 400;
      letter-spacing: 0.5px;
    }
    .body-content {
      padding: 36px 32px 28px;
    }
    .greeting {
      font-size: 18px;
      font-weight: 700;
      color: #0f172a;
      margin-bottom: 14px;
    }
    p {
      font-size: 15px;
      line-height: 1.6;
      color: #475569;
      margin: 0 0 16px 0;
    }
    .password-container {
      background: #f8fafc;
      border: 2px dashed #6366f1;
      border-radius: 12px;
      padding: 22px 20px;
      text-align: center;
      margin: 24px 0;
    }
    .password-label {
      font-size: 12px;
      font-weight: 700;
      text-transform: uppercase;
      letter-spacing: 1.5px;
      color: #6366f1;
      margin-bottom: 8px;
      display: block;
    }
    .password-value {
      font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, Courier, monospace;
      font-size: 26px;
      font-weight: 800;
      letter-spacing: 3px;
      color: #1e1b4b;
      padding: 8px 18px;
      background: #ffffff;
      border: 1px solid #cbd5e1;
      border-radius: 8px;
      display: inline-block;
      user-select: all;
    }
    .expiry-pill {
      display: inline-block;
      margin-top: 10px;
      font-size: 12px;
      font-weight: 600;
      color: #b45309;
      background: #fef3c7;
      padding: 3px 12px;
      border-radius: 9999px;
    }
    .kind-card {
      background-color: #f0fdf4;
      border-left: 4px solid #10b981;
      border-radius: 8px;
      padding: 18px 20px;
      margin: 24px 0;
    }
    .kind-card-title {
      font-size: 15px;
      font-weight: 700;
      color: #065f46;
      margin: 0 0 8px 0;
    }
    .kind-card-text {
      font-size: 14px;
      line-height: 1.6;
      color: #047857;
      margin: 0;
    }
    .steps-box {
      margin: 22px 0 24px;
    }
    .step-row {
      margin-bottom: 12px;
      font-size: 14px;
      line-height: 1.5;
      color: #334155;
    }
    .step-number {
      display: inline-block;
      width: 22px;
      height: 22px;
      background: #e0e7ff;
      color: #4338ca;
      font-weight: 700;
      font-size: 12px;
      line-height: 22px;
      text-align: center;
      border-radius: 50%%;
      margin-right: 10px;
    }
    .notice-box {
      background-color: #f8fafc;
      border: 1px solid #e2e8f0;
      border-radius: 8px;
      padding: 14px 16px;
      font-size: 13px;
      color: #64748b;
      line-height: 1.5;
      margin-top: 24px;
    }
    .signoff {
      margin-top: 28px;
      padding-top: 20px;
      border-top: 1px solid #f1f5f9;
      font-size: 14px;
      color: #475569;
    }
    .footer {
      background-color: #f8fafc;
      padding: 24px 30px;
      text-align: center;
      font-size: 12px;
      color: #94a3b8;
      border-top: 1px solid #e2e8f0;
      line-height: 1.6;
    }
  </style>
</head>
<body>
  <table role="presentation" class="wrapper">
    <tr>
      <td align="center">
        <div class="card">
          <div class="banner">
            <h1 class="brand-title">Rutils</h1>
            <div class="brand-sub">Account Recovery & Security</div>
          </div>
          <div class="body-content" align="left">
            <div class="greeting">%s</div>
            <p>We received a request to recover access to your Rutils account. Don't worry—we've generated a secure temporary password so you can sign in easily.</p>
            
            <div class="password-container">
              <span class="password-label">Your Temporary Password</span>
              <div class="password-value">%s</div>
              <br>
              <span class="expiry-pill">⏳ Valid for 24 hours</span>
            </div>

            <div class="kind-card">
              <div class="kind-card-title">🌸 A Kind Reminder to Change Your Password</div>
              <div class="kind-card-text">
                For your ongoing security and peace of mind, this temporary password provides <strong>one-time access</strong> only. When you log in with it, you will be warmly guided to set a <strong>fresh permanent password</strong> of your choice.<br><br>
                Please take a moment to choose a strong password that only you know to keep your account safe and protected.
              </div>
            </div>

            <p style="font-weight: 700; color: #1e293b; margin-bottom: 10px;">Follow these simple steps:</p>
            <div class="steps-box">
              <div class="step-row">
                <span class="step-number">1</span>
                <strong>Copy</strong> your temporary password shown above.
              </div>
              <div class="step-row">
                <span class="step-number">2</span>
                <strong>Sign in</strong> to Rutils using your registered email and the temporary password.
              </div>
              <div class="step-row">
                <span class="step-number">3</span>
                <strong>Set your permanent password</strong> when prompted on your screen.
              </div>
            </div>

            <div class="notice-box">
              <strong>Didn't request this?</strong> If you did not ask for a password reset, you can safely disregard this email. Your current password remains secure, and no changes have been applied to your account.
            </div>

            <div class="signoff">
              Warm regards,<br>
              <strong style="color: #1e293b;">The Rutils Team</strong>
            </div>
          </div>
          <div class="footer">
            This is an automated security notification from Rutils.<br>
            Please do not reply directly to this email.<br>
            &copy; 2026 Rutils. All rights reserved.
          </div>
        </div>
      </td>
    </tr>
  </table>
</body>
</html>`, html.EscapeString(greeting), html.EscapeString(tempPassword))

	return e.sendMail(toEmail, subject, textBody, htmlBody)
}

// SendPasswordChangedNotification sends a confirmation email when a user changes their password
func (e *EmailSender) SendPasswordChangedNotification(toEmail string, args ...string) error {
	var userName string
	if len(args) >= 1 {
		userName = strings.TrimSpace(args[0])
	}

	if !e.IsConfigured() {
		log.Printf("[AUTH-DEV] SMTP not configured in config.json. Password change confirmation email for <%s> simulated.", toEmail)
		return nil
	}

	greeting := "Hello,"
	if userName != "" {
		greeting = fmt.Sprintf("Hello %s,", userName)
	}

	subject := "🔒 Security Alert: Password Changed for Your Rutils Account"

	textBody := fmt.Sprintf(
`%s

This email confirms that the password for your Rutils account was just successfully changed.

Your new password is now active. You can use it the next time you sign in.

If you made this change, you can safely disregard this email.

If you did NOT change your password, please contact support or reset your password immediately to protect your account.

Warm regards,
The Rutils Team
`, greeting)

	htmlBody := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Password Changed Confirmation</title>
  <style>
    body {
      margin: 0;
      padding: 0;
      background-color: #f1f5f9;
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif;
      color: #334155;
    }
    table { border-collapse: collapse; }
    .wrapper {
      width: 100%%;
      background-color: #f1f5f9;
      padding: 40px 12px;
    }
    .card {
      max-width: 580px;
      margin: 0 auto;
      background-color: #ffffff;
      border-radius: 16px;
      overflow: hidden;
      box-shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.05);
      border: 1px solid #e2e8f0;
    }
    .banner {
      background: linear-gradient(135deg, #10b981 0%%, #059669 100%%);
      padding: 32px 30px;
      text-align: center;
      color: #ffffff;
    }
    .brand-title {
      font-size: 26px;
      font-weight: 800;
      margin: 0;
    }
    .body-content {
      padding: 36px 32px 28px;
    }
    .greeting {
      font-size: 18px;
      font-weight: 700;
      color: #0f172a;
      margin-bottom: 14px;
    }
    p {
      font-size: 15px;
      line-height: 1.6;
      color: #475569;
      margin: 0 0 16px 0;
    }
    .success-card {
      background-color: #f0fdf4;
      border-left: 4px solid #10b981;
      border-radius: 8px;
      padding: 16px 18px;
      margin: 20px 0;
      font-size: 14px;
      color: #065f46;
    }
    .alert-card {
      background-color: #fff1f2;
      border: 1px solid #fecdd3;
      border-radius: 8px;
      padding: 14px 16px;
      font-size: 13px;
      color: #9f1239;
      margin-top: 24px;
    }
    .signoff {
      margin-top: 28px;
      padding-top: 20px;
      border-top: 1px solid #f1f5f9;
      font-size: 14px;
      color: #475569;
    }
    .footer {
      background-color: #f8fafc;
      padding: 24px 30px;
      text-align: center;
      font-size: 12px;
      color: #94a3b8;
      border-top: 1px solid #e2e8f0;
    }
  </style>
</head>
<body>
  <table role="presentation" class="wrapper">
    <tr>
      <td align="center">
        <div class="card">
          <div class="banner">
            <h1 class="brand-title">Rutils Security</h1>
          </div>
          <div class="body-content" align="left">
            <div class="greeting">%s</div>
            <p>This email is a friendly confirmation that the password for your Rutils account was just successfully updated.</p>

            <div class="success-card">
              <strong>✅ Password Updated Successfully</strong><br>
              Your new password is now active and ready to use next time you sign in.
            </div>

            <p>If you made this change, you're all set! No further action is required on your part.</p>

            <div class="alert-card">
              <strong>Didn't make this change?</strong><br>
              If you did not change your password, please contact our support team immediately or perform a password reset to secure your account.
            </div>

            <div class="signoff">
              Warm regards,<br>
              <strong style="color: #1e293b;">The Rutils Team</strong>
            </div>
          </div>
          <div class="footer">
            This is an automated security notification from Rutils.<br>
            &copy; 2026 Rutils. All rights reserved.
          </div>
        </div>
      </td>
    </tr>
  </table>
</body>
</html>`, html.EscapeString(greeting))

	return e.sendMail(toEmail, subject, textBody, htmlBody)
}

// ⬅️ NEW: SendTaskNotification sends a confirmation email to the authenticated
// user whenever they create a task via POST /tasks.
//
// Arguments:
//   - toEmail:  the logged-in user's email (from the JWT claims)
//   - userName: the logged-in user's name (from the JWT claims; may be empty)
//   - taskText: the exact text the user typed into the Saya/textarea
//   - dueDate:  the due date in YYYY-MM-DD format
//
// The email is dispatched synchronously (see internal/api/tasks_handler.go
// for the timeout wrapper). When SMTP is not configured, this method logs
// a single development-fallback line to stdout, matching the pattern used
// by SendTemporaryPassword and SendPasswordChangedNotification.
func (e *EmailSender) SendTaskNotification(toEmail, userName, taskText, dueDate string) error {
	if !e.IsConfigured() {
		log.Printf("[AUTH-DEV] SMTP not configured in config.json. Task notification for <%s> simulated: text=%q due=%q",
			toEmail, taskText, dueDate)
		return nil
	}

	greeting := "Hello,"
	if strings.TrimSpace(userName) != "" {
		greeting = fmt.Sprintf("Hello %s,", userName)
	}

	// Truncate very long task texts for the subject line so the email
	// header stays well under RFC limits. The body carries the full text.
	subjectSnippet := taskText
	if len(subjectSnippet) > 60 {
		subjectSnippet = subjectSnippet[:57] + "..."
	}
	subjectSnippet = strings.ReplaceAll(subjectSnippet, "\n", " ")
	subjectSnippet = strings.ReplaceAll(subjectSnippet, "\r", " ")
	subject := fmt.Sprintf("📌 Task Created: %s", subjectSnippet)

	textBody := fmt.Sprintf(
`%s

Your task has been successfully created and saved in Rutils.

--------------------------------------------------
TASK:
  %s

DUE DATE:
  %s
--------------------------------------------------

You can view, edit, or delete this task anytime from the Tasks section of the app.

Warm regards,
The Rutils Team
`, greeting, taskText, dueDate)

	htmlBody := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Task Created</title>
  <style>
    body {
      margin: 0;
      padding: 0;
      background-color: #f1f5f9;
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif;
      color: #334155;
      -webkit-font-smoothing: antialiased;
    }
    table { border-collapse: collapse; }
    .wrapper {
      width: 100%%;
      background-color: #f1f5f9;
      padding: 40px 12px;
    }
    .card {
      max-width: 580px;
      margin: 0 auto;
      background-color: #ffffff;
      border-radius: 16px;
      overflow: hidden;
      box-shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.05), 0 8px 10px -6px rgba(0, 0, 0, 0.03);
      border: 1px solid #e2e8f0;
    }
    .banner {
      background: linear-gradient(135deg, #4f46e5 0%%, #7c3aed 50%%, #06b6d4 100%%);
      padding: 32px 30px;
      text-align: center;
      color: #ffffff;
    }
    .brand-title {
      font-size: 26px;
      font-weight: 800;
      letter-spacing: -0.5px;
      margin: 0;
      text-transform: uppercase;
    }
    .brand-sub {
      font-size: 13px;
      opacity: 0.92;
      margin-top: 6px;
      font-weight: 400;
      letter-spacing: 0.5px;
    }
    .body-content {
      padding: 32px 32px 24px;
    }
    .greeting {
      font-size: 18px;
      font-weight: 700;
      color: #0f172a;
      margin-bottom: 12px;
    }
    p {
      font-size: 15px;
      line-height: 1.6;
      color: #475569;
      margin: 0 0 16px 0;
    }
    .task-card {
      background: #f8fafc;
      border: 1px solid #e2e8f0;
      border-radius: 12px;
      padding: 20px 22px;
      margin: 20px 0;
    }
    .task-label {
      font-size: 11px;
      font-weight: 700;
      text-transform: uppercase;
      letter-spacing: 1.4px;
      color: #6366f1;
      margin-bottom: 8px;
      display: block;
    }
    .task-text {
      font-size: 16px;
      font-weight: 600;
      color: #0f172a;
      line-height: 1.5;
      white-space: pre-wrap;
      word-break: break-word;
      margin: 0 0 16px 0;
    }
    .task-divider {
      border: 0;
      border-top: 1px solid #e2e8f0;
      margin: 16px 0;
    }
    .due-label {
      font-size: 11px;
      font-weight: 700;
      text-transform: uppercase;
      letter-spacing: 1.4px;
      color: #0ea5e9;
      margin-bottom: 4px;
      display: block;
    }
    .due-value {
      font-size: 16px;
      font-weight: 700;
      color: #0f172a;
      font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, Courier, monospace;
    }
    .signoff {
      margin-top: 26px;
      padding-top: 18px;
      border-top: 1px solid #f1f5f9;
      font-size: 14px;
      color: #475569;
    }
    .footer {
      background-color: #f8fafc;
      padding: 22px 30px;
      text-align: center;
      font-size: 12px;
      color: #94a3b8;
      border-top: 1px solid #e2e8f0;
      line-height: 1.6;
    }
  </style>
</head>
<body>
  <table role="presentation" class="wrapper">
    <tr>
      <td align="center">
        <div class="card">
          <div class="banner">
            <h1 class="brand-title">Rutils Tasks</h1>
            <div class="brand-sub">Your Task Was Created</div>
          </div>
          <div class="body-content" align="left">
            <div class="greeting">%s</div>
            <p>Your task has been successfully created and saved in Rutils.</p>

            <div class="task-card">
              <span class="task-label">📝 Task</span>
              <div class="task-text">%s</div>
              <hr class="task-divider">
              <span class="due-label">🗓 Due Date</span>
              <div class="due-value">%s</div>
            </div>

            <p>You can view, edit, or delete this task anytime from the <strong>Tasks</strong> section of the app.</p>

            <div class="signoff">
              Warm regards,<br>
              <strong style="color: #1e293b;">The Rutils Team</strong>
            </div>
          </div>
          <div class="footer">
            This is an automated notification from Rutils.<br>
            Please do not reply directly to this email.<br>
            &copy; 2026 Rutils. All rights reserved.
          </div>
        </div>
      </td>
    </tr>
  </table>
</body>
</html>`,
		html.EscapeString(greeting),
		html.EscapeString(taskText),
		html.EscapeString(dueDate),
	)

	return e.sendMail(toEmail, subject, textBody, htmlBody)
}

// buildMultipartMessage creates a MIME multipart/alternative message (plain text + HTML)
func buildMultipartMessage(fromEmail, fromName, toEmail, subject, textBody, htmlBody string) []byte {
	boundary := fmt.Sprintf("bnd_%d_%d", time.Now().UnixNano(), rand.Int63())
	var b bytes.Buffer

	fromHeader := fromEmail
	if strings.TrimSpace(fromName) != "" {
		fromHeader = fmt.Sprintf("%s <%s>", fromName, fromEmail)
	}

	b.WriteString(fmt.Sprintf("From: %s\r\n", fromHeader))
	b.WriteString(fmt.Sprintf("To: %s\r\n", toEmail))
	b.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	b.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))
	b.WriteString(fmt.Sprintf("Message-ID: <%d.%d@rutils.local>\r\n", time.Now().UnixNano(), rand.Int63()))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n", boundary))
	b.WriteString("\r\n")

	// 1. Plain text version
	b.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(textBody)
	b.WriteString("\r\n\r\n")

	// 2. HTML version
	b.WriteString(fmt.Sprintf("--%s\r\n", boundary))
	b.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n\r\n")
	b.WriteString(htmlBody)
	b.WriteString("\r\n\r\n")

	// Closing boundary
	b.WriteString(fmt.Sprintf("--%s--\r\n", boundary))

	return b.Bytes()
}

// sendMail formats and delivers an email via SMTP with support for ports 587 (STARTTLS) and 465 (Direct TLS)
func (e *EmailSender) sendMail(toEmail, subject, textBody, htmlBody string) error {
	host := e.cfg.SMTPHost
	if host == "" {
		host = "smtp.gmail.com"
	}
	port := e.cfg.SMTPPort
	if port == "" {
		port = "587"
	}

	from := e.cfg.SMTPEmail
	senderName := e.cfg.SMTPSenderName
	if senderName == "" {
		senderName = "Rutils Team"
	}

	addr := fmt.Sprintf("%s:%s", host, port)
	message := buildMultipartMessage(from, senderName, toEmail, subject, textBody, htmlBody)

	auth := smtp.PlainAuth("", from, e.cfg.SMTPAppPassword, host)

	log.Printf("[AUTH] Dispatching email to <%s> via %s (%s)...", toEmail, addr, subject)

	if port == "465" {
		// Direct TLS / SMTPS connection
		tlsConfig := &tls.Config{
			InsecureSkipVerify: false,
			ServerName:         host,
		}
		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			log.Printf("[ERROR] Failed to connect to SMTP via TLS %s: %v", addr, err)
			return fmt.Errorf("failed to dial SMTP via TLS: %w", err)
		}
		defer conn.Close()

		client, err := smtp.NewClient(conn, host)
		if err != nil {
			log.Printf("[ERROR] Failed to initialize SMTP client on %s: %v", addr, err)
			return fmt.Errorf("failed to initialize SMTP client: %w", err)
		}
		defer client.Quit()

		if err := client.Auth(auth); err != nil {
			log.Printf("[ERROR] SMTP authentication failed for %s: %v", from, err)
			return fmt.Errorf("SMTP authentication failed: %w", err)
		}

		if err := client.Mail(from); err != nil {
			return fmt.Errorf("SMTP MAIL command failed: %w", err)
		}
		if err := client.Rcpt(toEmail); err != nil {
			return fmt.Errorf("SMTP RCPT command failed: %w", err)
		}
		w, err := client.Data()
		if err != nil {
			return fmt.Errorf("SMTP DATA command failed: %w", err)
		}
		if _, err := w.Write(message); err != nil {
			return fmt.Errorf("failed writing email message: %w", err)
		}
		if err := w.Close(); err != nil {
			return fmt.Errorf("failed finalizing email message: %w", err)
		}
	} else {
		// Port 587 (STARTTLS) or standard SMTP
		deadlineConn, err := net.DialTimeout("tcp", addr, 15*time.Second)
		if err != nil {
			log.Printf("[ERROR] Could not connect to SMTP server %s: %v", addr, err)
			return fmt.Errorf("failed to connect to SMTP server: %w", err)
		}
		_ = deadlineConn.Close()

		err = smtp.SendMail(addr, auth, from, []string{toEmail}, message)
		if err != nil {
			log.Printf("[ERROR] Failed to send email to %s via %s: %v", toEmail, addr, err)
			return fmt.Errorf("failed to send email: %w", err)
		}
	}

	log.Printf("[AUTH] Successfully delivered email to <%s> via %s!", toEmail, addr)
	return nil
}