package scheduler

import (
	"testing"
	"time"

	"file-converter/internal/auth"
	"file-converter/internal/tasks"
)

// fixedNow is a stable reference instant so tests never depend on
// wall-clock time. 2026-10-28 is a Wednesday; the exact weekday does
// not matter to tasksDueToday, only the date portion.
var fixedNow = time.Date(2026, 10, 28, 8, 0, 0, 0, time.UTC)

func TestTasksDueToday_EmptyInput(t *testing.T) {
	got := tasksDueToday(nil, fixedNow)
	if len(got) != 0 {
		t.Fatalf("expected empty result, got %d tasks", len(got))
	}
}

func TestTasksDueToday_AllFuture(t *testing.T) {
	in := []tasks.Task{
		{Text: "tomorrow", DueDate: "2026-10-29"},   // inside 3-day window (fixedNow = 2026-10-28)
		{Text: "next week", DueDate: "2026-11-04"}, // outside 3-day window
	}
	got := tasksDueToday(in, fixedNow)
	// With reminderWindowDays=3, only "tomorrow" is inside the window.
	if len(got) != 1 {
		t.Fatalf("expected 1 due task (only 'tomorrow' in window), got %d", len(got))
	}
	if got[0].Text != "tomorrow" {
		t.Fatalf("expected 'tomorrow', got %q", got[0].Text)
	}
}
func TestTasksDueToday_AllPast(t *testing.T) {
	in := []tasks.Task{
		{Text: "yesterday", DueDate: "2026-10-27"},
		{Text: "last week", DueDate: "2026-10-21"},
		{Text: "ancient", DueDate: "2020-01-01"},
	}
	got := tasksDueToday(in, fixedNow)
	if len(got) != 3 {
		t.Fatalf("expected 3 due tasks, got %d", len(got))
	}
}

func TestTasksDueToday_OnlyToday(t *testing.T) {
	in := []tasks.Task{
		{Text: "today", DueDate: "2026-10-28"},
	}
	got := tasksDueToday(in, fixedNow)
	if len(got) != 1 {
		t.Fatalf("expected 1 due task, got %d", len(got))
	}
	if got[0].Text != "today" {
		t.Fatalf("expected 'today', got %q", got[0].Text)
	}
}

func TestTasksDueToday_Mixed(t *testing.T) {
	in := []tasks.Task{
		{Text: "old", DueDate: "2026-10-20"},       // overdue
		{Text: "today", DueDate: "2026-10-28"},     // due today
		{Text: "tomorrow", DueDate: "2026-10-29"},  // inside window
		{Text: "next month", DueDate: "2026-11-15"},// outside window
	}
	got := tasksDueToday(in, fixedNow)
	// With reminderWindowDays=3, threshold = 2026-10-31.
	// "old", "today", "tomorrow" are all <= threshold; "next month" is not.
	if len(got) != 3 {
		t.Fatalf("expected 3 due tasks, got %d", len(got))
	}
	if got[0].Text != "old" || got[1].Text != "today" || got[2].Text != "tomorrow" {
		t.Fatalf("unexpected order/contents: %+v", got)
	}
}

func TestTasksDueToday_EmptyDueDateSkipped(t *testing.T) {
	in := []tasks.Task{
		{Text: "no date", DueDate: ""},
		{Text: "today", DueDate: "2026-10-28"},
	}
	got := tasksDueToday(in, fixedNow)
	if len(got) != 1 {
		t.Fatalf("expected 1 due task (empty-date skipped), got %d", len(got))
	}
	if got[0].Text != "today" {
		t.Fatalf("expected 'today', got %q", got[0].Text)
	}
}

func TestTasksDueToday_MalformedDueDateSkipped(t *testing.T) {
	in := []tasks.Task{
		{Text: "bad format", DueDate: "28-10-2026"},
		{Text: "garbage", DueDate: "not-a-date"},
		{Text: "valid today", DueDate: "2026-10-28"},
	}
	got := tasksDueToday(in, fixedNow)
	if len(got) != 1 {
		t.Fatalf("expected 1 due task (malformed skipped), got %d", len(got))
	}
	if got[0].Text != "valid today" {
		t.Fatalf("expected 'valid today', got %q", got[0].Text)
	}
}

func TestTasksDueToday_BoundaryYesterday(t *testing.T) {
	in := []tasks.Task{
		{Text: "yesterday", DueDate: "2026-10-27"},
		{Text: "tomorrow", DueDate: "2026-10-29"},
	}
	got := tasksDueToday(in, fixedNow)
	// Both are within the 3-day window, so both are included.
	if len(got) != 2 {
		t.Fatalf("expected 2 tasks (both in window), got %d: %+v", len(got), got)
	}
	if got[0].Text != "yesterday" || got[1].Text != "tomorrow" {
		t.Fatalf("unexpected order/contents: %+v", got)
	}
}

func TestFormatTaskLine(t *testing.T) {
	withDate := formatTaskLine(auth.TaskLine{Text: "Pay bill", DueDate: "2026-10-25"})
	if withDate != "- Pay bill — due 2026-10-25" {
		t.Fatalf("unexpected format: %q", withDate)
	}
	noDate := formatTaskLine(auth.TaskLine{Text: "Buy milk"})
	if noDate != "- Buy milk" {
		t.Fatalf("unexpected format: %q", noDate)
	}
}