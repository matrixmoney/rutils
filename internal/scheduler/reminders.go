// Package scheduler — reminders.go implements the "daily-task-reminders"
// scheduled job.
//
// The job runs once per day at 08:00 local time (registered from
// scheduler.go). Its only job is to scan every user's tasks and send
// one aggregated reminder email per user who has at least one task
// whose DueDate is today or in the past.
//
// It is deliberately read-only against both the auth store and the
// tasks store — nothing here mutates data. The only side effect is
// sending email, and that goes through internal/auth.EmailSender so
// the dev-fallback path (log-to-stdout) works automatically when SMTP
// is not configured.
package scheduler

import (
	"context"
	"fmt"
	"log"
	"time"

	"file-converter/internal/auth"
	"file-converter/internal/tasks"
)

// reminderJob is the Task the scheduler invokes once per day. It
// iterates every registered user, filters their tasks down to those
// due today or overdue, and dispatches exactly one aggregated email
// per user who has at least one such task.
//
// Errors are handled per-user: a failure for Alice's email does not
// stop Bob's reminder. The only way this function returns a non-nil
// error is if the *user list itself* cannot be loaded — in which case
// the scheduler's ErrorHandler logs it and the next day's tick still
// fires normally.
func (d Deps) reminderJob(ctx context.Context) error {
	if d.Users == nil {
		return fmt.Errorf("reminderJob: user store is nil")
	}
	if d.Tasks == nil {
		return fmt.Errorf("reminderJob: tasks service is nil")
	}
	if d.EmailSender == nil {
		return fmt.Errorf("reminderJob: email sender is nil")
	}

	users, err := d.Users.GetAll()
	if err != nil {
		return fmt.Errorf("reminderJob: failed to load users: %w", err)
	}

	log.Printf("[scheduler] daily-task-reminders: scanning %d user(s)", len(users))

	now := time.Now()
	sent := 0
	skipped := 0
	failed := 0

	for _, u := range users {
		// Respect cancellation between users so a graceful shutdown
		// during a long scan does not get stuck sending the rest.
		if err := ctx.Err(); err != nil {
			return err
		}

		userTasks, err := d.Tasks.List(u.ID)
		if err != nil {
			log.Printf(
				"[scheduler] daily-task-reminders: %s — failed to load tasks: %v",
				u.Email, err,
			)
			failed++
			continue
		}

		due := tasksDueToday(userTasks, now)
		if len(due) == 0 {
			skipped++
			continue
		}

		lines := make([]auth.TaskLine, 0, len(due))
		for _, t := range due {
			lines = append(lines, auth.TaskLine{
				Text:    t.Text,
				DueDate: t.DueDate,
			})
		}

		if err := d.EmailSender.SendDailyReminder(u.Email, u.Name, lines); err != nil {
			log.Printf(
				"[scheduler] daily-task-reminders: %s — email failed: %v",
				u.Email, err,
			)
			failed++
			continue
		}

		log.Printf(
			"[scheduler] daily-task-reminders: %s — sent, %d task(s) due",
			u.Email, len(due),
		)
		sent++
	}

	log.Printf(
		"[scheduler] daily-task-reminders: done (sent=%d skipped=%d failed=%d)",
		sent, skipped, failed,
	)
	return nil
}

// tasksDueToday returns the subset of tasks that should be included
// in today's reminder email.
//
// Policy (reminder window): include every task whose DueDate is
// within the next N days, or is due today, or is already overdue.
// Tasks far in the future (more than N days out) are still silent.
//
// N is the reminderWindowDays constant below. Raising it widens the
// heads-up window; lowering it narrows it. N=0 means "remind only on
// and after the due date" (the pre-window behaviour).
//
// Tasks with an empty or unparseable DueDate are excluded — the
// scheduler should not guess.
//
// Extracted into its own function so it can be unit-tested in
// isolation without touching stores, email, or the scheduler itself.
func tasksDueToday(all []tasks.Task, now time.Time) []tasks.Task {
	// How many days before the due date the first reminder appears.
	// 3 means "you start hearing about it 3 days before it's due,
	// then daily from then on (including on and after the due date)."
	const reminderWindowDays = 3

	threshold := now.AddDate(0, 0, reminderWindowDays).Format("2006-01-02")

	out := make([]tasks.Task, 0, len(all))
	for _, t := range all {
		if t.DueDate == "" {
			continue
		}
		// Validate format so garbage rows are silently skipped
		// rather than mis-compared against the threshold.
		if _, err := time.Parse("2006-01-02", t.DueDate); err != nil {
			continue
		}
		if t.DueDate <= threshold {
			out = append(out, t)
		}
	}
	return out
}

// formatTaskLine renders one task for the email body.
// Kept here (not in the email package) because it's a presentation
// detail of *this* job, not of EmailSender's general contract.
func formatTaskLine(t auth.TaskLine) string {
	if t.DueDate == "" {
		return "- " + t.Text
	}
	return fmt.Sprintf("- %s — due %s", t.Text, t.DueDate)
}