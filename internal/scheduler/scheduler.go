// Package scheduler is the rutils-side adapter over the
// github.com/matrixmoney/rlib-scheduler library.
//
// It exposes a single Start function that cmd/server/main.go can call
// to launch the scheduler alongside the HTTP servers. Concrete jobs
// (e.g. the daily task-reminder job) are registered inside Start.
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

	// daily-task-reminders — runs once per day at 08:00 local time.
	// Sends one aggregated email per user whose tasks include at
	// least one with DueDate <= today.
		dailySched, err := rlib.Daily(8, 0, 0)
	if err != nil {
		return err
	}
	if err := s.Register(
		"daily-task-reminders",
		dailySched,
		deps.reminderJob,
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