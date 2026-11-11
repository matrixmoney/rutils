// Package scheduler — reminders.go implements the per-minute reminder
// tick that is the heart of the per-task reminder feature.
//
// ─────────────────────────────────────────────────────────────────────
// The requirement, stated precisely
// ─────────────────────────────────────────────────────────────────────
//
// Every task carries its own reminder schedule. When a user creates a
// task, they pick:
//
//   - reminder_start_date  — the first day reminders should fire
//   - reminder_time        — the time-of-day (HH:MM) to fire each day
//
// From reminder_start_date at reminder_time onward, an email fires
// EVERY DAY at reminder_time, and keeps firing UNTIL THE TASK IS
// DELETED. There is no automatic stop at the due date, no "N days
// before" window, and no "stop after the due date" behaviour.
//
// Example: due_date=2026-12-03, reminder_start_date=2026-11-13,
// reminder_time=20:00. The email fires at 20:00 on 2026-11-13,
// 2026-11-14, 2026-11-15, … and continues at 20:00 every day forever,
// until the task is deleted. The due date is irrelevant to the
// reminder schedule.
//
// ─────────────────────────────────────────────────────────────────────
// The tick algorithm
// ─────────────────────────────────────────────────────────────────────
//
// A single scheduler job fires EVERY MINUTE. On each tick:
//
//	for each user:
//	  for each of their tasks:
//	    if shouldSendReminder(task, now, tz):
//	      collect task into this user's "due this minute" bucket
//	  if bucket not empty:
//	    send ONE aggregated email listing every task in the bucket
//	    for each task in bucket:
//	      service.MarkReminderSent(userID, taskID, now)
//
// The idempotency guard is task.LastReminderSentAt. If it falls on the
// same calendar day as `now` (in the task's timezone), the task is
// skipped — this is what prevents a per-minute ticker from firing 1,440
// emails per day. See shouldSendReminder for the exact predicate.
//
// Why per-minute and not a dynamic cron-per-task:
//   - The dataset is small (~35 tasks today, trivially 10k+). The tick
//     is one `for` loop in memory — effectively free.
//   - Precision: any HH:MM the user picks is caught exactly.
//   - Simplicity: one job, one function, one test surface.
//   - Idempotency: LastReminderSentAt makes double-sends impossible.
package scheduler

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"file-converter/internal/auth"
	"file-converter/internal/tasks"
)

// reminderLayout is the on-wire / on-disk format for a reminder's date
// and time strings. Exported constants in tasks package would also
// work, but these are local to the scheduler and used only here.
const (
	reminderDateLayout = "2006-01-02"
	reminderTimeLayout = "15:04"
)

// reminderTick is the Task the scheduler invokes once per minute. It
// iterates every registered user, filters their tasks down to those
// whose reminder fires at this instant, and dispatches exactly one
// aggregated email per user who has at least one such task.
//
// Errors are handled per-user: a failure for Alice's email does not
// stop Bob's reminder. The only way this function returns a non-nil
// error is if the *user list itself* cannot be loaded — in which case
// the scheduler's ErrorHandler logs it and the next minute's tick
// still fires normally.
//
// Per-task failures (invalid stored date, missing timezone) are
// logged and skipped — they never abort the tick.
func (d Deps) reminderTick(ctx context.Context) error {
	if d.Users == nil {
		return fmt.Errorf("reminderTick: user store is nil")
	}
	if d.Tasks == nil {
		return fmt.Errorf("reminderTick: tasks service is nil")
	}
	if d.EmailSender == nil {
		return fmt.Errorf("reminderTick: email sender is nil")
	}

	users, err := d.Users.GetAll()
	if err != nil {
		return fmt.Errorf("reminderTick: failed to load users: %w", err)
	}

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
				"[scheduler] reminder-tick: %s — failed to load tasks: %v",
				u.Email, err,
			)
			failed++
			continue
		}

		// Determine which of this user's tasks are due for a reminder
		// at this exact minute. doneTasks is kept in the same order
		// as userTasks so the aggregated email lists tasks in a
		// stable, predictable order.
		var due []tasks.Task
		for i := range userTasks {
			t := &userTasks[i]
			if shouldSendReminder(t, now) {
				due = append(due, *t)
			}
		}

		if len(due) == 0 {
			skipped++
			continue
		}

		// Build the aggregated email payload.
		lines := make([]auth.TaskLine, 0, len(due))
		for _, t := range due {
			lines = append(lines, auth.TaskLine{
				Text:    t.Text,
				DueDate: t.DueDate,
			})
		}

		if err := d.EmailSender.SendDailyReminder(u.Email, u.Name, lines); err != nil {
			log.Printf(
				"[scheduler] reminder-tick: %s — email failed: %v",
				u.Email, err,
			)
			failed++
			continue
		}

		// Email succeeded. Record LastReminderSentAt on every task
		// that was included, so the next minute's tick skips them.
		// We use the tick's `now` (not time.Now()) so that every
		// task in this batch gets the same sentAt — makes reasoning
		// about "same day" deterministic.
		marked := 0
		for _, t := range due {
			if err := d.Tasks.MarkReminderSent(u.ID, t.ID, now); err != nil {
				// A task that vanished between List and Mark is a
				// benign race — the user deleted it while we were
				// composing the email. Log and move on.
				log.Printf(
					"[scheduler] reminder-tick: %s — failed to mark task %s as sent: %v",
					u.Email, t.ID, err,
				)
				continue
			}
			marked++
		}

		log.Printf(
			"[scheduler] reminder-tick: %s — sent %d reminder(s) [%s]",
			u.Email, len(due), formatTaskIDList(due),
		)
		sent++
		_ = marked
	}

	// Quiet success: only log when something actually happened. A
	// per-minute tick that logs a summary line every 60 seconds would
	// drown out everything else in the log file. We log a one-liner
	// only if at least one email was sent.
	if sent > 0 || failed > 0 {
		log.Printf(
			"[scheduler] reminder-tick: done (users_sent=%d users_skipped=%d users_failed=%d)",
			sent, skipped, failed,
		)
	}
	return nil
}

// shouldSendReminder is the ENTIRE specification of the reminder
// feature, expressed as one pure predicate. It is deliberately
// dependency-free (no store, no clock, no I/O) so that every test case
// is a single line in a table.
//
// Returns true iff ALL of the following hold:
//
//  1. task.ReminderEnabled is true.
//  2. `now` is on or after the task's reminder start instant —
//     i.e. on the start date, at or after the reminder time-of-day,
//     in the task's reminder timezone.
//  3. The task has NOT already had a reminder sent today, in the
//     task's reminder timezone (i.e. LastReminderSentAt is nil, or
//     falls on a calendar day strictly earlier than `now` in that
//     timezone).
//
// There is NO due-date check: an overdue task keeps firing daily,
// by design, until it is deleted.
//
// Edge cases handled here:
//   - Invalid reminder_timezone (e.g. a bad IANA name): treated as
//     UTC so the tick still runs. A warning is logged at load time
//     by the service layer's validator, but old rows written before
//     that validator existed could theoretically carry a bad name.
//   - Invalid reminder_start_date: treated as "not yet started"
//     so the task is skipped rather than fired with garbage.
//   - Invalid reminder_time: same as above — skipped.
//   - LastReminderSentAt is checked by calendar-day, not by 24h
//     arithmetic. This means a "20:00" reminder correctly fires
//     again at 20:00 the next day even if the previous send was at
//     20:00:01 the day before (a 23h59m gap).
func shouldSendReminder(t *tasks.Task, now time.Time) bool {
	if t == nil {
		return false
	}
	if !t.ReminderEnabled {
		return false
	}

	// ── Resolve the task's timezone ─────────────────────────────
	//
	// An invalid IANA name should not crash the tick. Fall back to
	// UTC and let the caller (the tick loop) log the anomaly if it
	// cares to. This is a defensive fallback: the service validates
	// timezones on Create/Update, so a bad name can only be present
	// in rows written by an earlier version of the code or by hand.
	loc, err := time.LoadLocation(strings.TrimSpace(t.ReminderTimezone))
	if err != nil || loc == nil {
		loc = time.UTC
	}

	// ── Resolve the reminder start instant ──────────────────────
	//
	// The stored start_date and time are local to loc. We build the
	// instant in that location, then compare against `now`.
	//
	// Empty fields should never reach here (the store normalises
	// them at load time), but if they do we treat them as "skip":
	// a task with no start date cannot meaningfully fire.
	startDate := strings.TrimSpace(t.ReminderStartDate)
	startTime := strings.TrimSpace(t.ReminderTime)
	if startDate == "" || startTime == "" {
		return false
	}

	startInstant, err := parseReminderInstant(startDate, startTime, loc)
	if err != nil {
		return false
	}

	// Rule #2: now must be on or after the start instant.
	if now.Before(startInstant) {
		return false
	}

	// Rule #3: no reminder already sent today (in task tz).
	nowLocal := now.In(loc)
	if t.LastReminderSentAt != nil {
		lastSent := t.LastReminderSentAt.In(loc)
		if sameCalendarDay(lastSent, nowLocal) {
			return false
		}
	}

	return true
}

// parseReminderInstant combines a YYYY-MM-DD date and an HH:MM time
// into a concrete time.Time in the given location.
//
// Returns an error if either input is malformed. The caller
// (shouldSendReminder) treats any error as "skip this task" — a
// corrupted reminder row must never crash the tick or fire at a
// nonsensical instant.
func parseReminderInstant(dateStr, timeStr string, loc *time.Location) (time.Time, error) {
	d, err := time.ParseInLocation(reminderDateLayout, dateStr, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid reminder_start_date %q: %w", dateStr, err)
	}
	t, err := time.Parse(reminderTimeLayout, timeStr)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid reminder_time %q: %w", timeStr, err)
	}

	// Combine: keep the date's Y/M/D and the time's H/M in the given
	// location. Second and nanosecond are always 0.
	return time.Date(
		d.Year(), d.Month(), d.Day(),
		t.Hour(), t.Minute(), 0, 0,
		loc,
	), nil
}

// sameCalendarDay reports whether two instants fall on the same
// Y/M/D in their shared location (the caller passes them already
// converted via .In(loc)).
func sameCalendarDay(a, b time.Time) bool {
	return a.Year() == b.Year() &&
		a.Month() == b.Month() &&
		a.Day() == b.Day()
}

// formatTaskIDList renders a compact comma-separated list of the
// task IDs in `tasks` — used only in the tick's log line so an
// operator can trace which tasks fired without grepping for IDs.
func formatTaskIDList(items []tasks.Task) string {
	ids := make([]string, 0, len(items))
	for _, t := range items {
		ids = append(ids, t.ID)
	}
	return strings.Join(ids, ",")
}

// ─────────────────────────────────────────────────────────────────────
// Backwards-compatible helpers
//
// The old reminders.go exposed `tasksDueToday` and `formatTaskLine`.
// Both were unexported, so no external package can be calling them —
// but reminders_test.go (rewritten in file #7) will exercise the new
// shouldSendReminder instead. If any other caller in the codebase ever
// referenced these, this block keeps them compilable.
// ─────────────────────────────────────────────────────────────────────

// formatTaskLine renders one task for the email body. Kept for
// backwards compatibility with the pre-revision helper of the same
// name. Not called by the new tick.
//
// Deprecated: use auth.TaskLine and SendDailyReminder directly.
func formatTaskLine(t auth.TaskLine) string {
	if t.DueDate == "" {
		return "- " + t.Text
	}
	return fmt.Sprintf("- %s — due %s", t.Text, t.DueDate)
}
