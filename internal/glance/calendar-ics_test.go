package glance

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	ics "github.com/arran4/golang-ical"
)

const calendarTestICS = `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//Test//EN
BEGIN:VEVENT
UID:all-day-event@example.com
SUMMARY:Vacation
DTSTART;VALUE=DATE:20251105
DTEND;VALUE=DATE:20251107
END:VEVENT
BEGIN:VEVENT
UID:utc-event@example.com
SUMMARY:Standup\, weekly
DTSTART:20251105T090000Z
DTEND:20251105T093000Z
END:VEVENT
BEGIN:VEVENT
UID:tzid-event@example.com
SUMMARY:Team sync
DTSTART;TZID=Asia/Shanghai:20251106T093000
DTEND;TZID=Asia/Shanghai:20251106T100000
END:VEVENT
BEGIN:VEVENT
UID:duration-event@example.com
SUMMARY:Lunch
DTSTART:20251107T120000Z
DURATION:PT1H30M
END:VEVENT
END:VCALENDAR
`

var calendarTestWindowStart = time.Date(2025, 10, 1, 0, 0, 0, 0, time.UTC)
var calendarTestWindowEnd = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

func parseTestCalendarInWindow(t *testing.T, icsContent string, windowStart, windowEnd time.Time) calendarEventList {
	t.Helper()

	parsed, err := ics.ParseCalendar(strings.NewReader(icsContent))
	if err != nil {
		t.Fatalf("failed to parse ICS: %v", err)
	}

	events, err := parseCalendarEvents(parsed, "var(--color-primary)", windowStart, windowEnd)
	if err != nil {
		t.Fatalf("failed to parse calendar events: %v", err)
	}

	return events
}

func parseTestCalendar(t *testing.T, icsContent string) calendarEventList {
	t.Helper()

	return parseTestCalendarInWindow(t, icsContent, calendarTestWindowStart, calendarTestWindowEnd)
}

func findEvent(t *testing.T, events calendarEventList, uid string) calendarEvent {
	t.Helper()

	for _, event := range events {
		if event.Summary == uid {
			return event
		}
	}

	t.Fatalf("event %q not found", uid)
	return calendarEvent{}
}

func TestParseCalendarEventsBasics(t *testing.T) {
	events := parseTestCalendar(t, calendarTestICS)

	allDay := findEvent(t, events, "Vacation")
	if !allDay.AllDay {
		t.Errorf("Vacation event should be all-day")
	}
	if want := time.Date(2025, 11, 5, 0, 0, 0, 0, time.UTC); !allDay.Start.Equal(want) {
		t.Errorf("all-day start = %v, want %v", allDay.Start, want)
	}
	if want := time.Date(2025, 11, 7, 0, 0, 0, 0, time.UTC); !allDay.End.Equal(want) {
		t.Errorf("all-day end = %v, want %v (exclusive)", allDay.End, want)
	}

	utcEvent := findEvent(t, events, "Standup, weekly")
	if utcEvent.AllDay {
		t.Errorf("Standup event should not be all-day")
	}
	if want := time.Date(2025, 11, 5, 9, 0, 0, 0, time.UTC); !utcEvent.Start.Equal(want) {
		t.Errorf("UTC event start = %v, want %v", utcEvent.Start, want)
	}
	if want := time.Date(2025, 11, 5, 9, 30, 0, 0, time.UTC); !utcEvent.End.Equal(want) {
		t.Errorf("UTC event end = %v, want %v", utcEvent.End, want)
	}
	// Summary must be unescaped.
	if utcEvent.Summary != "Standup, weekly" {
		t.Errorf("summary = %q, want %q", utcEvent.Summary, "Standup, weekly")
	}

	tzidEvent := findEvent(t, events, "Team sync")
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Skip("IANA tzdata not available on this system")
	}
	if want := time.Date(2025, 11, 6, 9, 30, 0, 0, shanghai); !tzidEvent.Start.Equal(want) {
		t.Errorf("TZID event start = %v, want %v", tzidEvent.Start, want)
	}

	durationEvent := findEvent(t, events, "Lunch")
	if want := time.Date(2025, 11, 7, 13, 30, 0, 0, time.UTC); !durationEvent.End.Equal(want) {
		t.Errorf("duration event end = %v, want %v", durationEvent.End, want)
	}
}

func TestParseCalendarEventsRecurring(t *testing.T) {
	icsContent := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//Test//EN
BEGIN:VEVENT
UID:recurring@example.com
SUMMARY:Gym
DTSTART;TZID=Asia/Shanghai:20251104T080000
DTEND;TZID=Asia/Shanghai:20251104T090000
RRULE:FREQ=WEEKLY;BYDAY=TU,TH
EXDATE;TZID=Asia/Shanghai:20251113T080000
END:VEVENT
BEGIN:VEVENT
UID:all-day-recurring@example.com
SUMMARY:Payday
DTSTART;VALUE=DATE:20251101
RRULE:FREQ=MONTHLY;BYMONTHDAY=1
EXDATE;VALUE=DATE:20251201
END:VEVENT
END:VCALENDAR
`

	events := parseTestCalendarInWindow(t, icsContent,
		time.Date(2025, 11, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
	)

	gym := make([]time.Time, 0)
	for _, event := range events {
		if event.Summary == "Gym" {
			gym = append(gym, event.Start)
		}
	}

	// Nov 2025 Tuesdays and Thursdays: 4, 6, 11, 13, 18, 20, 25, 27. The 13th is
	// excluded via EXDATE.
	wantDates := []int{4, 6, 11, 18, 20, 25, 27}
	if len(gym) != len(wantDates) {
		t.Fatalf("gym occurrences = %d (%v), want %d", len(gym), gym, len(wantDates))
	}

	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Skip("IANA tzdata not available on this system")
	}

	for i, occurrence := range gym {
		want := time.Date(2025, 11, wantDates[i], 8, 0, 0, 0, shanghai)
		if !occurrence.Equal(want) {
			t.Errorf("gym occurrence %d = %v, want %v", i, occurrence, want)
		}
	}

	paydays := 0
	for _, event := range events {
		if event.Summary != "Payday" {
			continue
		}

		if event.Start.UTC() == time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC) {
			t.Errorf("excluded payday occurrence (2025-12-01) must not be present")
		}

		paydays++
	}

	// Nov 1 and Dec 1 are within the window; Dec 1 is excluded.
	if paydays != 1 {
		t.Errorf("payday occurrences = %d, want 1", paydays)
	}
}

func TestParseCalendarEventsRecurrenceOverride(t *testing.T) {
	icsContent := `BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//Test//Test//EN
BEGIN:VEVENT
UID:daily@example.com
SUMMARY:Review
DTSTART:20251110T140000Z
DTEND:20251110T150000Z
RRULE:FREQ=DAILY;COUNT=5
END:VEVENT
BEGIN:VEVENT
UID:daily@example.com
SUMMARY:Review (moved)
RECURRENCE-ID:20251112T140000Z
DTSTART:20251113T160000Z
DTEND:20251113T170000Z
END:VEVENT
END:VCALENDAR
`

	events := parseTestCalendar(t, icsContent)

	if len(events) != 5 {
		t.Fatalf("events = %d, want 5 (4 recurring instances + 1 override)", len(events))
	}

	for _, event := range events {
		if event.Start.UTC() == time.Date(2025, 11, 12, 14, 0, 0, 0, time.UTC) {
			t.Errorf("overridden occurrence (2025-11-12 14:00 UTC) must not be present")
		}
	}

	override := findEvent(t, events, "Review (moved)")
	if want := time.Date(2025, 11, 13, 16, 0, 0, 0, time.UTC); !override.Start.Equal(want) {
		t.Errorf("override start = %v, want %v", override.Start, want)
	}
}

func TestCalendarWidgetBuildEventsJSON(t *testing.T) {
	widget := calendarWidget{
		Events: calendarEventList{
			{Summary: "All day <event>", Start: time.Date(2025, 11, 5, 0, 0, 0, 0, time.UTC),
				End: time.Date(2025, 11, 6, 0, 0, 0, 0, time.UTC), AllDay: true, Color: "#ff0000"},
			{Summary: "Timed event", Start: time.Date(2025, 11, 5, 9, 30, 0, 0, time.UTC),
				End: time.Date(2025, 11, 5, 10, 0, 0, 0, time.UTC), Color: "var(--color-primary)"},
		},
	}

	var events []calendarEventJSON
	if err := json.Unmarshal([]byte(widget.buildEventsJSON()), &events); err != nil {
		t.Fatalf("events JSON is not valid JSON: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}

	if !strings.Contains(string(widget.buildEventsJSON()), `\u003c`) {
		t.Errorf("events JSON must escape < to be safe inside a script tag")
	}

	if events[0].AllDay != true {
		t.Errorf("first event AllDay = false, want true")
	}
	if events[1].Start != 1762335000 {
		t.Errorf("second event start = %d, want %d", events[1].Start, 1762335000)
	}
}

func TestCalendarWidgetInitializeValidatesSubscriptions(t *testing.T) {
	widget := calendarWidget{
		FirstDayOfWeek: "monday",
		Calendars: []calendarSubscription{
			{URL: "https://example.com/calendar.ics", Color: "#ff0000"},
			{URL: "http://example.com/other.ics"},
		},
	}

	if err := widget.initialize(); err != nil {
		t.Fatalf("initialize() = %v, want nil", err)
	}

	if !widget.HasCalendars {
		t.Errorf("HasCalendars = false, want true")
	}

	// Unset colors get an auto-assigned theme color.
	if widget.Calendars[1].Color == "" {
		t.Errorf("auto color not assigned")
	}

	invalid := calendarWidget{
		FirstDayOfWeek: "monday",
		Calendars:      []calendarSubscription{{URL: "https://example.com/calendar.ics", Color: "red"}},
	}
	if err := invalid.initialize(); err == nil {
		t.Errorf("initialize() with non-hex color = nil, want error")
	}

	invalidURL := calendarWidget{
		FirstDayOfWeek: "monday",
		Calendars:      []calendarSubscription{{URL: "ftp://example.com/calendar.ics"}},
	}
	if err := invalidURL.initialize(); err == nil {
		t.Errorf("initialize() with non-http URL = nil, want error")
	}

	plain := calendarWidget{}
	if err := plain.initialize(); err != nil {
		t.Fatalf("initialize() without subscriptions = %v, want nil", err)
	}
	if plain.HasCalendars {
		t.Errorf("HasCalendars = true, want false for widget without subscriptions")
	}
}
