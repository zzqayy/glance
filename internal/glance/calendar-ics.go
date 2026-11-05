package glance

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	ics "github.com/arran4/golang-ical"
	"github.com/teambition/rrule-go"
)

// auto-assigned event colors, cycled by subscription index, overridable via the
// color property of a subscription.
var calendarAutoColors = []string{
	"var(--color-primary)",
	"var(--color-positive)",
	"var(--color-negative)",
	"var(--color-text-highlight)",
}

var calendarSubscriptionColorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{3,8}$`)

var calendarDurationPattern = regexp.MustCompile(
	`^P(?:(\d+)W)?(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?)?$`,
)

type calendarSubscription struct {
	URL   string `yaml:"url"`
	Color string `yaml:"color"`
}

type calendarEvent struct {
	Summary string
	Start   time.Time
	End     time.Time
	AllDay  bool
	Color   string
}

type calendarEventList []calendarEvent

func (l calendarEventList) sortChronologically() {
	sort.Slice(l, func(i, j int) bool {
		if !l[i].Start.Equal(l[j].Start) {
			return l[i].Start.Before(l[j].Start)
		}

		return l[i].Summary < l[j].Summary
	})
}

// calendarEventWindow returns the [start, end) window in which events get
// fetched and recurring events get expanded: from the first day of the
// previous month up until the first day of the month 13 months ahead.
func calendarEventWindow(now time.Time) (time.Time, time.Time) {
	y, m, _ := now.Date()
	start := time.Date(y, m-1, 1, 0, 0, 0, 0, now.Location())
	end := time.Date(y, m+13, 1, 0, 0, 0, 0, now.Location())

	return start, end
}

func (widget *calendarWidget) fetchEventsFromCalendars() (calendarEventList, error) {
	subscriptions := widget.Calendars
	windowStart, windowEnd := calendarEventWindow(time.Now())

	job := newJob(func(s calendarSubscription) ([]calendarEvent, error) {
		return widget.fetchEventsFromSubscription(s, windowStart, windowEnd)
	}, subscriptions).withWorkers(10)

	perSubscription, errs, err := workerPoolDo(job)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errNoContent, err)
	}

	failed := 0
	events := make(calendarEventList, 0, len(perSubscription)*10)

	for i := range perSubscription {
		if errs[i] != nil {
			failed++
			slog.Error("Failed to fetch calendar subscription", "url", subscriptions[i].URL, "error", errs[i])
			continue
		}

		events = append(events, perSubscription[i]...)
	}

	events.sortChronologically()

	if failed == len(subscriptions) {
		return nil, errNoContent
	}

	if failed > 0 {
		return events, fmt.Errorf("%w: missing %d calendar subscriptions", errPartialContent, failed)
	}

	return events, nil
}

func (widget *calendarWidget) fetchEventsFromSubscription(
	subscription calendarSubscription,
	windowStart, windowEnd time.Time,
) ([]calendarEvent, error) {
	req, err := http.NewRequest("GET", subscription.URL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("User-Agent", glanceUserAgentString)

	resp, err := defaultHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d from %s", resp.StatusCode, subscription.URL)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return nil, err
	}

	parsed, err := ics.ParseCalendar(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("parsing iCal file: %v", err)
	}

	return parseCalendarEvents(parsed, subscription.Color, windowStart, windowEnd)
}

func parseCalendarEvents(
	calendar *ics.Calendar,
	color string,
	windowStart, windowEnd time.Time,
) ([]calendarEvent, error) {
	events := make([]calendarEvent, 0, len(calendar.Events()))
	overridesByUID := make(map[string][]time.Time)
	overrides := make([]*ics.VEvent, 0)

	for _, event := range calendar.Events() {
		if event.GetProperty(ics.ComponentPropertyRecurrenceId) == nil {
			continue
		}

		overrides = append(overrides, event)

		if uidProp := event.GetProperty(ics.ComponentPropertyUniqueId); uidProp != nil {
			if rid, err := parseRecurrenceID(event); err == nil {
				overridesByUID[uidProp.Value] = append(overridesByUID[uidProp.Value], rid)
			}
		}
	}

	for _, event := range calendar.Events() {
		if event.GetProperty(ics.ComponentPropertyRecurrenceId) != nil {
			continue
		}

		base, err := parseEventInstance(event, color)
		if err != nil {
			slog.Debug("Skipping calendar event", "uid", event.Id(), "error", err)
			continue
		}

		instances, err := expandRecurringEvent(event, base, windowStart, windowEnd, overridesByUID)
		if err != nil {
			slog.Debug("Failed to expand recurring calendar event", "uid", event.Id(), "error", err)
			instances = []time.Time{base.Start}
		}

		for _, start := range instances {
			instance := base
			instance.Start = start
			instance.End = start.Add(base.End.Sub(base.Start))
			events = append(events, instance)
		}
	}

	for _, override := range overrides {
		instance, err := parseEventInstance(override, color)
		if err != nil {
			slog.Debug("Skipping overridden calendar event", "uid", override.Id(), "error", err)
			continue
		}

		if instance.Start.Before(windowStart) || !instance.Start.Before(windowEnd) {
			continue
		}

		events = append(events, instance)
	}

	return events, nil
}

// expandRecurringEvent returns the start times of all instances of an event
// within the given window. Non-recurring events return a single instance. The
// occurrences of events that were overridden by a RECURRENCE-ID event sharing
// the same UID are excluded, as those are added separately.
func expandRecurringEvent(
	event *ics.VEvent,
	base calendarEvent,
	windowStart, windowEnd time.Time,
	overridesByUID map[string][]time.Time,
) ([]time.Time, error) {
	ruleProp := event.GetProperty(ics.ComponentPropertyRrule)
	if ruleProp == nil {
		return []time.Time{base.Start}, nil
	}

	rule, err := rrule.StrToRRule(ruleProp.Value)
	if err != nil {
		return nil, fmt.Errorf("parsing RRULE: %v", err)
	}

	if freq := rule.OrigOptions.Freq; freq == rrule.HOURLY ||
		freq == rrule.MINUTELY || freq == rrule.SECONDLY {
		return []time.Time{base.Start}, nil
	}

	exdates, err := parseEventDateList(event, ics.ComponentPropertyExdate, base.AllDay)
	if err != nil {
		return nil, fmt.Errorf("parsing EXDATE: %v", err)
	}

	rdates, err := parseEventDateList(event, ics.ComponentPropertyRdate, base.AllDay)
	if err != nil {
		return nil, fmt.Errorf("parsing RDATE: %v", err)
	}

	if uidProp := event.GetProperty(ics.ComponentPropertyUniqueId); uidProp != nil {
		exdates = append(exdates, overridesByUID[uidProp.Value]...)
	}

	set := &rrule.Set{}
	set.RRule(rule)
	set.DTStart(base.Start)
	set.SetExDates(exdates)

	for _, rdate := range rdates {
		set.RDate(rdate)
	}

	occurrences := set.Between(windowStart, windowEnd, true)
	if len(occurrences) == 0 {
		occurrences = []time.Time{base.Start}
	}

	return occurrences, nil
}

func parseEventInstance(event *ics.VEvent, color string) (calendarEvent, error) {
	instance := calendarEvent{Color: color, AllDay: false}

	dtstart := event.GetProperty(ics.ComponentPropertyDtStart)
	if dtstart == nil {
		return instance, errors.New("event has no DTSTART")
	}

	instance.AllDay = isAllDayTimeValue(dtstart)

	var err error

	if instance.AllDay {
		instance.Start, err = event.GetAllDayStartAt()
		if err != nil {
			return instance, fmt.Errorf("parsing DTSTART: %v", err)
		}

		instance.Start = utcMidnight(instance.Start)

		if dtend := event.GetProperty(ics.ComponentPropertyDtEnd); dtend != nil {
			if instance.End, err = event.GetAllDayEndAt(); err != nil {
				return instance, fmt.Errorf("parsing DTEND: %v", err)
			}

			instance.End = utcMidnight(instance.End)
		} else {
			instance.End = instance.Start
		}
	} else {
		instance.Start, err = event.GetStartAt()
		if err != nil {
			return instance, fmt.Errorf("parsing DTSTART: %v", err)
		}

		instance.End, err = event.GetEndAt()
		if err != nil {
			if instance.End, err = eventEndFromDuration(event, instance.Start); err != nil {
				instance.End = instance.Start
			}
		}
	}

	if summary := event.GetProperty(ics.ComponentPropertySummary); summary != nil {
		instance.Summary = summary.Value
	}

	return instance, nil
}

// parseEventDateList collects the values of all properties with the given
// name, e.g. EXDATE or RDATE, which may hold multiple comma-separated values.
// For all-day events date-only values are returned as UTC midnight, otherwise
// they are parsed in the location of the DTSTART's TZID, or the local timezone
// when the value carries no timezone information of its own.
func parseEventDateList(event *ics.VEvent, property ics.ComponentProperty, allDay bool) ([]time.Time, error) {
	var times []time.Time

	for _, prop := range event.GetProperties(property) {
		for _, value := range strings.Split(prop.Value, ",") {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}

			if allDay {
				parsed, err := time.Parse("20060102", value)
				if err != nil {
					return nil, fmt.Errorf("parsing %s value %q: %v", property, value, err)
				}

				times = append(times, time.Date(
					parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, time.UTC,
				))

				continue
			}

			parsed, err := parseDateTimeValue(value, prop.ICalParameters)
			if err != nil {
				return nil, fmt.Errorf("parsing %s value %q: %v", property, value, err)
			}

			times = append(times, parsed)
		}
	}

	return times, nil
}

func parseRecurrenceID(event *ics.VEvent) (time.Time, error) {
	prop := event.GetProperty(ics.ComponentPropertyRecurrenceId)
	if prop == nil {
		return time.Time{}, errors.New("event has no RECURRENCE-ID")
	}

	if isAllDayTimeValue(prop) {
		parsed, err := time.Parse("20060102", prop.Value)
		if err != nil {
			return time.Time{}, fmt.Errorf("parsing RECURRENCE-ID: %v", err)
		}

		return time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, time.UTC), nil
	}

	return parseDateTimeValue(prop.Value, prop.ICalParameters)
}

// parseDateTimeValue parses an iCal date-time value, honoring its TZID
// parameter and the UTC designator. Values without timezone information are
// treated as floating times in the server's local timezone, and TZIDs are
// resolved through the IANA timezone database, with Windows timezone names
// being mapped beforehand.
func parseDateTimeValue(value string, params map[string][]string) (time.Time, error) {
	layouts := []string{"20060102T150405Z", "20060102T150405", "20060102"}

	location := time.Local

	if tzids, ok := params["TZID"]; ok && len(tzids) > 0 {
		var err error
		if location, err = resolveICSLocation(tzids[0]); err != nil {
			return time.Time{}, err
		}
	}

	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, value, location); err == nil {
			return parsed, nil
		}
	}

	return time.Time{}, fmt.Errorf("unsupported date-time value %q", value)
}

func resolveICSLocation(tzid string) (*time.Location, error) {
	if location := ics.WindowsTimezoneToIANA(tzid); location != nil {
		return location, nil
	}

	if location, err := time.LoadLocation(tzid); err == nil {
		return location, nil
	}

	// TZID parameters sometimes carry a prefix, e.g. "/freeassociation
	// .sourceforge.net/Europe/Berlin". The last component matching an IANA
	// timezone is used as a fallback.
	if idx := strings.LastIndex(tzid, "/"); idx != -1 {
		if location, err := time.LoadLocation(tzid[idx+1:]); err == nil {
			return location, nil
		}
	}

	return nil, fmt.Errorf("unknown timezone %q", tzid)
}

func isAllDayTimeValue(prop *ics.IANAProperty) bool {
	if values, ok := prop.ICalParameters["VALUE"]; ok {
		for _, value := range values {
			if value == string(ics.ValueDataTypeDate) {
				return true
			}
		}
	}

	return len(prop.Value) == 8
}

// utcMidnight normalizes an all-day time value to midnight UTC of the calendar
// date it represents, so that its date can be recovered in any timezone.
func utcMidnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// eventEndFromDuration computes the end of an event that specifies a DURATION
// property instead of a DTEND.
func eventEndFromDuration(event *ics.VEvent, start time.Time) (time.Time, error) {
	duration := event.GetProperty(ics.ComponentPropertyDuration)
	if duration == nil {
		return time.Time{}, errors.New("event has no DTEND or DURATION")
	}

	matches := calendarDurationPattern.FindStringSubmatch(strings.ToUpper(strings.TrimSpace(duration.Value)))
	if matches == nil {
		return time.Time{}, fmt.Errorf("unsupported DURATION value %q", duration.Value)
	}

	atoi := func(s string) int {
		n, _ := strconv.Atoi(s)
		return n
	}

	weeks := atoi(matches[1])
	days := atoi(matches[2])
	hours := atoi(matches[3])
	minutes := atoi(matches[4])
	seconds := atoi(matches[5])

	return start.AddDate(0, 0, weeks*7+days).
		Add(time.Duration(hours)*time.Hour + time.Duration(minutes)*time.Minute + time.Duration(seconds)*time.Second), nil
}
