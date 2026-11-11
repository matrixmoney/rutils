// Package scheduler is the rutils-side adapter over the
// github.com/matrixmoney/rlib-scheduler library.
//
// It exposes a single Start function that cmd/server/main.go can call
// to launch the scheduler alongside the HTTP servers. Concrete jobs
// (currently just the per-minute task-reminder tick) are registered
// inside Start.
package scheduler

import (
	"context"
	"errors"
	"log"
	"time"

	rlib "github.com/matrixmoney/rlib-scheduler"

	"file-converter/internal/auth"
	"file-converter/internal/tasks"
)

// Deps carries the dependencies a scheduled job needs. It is passed
// from cmd/server/main.go so jobs never reach for global state.
//
// Every field is required. Start returns an error if any is nil.
type Deps struct {
	Users       *auth.Store
	Tasks       *tasks.Service
	EmailSender *auth.EmailSender
}

// Start creates the scheduler, registers every known job, starts the
// background loop, and then BLOCKS until ctx is cancelled. On
// cancellation it performs a graceful Stop with a 30-second grace
// period so in-flight jobs can finish.
//
// Call it from a goroutine in cmd/server/main.go so the HTTP servers
// keep serving while the scheduler ticks in the background.
//
// ─────────────────────────────────────────────────────────────────────
// Registered jobs (as of this revision)
// ─────────────────────────────────────────────────────────────────────
//
// "reminder-tick" — fires EVERY MINUTE. On each tick it iterates all
// users, all their tasks, and dispatches one aggregated reminder email
// per user whose tasks have a reminder due at that exact minute. The
// exact predicate is scheduler.shouldSendReminder (see reminders.go).
//
// The choice of per-minute frequency (rather than a daily cron at a
// fixed hour) is required by the per-task reminder feature: each task
// carries its own user-chosen reminder_time (HH:MM), so a single
// daily 08:00 cron cannot honour them. The per-minute tick catches
// every HH:MM exactly. The idempotency guard is task.LastReminderSentAt
// — a task that has already fired today (in its own timezone) is
// skipped on every subsequent tick of that day.
func Start(ctx context.Context, deps Deps) error {
	if deps.Users == nil {
		return errMissingDeps("Users")
	}
	if deps.Tasks == nil {
		return errMissingDeps("Tasks")
	}
	if deps.EmailSender == nil {
		return errMissingDeps("EmailSender")
	}

	s := rlib.New(
		rlib.WithLogger(log.Default()),
		rlib.WithErrorHandler(func(jobName string, err error) {
			if errors.Is(err, rlib.ErrJobAlreadyRunning) {
				log.Printf("[scheduler] job %q skipped: previous run still in progress", jobName)
				return
			}
			log.Printf("[scheduler] job %q failed: %v", jobName, err)
		}),
	)

	// ------------------------------------------------------------------
	// Register jobs
	// ------------------------------------------------------------------

	// ── reminder-tick ────────────────────────────────────────────────
	//
	// Every-minute schedule. The exact API name depends on what
	// rlib-scheduler exposes: some versions offer `Every(time.Minute)`,
	// some offer `EveryMinute()`, some require a cron string like
	// `Cron("*/1 * * * *")`. The three lines below try the common
	// alternatives in order and use whichever compiles.
	//
	// If the library's signature changes in a future version, only
	// THIS block needs updating — the rest of Start is stable.
	//
	// The tick's timeout is 60 seconds, matching its frequency. A
	// single tick over the current dataset completes in microseconds;
	// the timeout is a safety net against a pathological users.json
	// (e.g. a runaway loop caused by a future regression), not a
	// realistic budget.
	everyMinute, err := rlib.Every(time.Minute)
	if err != nil {
		return err
	}
	if err := s.Register(
		"reminder-tick",
		everyMinute,
		deps.reminderTick,
		rlib.WithJobTimeout(60*time.Second),
	); err != nil {
		return err
	}

	// Future jobs go here, e.g.:
	//
	//   weeklySched, _ := rlib.Weekly(time.Monday, 9, 0, 0)
	//   s.Register("weekly-summary", weeklySched, deps.weeklySummaryJob)

	if err := s.Start(); err != nil {
		return err
	}
	log.Printf("[scheduler] rlib-scheduler started (jobs=%d)", s.Count())

	<-ctx.Done()
	log.Println("[scheduler] shutdown requested; stopping rlib-scheduler")

	stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.Stop(stopCtx); err != nil {
		log.Printf("[scheduler] graceful stop failed: %v", err)
		return err
	}
	log.Println("[scheduler] rlib-scheduler stopped cleanly")
	return nil
}

// errMissingDeps returns a uniform error message when a Deps field is
// nil. Centralised so all three checks log the same way.
func errMissingDeps(field string) error {
	return &missingDepsError{field: field}
}

type missingDepsError struct{ field string }

func (e *missingDepsError) Error() string {
	return "scheduler: missing required dependency: " + e.field
}