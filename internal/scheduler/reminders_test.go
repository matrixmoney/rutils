package scheduler

import (
	"testing"
	"time"

	"file-converter/internal/tasks"
)

// ─────────────────────────────────────────────────────────────────────
// Test fixtures
// ─────────────────────────────────────────────────────────────────────

// ist is the default reminder timezone used throughout the tests. It
// matches the tasks package's default, so a task built with zero-value
// reminder fields and then passed through normalizeReminderDefaults
// would land on the same zone this test suite reasons about.
var ist = mustLoadLocation("Asia/Kolkata")

// utc is used to test the "task timezone differs from tick timezone"
// case — the scheduler receives `now` in UTC and must convert it into
// the task's zone before applying the day-based idempotency check.
var utc = time.UTC

// mustLoadLocation fails the test immediately if the zone cannot be
// loaded. Every modern Go installation ships with a full tzdata, so a
// failure here means the CI environment is broken, not the code.
func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic("test setup: cannot load timezone " + name + ": " + err.Error())
	}
	return loc
}

// at builds a time in the given location from the given components.
// All seconds and nanoseconds are zero so that comparisons in the
// tests are exact.
func at(loc *time.Location, year int, month time.Month, day, hour, minute int) time.Time {
	return time.Date(year, month, day, hour, minute, 0, 0, loc)
}

// baseTask returns a Task with every field set to the "healthy default"
// that the store would produce after normalization:
//
//   - ReminderEnabled   = true
//   - ReminderStartDate = the due date
//   - ReminderTime      = "08:00"
//   - ReminderTimezone  = "Asia/Kolkata"
//   - LastReminderSentAt = nil
//
// Every test mutates only the field it cares about, so a failure points
// at exactly the condition under test.
func baseTask() tasks.Task {
	return tasks.Task{
		ID:                 "tsk_test",
		UserID:             "usr_test",
		Text:               "Test task",
		DueDate:            "2026-12-03",
		ReminderEnabled:    true,
		ReminderStartDate:  "2026-12-03",
		ReminderTime:       "08:00",
		ReminderTimezone:   "Asia/Kolkata",
		LastReminderSentAt: nil,
	}
}

// ─────────────────────────────────────────────────────────────────────
// shouldSendReminder — the table-driven specification
//
// Every row is one sentence of the requirement, expressed as a
// before/after pair. If a future contributor changes the reminder
// logic, exactly one of these rows will go red.
// ─────────────────────────────────────────────────────────────────────

func TestShouldSendReminder(t *testing.T) {
	// Fixed reference points so tests never depend on wall-clock time.
	const (
		year  = 2026
		month = time.November
	)

	cases := []struct {
		name     string
		mutate   func(t *tasks.Task) // nil = use baseTask as-is
		now      time.Time
		expected bool
	}{
		// ── 1. Master on/off switch ────────────────────────────────
		{
			name:     "disabled task is never sent",
			mutate:   func(t *tasks.Task) { t.ReminderEnabled = false },
			now:      at(ist, year, month, 13, 8, 0),
			expected: false,
		},
		{
			name:     "enabled task is sent when all other conditions hold",
			mutate:   nil,
			now:      at(ist, 2026, time.December, 3, 8, 0), // = start date @ 08:00 IST
			expected: true,
		},

		// ── 2. Before the start instant → do not fire ─────────────
		{
			name: "one minute before start",
			mutate: func(t *tasks.Task) {
				t.ReminderStartDate = "2026-11-13"
				t.ReminderTime = "20:00"
			},
			now:      at(ist, year, month, 13, 19, 59),
			expected: false,
		},
		{
			name: "exactly at start time",
			mutate: func(t *tasks.Task) {
				t.ReminderStartDate = "2026-11-13"
				t.ReminderTime = "20:00"
			},
			now:      at(ist, year, month, 13, 20, 0),
			expected: true,
		},
		{
			name: "one minute after start time",
			mutate: func(t *tasks.Task) {
				t.ReminderStartDate = "2026-11-13"
				t.ReminderTime = "20:00"
			},
			now:      at(ist, year, month, 13, 20, 1),
			expected: true,
		},
		{
			name: "a day before start date",
			mutate: func(t *tasks.Task) {
				t.ReminderStartDate = "2026-11-13"
				t.ReminderTime = "20:00"
			},
			now:      at(ist, year, month, 12, 23, 59),
			expected: false,
		},
		{
			name: "a day after start date at the reminder time",
			mutate: func(t *tasks.Task) {
				t.ReminderStartDate = "2026-11-13"
				t.ReminderTime = "20:00"
			},
			now:      at(ist, year, month, 14, 20, 0),
			expected: true,
		},

		// ── 3. Idempotency guard (LastReminderSentAt) ─────────────
		{
			name: "already sent today at the reminder time → skip",
			mutate: func(t *tasks.Task) {
				t.ReminderStartDate = "2026-11-13"
				t.ReminderTime = "20:00"
				sent := at(ist, year, month, 13, 20, 0)
				t.LastReminderSentAt = &sent
			},
			now:      at(ist, year, month, 13, 20, 30),
			expected: false,
		},
		{
			name: "already sent earlier today → still skip later the same day",
			mutate: func(t *tasks.Task) {
				t.ReminderStartDate = "2026-11-13"
				t.ReminderTime = "20:00"
				sent := at(ist, year, month, 13, 20, 5)
				t.LastReminderSentAt = &sent
			},
			now:      at(ist, year, month, 13, 22, 0),
			expected: false,
		},
		{
			name: "sent yesterday → fire again today",
			mutate: func(t *tasks.Task) {
				t.ReminderStartDate = "2026-11-13"
				t.ReminderTime = "20:00"
				sent := at(ist, year, month, 12, 20, 0)
				t.LastReminderSentAt = &sent
			},
			now:      at(ist, year, month, 13, 20, 0),
			expected: true,
		},

		// ── 4. No due-date stop (the core requirement) ────────────
		{
			name: "sends on the due date",
			mutate: func(t *tasks.Task) {
				t.DueDate = "2026-11-13" // = reminder start
				t.ReminderStartDate = "2026-11-13"
				t.ReminderTime = "20:00"
			},
			now:      at(ist, year, month, 13, 20, 0),
			expected: true,
		},
		{
			name: "keeps sending AFTER the due date — the due date is not a stop condition",
			mutate: func(t *tasks.Task) {
				t.DueDate = "2026-11-01" // already past
				t.ReminderStartDate = "2026-10-01"
				t.ReminderTime = "20:00"
			},
			now:      at(ist, year, month, 13, 20, 0),
			expected: true,
		},
		{
			name: "keeps sending far past the due date",
			mutate: func(t *tasks.Task) {
				t.DueDate = "2025-01-01"
				t.ReminderStartDate = "2025-01-01"
				t.ReminderTime = "20:00"
			},
			now:      at(ist, year, month, 13, 20, 0),
			expected: true,
		},

		// ── 5. Timezone handling ──────────────────────────────────
		{
			name: "now is in UTC but task is IST — 20:00 IST = 14:30 UTC",
			mutate: func(t *tasks.Task) {
				t.ReminderStartDate = "2026-11-13"
				t.ReminderTime = "20:00"
			},
			now:      at(utc, year, month, 13, 14, 30),
			expected: true,
		},
		{
			name: "UTC tick 1 minute before 20:00 IST",
			mutate: func(t *tasks.Task) {
				t.ReminderStartDate = "2026-11-13"
				t.ReminderTime = "20:00"
			},
			now:      at(utc, year, month, 13, 14, 29),
			expected: false,
		},

		// ── 6. Cross-day idempotency across timezones ────────────
		{
			name: "last sent at 23:00 IST yesterday, now is 00:30 IST today",
			mutate: func(t *tasks.Task) {
				t.ReminderStartDate = "2026-11-01"
				t.ReminderTime = "00:30"
				sent := at(ist, year, month, 12, 23, 0) // yesterday, 23:00 IST
				t.LastReminderSentAt = &sent
			},
			now:      at(ist, year, month, 13, 0, 30),
			expected: true,
		},

		// ── 7. Corrupt stored fields must be skipped, not fired ──
		{
			name:     "empty reminder_start_date → skip",
			mutate:   func(t *tasks.Task) { t.ReminderStartDate = "" },
			now:      at(ist, year, month, 13, 8, 0),
			expected: false,
		},
		{
			name:     "empty reminder_time → skip",
			mutate:   func(t *tasks.Task) { t.ReminderTime = "" },
			now:      at(ist, year, month, 13, 8, 0),
			expected: false,
		},
		{
			name: "invalid reminder_start_date → skip",
			mutate: func(t *tasks.Task) {
				t.ReminderStartDate = "not-a-date"
			},
			now:      at(ist, year, month, 13, 8, 0),
			expected: false,
		},
		{
			name: "invalid reminder_time → skip",
			mutate: func(t *tasks.Task) {
				t.ReminderTime = "25:99"
			},
			now:      at(ist, year, month, 13, 8, 0),
			expected: false,
		},
		{
			name: "invalid reminder_timezone → falls back to UTC",
			mutate: func(t *tasks.Task) {
				t.ReminderTimezone = "Not/AZone"
				t.ReminderStartDate = "2026-11-13"
				t.ReminderTime = "20:00"
			},
			// 20:00 in the fallback UTC zone.
			now:      at(utc, year, month, 13, 20, 0),
			expected: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			task := baseTask()
			if tc.mutate != nil {
				tc.mutate(&task)
			}

			got := shouldSendReminder(&task, tc.now)
			if got != tc.expected {
				t.Fatalf(
					"shouldSendReminder() = %v, want %v\n"+
						"  task: enabled=%v start=%q time=%q tz=%q due=%q last_sent=%v\n"+
						"  now:  %v",
					got, tc.expected,
					task.ReminderEnabled, task.ReminderStartDate, task.ReminderTime,
					task.ReminderTimezone, task.DueDate, task.LastReminderSentAt,
					tc.now,
				)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────
// Nil-safety
// ─────────────────────────────────────────────────────────────────────

func TestShouldSendReminder_NilTask(t *testing.T) {
	got := shouldSendReminder(nil, time.Now())
	if got {
		t.Fatal("expected shouldSendReminder(nil, …) to return false")
	}
}

// ─────────────────────────────────────────────────────────────────────
// parseReminderInstant
// ─────────────────────────────────────────────────────────────────────

func TestParseReminderInstant_Valid(t *testing.T) {
	got, err := parseReminderInstant("2026-12-03", "20:30", ist)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2026, 12, 3, 20, 30, 0, 0, ist)
	if !got.Equal(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestParseReminderInstant_InvalidDate(t *testing.T) {
	if _, err := parseReminderInstant("not-a-date", "20:30", ist); err == nil {
		t.Fatal("expected error for invalid date")
	}
}

func TestParseReminderInstant_InvalidTime(t *testing.T) {
	if _, err := parseReminderInstant("2026-12-03", "25:00", ist); err == nil {
		t.Fatal("expected error for invalid time")
	}
}

func TestParseReminderInstant_RespectsLocation(t *testing.T) {
	// Same date and time string, but interpreted in two different
	// zones. The two instants are NOT the same wall-clock moment:
	//
	//   istTime = 2026-11-13 20:00 IST = 2026-11-13 14:30 UTC
	//   utcTime = 2026-11-13 20:00 UTC
	//
	// utcTime is 5h30m LATER than istTime, so utcTime.Sub(istTime)
	// is +5h30m. (IST is ahead of UTC, so a given wall-clock reading
	// in IST represents an earlier instant than the same reading in
	// UTC.)
	istTime, err := parseReminderInstant("2026-11-13", "20:00", ist)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	utcTime, err := parseReminderInstant("2026-11-13", "20:00", utc)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if diff := utcTime.Sub(istTime); diff != 5*time.Hour+30*time.Minute {
		t.Fatalf("expected utcTime−istTime = +5h30m, got %v", diff)
	}
}

// ─────────────────────────────────────────────────────────────────────
// sameCalendarDay
// ─────────────────────────────────────────────────────────────────────

func TestSameCalendarDay(t *testing.T) {
	cases := []struct {
		name     string
		a        time.Time
		b        time.Time
		expected bool
	}{
		{
			name:     "same instant",
			a:        at(ist, 2026, time.November, 13, 20, 0),
			b:        at(ist, 2026, time.November, 13, 20, 0),
			expected: true,
		},
		{
			name:     "same day, different times",
			a:        at(ist, 2026, time.November, 13, 0, 0),
			b:        at(ist, 2026, time.November, 13, 23, 59),
			expected: true,
		},
		{
			name:     "consecutive days",
			a:        at(ist, 2026, time.November, 13, 23, 59),
			b:        at(ist, 2026, time.November, 14, 0, 0),
			expected: false,
		},
		{
			name:     "different months",
			a:        at(ist, 2026, time.November, 30, 12, 0),
			b:        at(ist, 2026, time.December, 1, 12, 0),
			expected: false,
		},
		{
			name:     "different years",
			a:        at(ist, 2025, time.December, 31, 23, 59),
			b:        at(ist, 2026, time.January, 1, 0, 0),
			expected: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameCalendarDay(tc.a, tc.b); got != tc.expected {
				t.Fatalf("sameCalendarDay(%v, %v) = %v, want %v",
					tc.a, tc.b, got, tc.expected)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────
// formatTaskIDList
// ─────────────────────────────────────────────────────────────────────

func TestFormatTaskIDList(t *testing.T) {
	items := []tasks.Task{
		{ID: "tsk_a"},
		{ID: "tsk_b"},
		{ID: "tsk_c"},
	}
	if got, want := formatTaskIDList(items), "tsk_a,tsk_b,tsk_c"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	if got := formatTaskIDList(nil); got != "" {
		t.Fatalf("expected empty string for nil input, got %q", got)
	}
}
