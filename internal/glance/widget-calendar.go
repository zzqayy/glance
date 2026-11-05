package glance

import (
	"context"
	"encoding/json"
	"errors"
	"html/template"
	"net/url"
	"time"
)
var calendarWidgetTemplate = mustParseTemplate("calendar.html", "widget-base.html")

var calendarWeekdaysToInt = map[string]time.Weekday{
	"sunday":    time.Sunday,
	"monday":    time.Monday,
	"tuesday":   time.Tuesday,
	"wednesday": time.Wednesday,
	"thursday":  time.Thursday,
	"friday":    time.Friday,
	"saturday":  time.Saturday,
}

type calendarWidget struct {
	widgetBase     `yaml:",inline"`
	FirstDayOfWeek string                 `yaml:"first-day-of-week"`
	FirstDay       int                    `yaml:"-"`
	Calendars      []calendarSubscription `yaml:"calendars"`
	Events         calendarEventList      `yaml:"-"`
	HasCalendars   bool                   `yaml:"-"`
	EventsJSON     string                 `yaml:"-"`
}

type calendarEventJSON struct {
	Summary string `json:"summary"`
	Start   int64  `json:"start"`
	End     int64  `json:"end"`
	AllDay  bool   `json:"allDay"`
	Color   string `json:"color"`
}

func (widget *calendarWidget) initialize() error {
	widget.withTitle(T("Calendar")).withError(nil)

	if widget.FirstDayOfWeek == "" {
		widget.FirstDayOfWeek = "monday"
	} else if _, ok := calendarWeekdaysToInt[widget.FirstDayOfWeek]; !ok {
		return errors.New("invalid first day of week")
	}

	widget.FirstDay = int(calendarWeekdaysToInt[widget.FirstDayOfWeek])

	if len(widget.Calendars) == 0 {
		return nil
	}

	for i := range widget.Calendars {
		subscription := &widget.Calendars[i]

		parsed, err := url.Parse(subscription.URL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return errors.New("calendar subscription URL is empty or not a valid http(s) URL")
		}

		if subscription.Color != "" && !calendarSubscriptionColorPattern.MatchString(subscription.Color) {
			return errors.New("calendar subscription color must be a hex color, e.g. #ff0000")
		} else if subscription.Color == "" {
			subscription.Color = calendarAutoColors[i%len(calendarAutoColors)]
		}
	}

	widget.HasCalendars = true
	widget.withCacheDuration(1 * time.Hour)

	return nil
}

func (widget *calendarWidget) update(ctx context.Context) {
	if !widget.HasCalendars {
		return
	}

	events, err := widget.fetchEventsFromCalendars()
	if !widget.canContinueUpdateAfterHandlingErr(err) {
		return
	}

	widget.Events = events
}

func (widget *calendarWidget) Render() template.HTML {
	widget.EventsJSON = widget.buildEventsJSON()

	return widget.renderTemplate(widget, calendarWidgetTemplate)
}

func (widget *calendarWidget) buildEventsJSON() string {
	events := make([]calendarEventJSON, 0, len(widget.Events))

	for i := range widget.Events {
		event := &widget.Events[i]
		events = append(events, calendarEventJSON{
			Summary: event.Summary,
			Start:   event.Start.Unix(),
			End:     event.End.Unix(),
			AllDay:  event.AllDay,
			Color:   event.Color,
		})
	}

	marshalled, err := json.Marshal(events)
	if err != nil {
		return "[]"
	}

	return string(marshalled)
}
