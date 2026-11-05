import { directions, easeOutQuint, slideFade } from "./animations.js";
import { elem, repeat, text } from "./templating.js";
import { t, monthNames as MONTH_NAMES, weekDayAbbrs as WEEKDAY_ABBRS, localizedDate } from "./i18n.js";

const FULL_MONTH_SLOTS = 7*6;
const MAX_DOTS_PER_DAY = 3;

const leftArrowSvg = `<svg stroke="var(--color-text-base)" fill="none" viewBox="0 0 24 24" stroke-width="1.5" xmlns="http://www.w3.org/2000/svg">
  <path stroke-linecap="round" stroke-linejoin="round" d="M15.75 19.5 8.25 12l7.5-7.5" />
</svg>`;

const rightArrowSvg = `<svg stroke="var(--color-text-base)" fill="none" viewBox="0 0 24 24" stroke-width="1.5" xmlns="http://www.w3.org/2000/svg">
  <path stroke-linecap="round" stroke-linejoin="round" d="m8.25 4.5 7.5 7.5-7.5 7.5" />
</svg>`;

const undoArrowSvg = `<svg stroke="var(--color-text-base)" fill="none" viewBox="0 0 24 24" stroke-width="1.5" xmlns="http://www.w3.org/2000/svg">
  <path stroke-linecap="round" stroke-linejoin="round" d="M9 15 3 9m0 0 6-6M3 9h12a6 6 0 0 1 0 12h-3" />
</svg>`;

const [datesExitLeft, datesExitRight] = directions(
    slideFade, { distance: "2rem", duration: 120, offset: 1 },
    "left", "right"
);

const [datesEntranceLeft, datesEntranceRight] = directions(
    slideFade, { distance: "0.8rem", duration: 500, easing: easeOutQuint },
    "left", "right"
);

const undoEntrance = slideFade({ direction: "left", distance: "100%", duration: 300 });

export default function(element) {
    let events = [];

    if (element.dataset.events) {
        try {
            events = JSON.parse(element.dataset.events);
        } catch {
            events = [];
        }
    }

    element.swapWith(Calendar(
        Number(element.dataset.firstDayOfWeek ?? 1),
        events
    ));
}

// TODO: when viewing the previous/next month, display the current date if it's within the spill-over days
function Calendar(firstDay, events) {
    let header, dates, eventsList;
    let advanceTimeTicker;
    let now = new Date();
    let activeDate;

    const update = (newDate) => {
        header.component.update(now, newDate);
        dates.component.update(now, newDate);
        eventsList.component.update(now, newDate);
        activeDate = newDate;
    };

    const autoAdvanceNow = () => {
        advanceTimeTicker = setTimeout(() => {
            // TODO: don't auto advance if looking at a different month
            update(now = new Date());
            autoAdvanceNow();
        }, msTillNextDay());
    };

    const adjacentMonth = (dir) => new Date(activeDate.getFullYear(), activeDate.getMonth() + dir, 1);
    const nextClicked = () => update(adjacentMonth(1));
    const prevClicked = () => update(adjacentMonth(-1));
    const undoClicked = () => update(now);

    const calendar = elem().classes("calendar").append(
        header = Header(nextClicked, prevClicked, undoClicked),
        dates = Dates(firstDay, buildEventIndex(events)),
        eventsList = EventsList(events)
    );

    update(now);
    autoAdvanceNow();

    return calendar.component({
        suspend: () => clearTimeout(advanceTimeTicker)
    });
}

function Header(nextClicked, prevClicked, undoClicked) {
    let month, monthNumber, year, undo;
    const button = () => elem("button").classes("calendar-header-button");

    const monthAndYear = elem().classes("size-h2", "color-highlight").append(
        month = text(),
        " ",
        year = elem("span").classes("size-h3"),
        undo = button()
            .hide()
            .classes("calendar-undo-button")
            .attr("title", t("Back to current month"))
            .on("click", undoClicked)
            .html(undoArrowSvg)
    );

    const monthSwitcher = elem()
        .classes("flex", "gap-7", "items-center")
        .append(
            button()
                .attr("title", t("Previous month"))
                .on("click", prevClicked)
                .html(leftArrowSvg),
            monthNumber = elem()
                .classes("color-highlight")
                .styles({ marginTop: "0.1rem" }),
            button()
                .attr("title", t("Next month"))
                .on("click", nextClicked)
                .html(rightArrowSvg),
        );

    return elem().classes("flex", "justify-between", "items-center").append(
        monthAndYear,
        monthSwitcher
    ).component({
        update: function (now, newDate) {
            month.text(MONTH_NAMES[newDate.getMonth()]);
            year.text(newDate.getFullYear());
            const m = newDate.getMonth() + 1;
            monthNumber.text((m < 10 ? "0" : "") + m);

            if (!datesWithinSameMonth(now, newDate)) {
                if (undo.isHidden()) undo.show().animate(undoEntrance);
            } else {
                undo.hide();
            }

            return this;
        }
    });
}

function Dates(firstDay, eventsByDay) {
    let dates, lastRenderedDate;

    const setCellContents = function(cell, numberText, dayKey) {
        cell.childNodes[0].text(numberText);

        const dots = cell.childNodes[1];
        dots.replaceChildren();

        const colors = dayColors(dayKey, eventsByDay);

        for (const color of colors) {
            dots.append(elem("span").classes("calendar-event-dot").styles({ backgroundColor: color }));
        }

        cell.classesIf(colors.length > 0, "calendar-has-events");
    };

    const updateFullMonth = function(now, newDate) {
        const firstWeekday = new Date(newDate.getFullYear(), newDate.getMonth(), 1).getDay();
        const previousMonthSpilloverDays = (firstWeekday - firstDay + 7) % 7 || 7;
        const currentMonthDays = daysInMonth(newDate.getFullYear(), newDate.getMonth());
        const nextMonthSpilloverDays = FULL_MONTH_SLOTS - (previousMonthSpilloverDays + currentMonthDays);
        const previousMonthDays = daysInMonth(newDate.getFullYear(), newDate.getMonth() - 1)
        const previousMonthDate = new Date(newDate.getFullYear(), newDate.getMonth() - 1, 1);
        const nextMonthDate = new Date(newDate.getFullYear(), newDate.getMonth() + 1, 1);
        const isCurrentMonth = datesWithinSameMonth(now, newDate);
        const currentDate = now.getDate();

        const dayKey = (y, m, d) => `${y}-${m}-${d}`;

        let children = dates.children;
        let index = 0;

        for (let i = 0; i < FULL_MONTH_SLOTS; i++) {
            children[i].clearClasses("calendar-spillover-date", "calendar-current-date", "calendar-has-events");
        }

        for (let i = 0; i < previousMonthSpilloverDays; i++, index++) {
            const day = previousMonthDays - previousMonthSpilloverDays + i + 1;
            setCellContents(
                children[index],
                day,
                dayKey(previousMonthDate.getFullYear(), previousMonthDate.getMonth(), day)
            );
            children[index].classes("calendar-spillover-date");
        }

        for (let i = 1; i <= currentMonthDays; i++, index++) {
            setCellContents(
                children[index],
                i,
                dayKey(newDate.getFullYear(), newDate.getMonth(), i)
            );
            children[index].classesIf(isCurrentMonth && i === currentDate, "calendar-current-date");
        }

        for (let i = 0; i < nextMonthSpilloverDays; i++, index++) {
            setCellContents(
                children[index],
                i + 1,
                dayKey(nextMonthDate.getFullYear(), nextMonthDate.getMonth(), i + 1)
            );
            children[index].classes("calendar-spillover-date");
        }

        lastRenderedDate = newDate;
    };

    const update = function(now, newDate) {
        if (lastRenderedDate === undefined || datesWithinSameMonth(newDate, lastRenderedDate)) {
            updateFullMonth(now, newDate);
            return;
        }

        const next = newDate > lastRenderedDate;
        dates.animateUpdate(
            () => updateFullMonth(now, newDate),
            next ? datesExitLeft : datesExitRight,
            next ? datesEntranceRight : datesEntranceLeft,
        );
    }

    return elem().append(
        elem().classes("calendar-dates", "margin-top-15").append(
            ...repeat(7, (i) => elem().classes("size-h6", "color-subdue").text(
                WEEKDAY_ABBRS[(firstDay + i) % 7]
            ))
        ),

        dates = elem().classes("calendar-dates", "margin-top-3").append(
            ...repeat(FULL_MONTH_SLOTS, () => elem().classes("calendar-date").append(
                elem().classes("calendar-date-number"),
                elem().classes("calendar-date-dots"),
            ))
        )
    ).component({ update });
}

function EventsList(events) {
    let list;

    const container = elem()
        .classes("calendar-events", "margin-top-15")
        .append(list = elem("ul").classes("calendar-events-list"))
        .hide();

    const update = function(now, newDate) {
        list.replaceChildren();

        const year = newDate.getFullYear();
        const month = newDate.getMonth();
        let shown = 0;

        for (const event of events) {
            const startDate = new Date(event.start * 1000);
            const [y, m] = event.allDay
                ? [startDate.getUTCFullYear(), startDate.getUTCMonth()]
                : [startDate.getFullYear(), startDate.getMonth()];

            if (y !== year || m !== month) continue;

            list.append(buildEventItem(event, startDate));
            shown++;
        }

        container.showIf(shown > 0);

        return this;
    };

    return container.component({ update });
}

function buildEventItem(event, startDate) {
    let timeText;

    if (event.allDay) {
        const endDate = event.end && event.end !== event.start
            ? new Date(event.end * 1000)
            : null;

        if (endDate) {
            const lastDay = new Date(endDate.getTime() - 1);
            timeText = `${localizedDate(startDate)} – ${localizedDate(lastDay)} ${t("All day")}`;
        } else {
            timeText = `${localizedDate(startDate)} ${t("All day")}`;
        }
    } else {
        timeText = `${localizedDate(startDate)} ${formatTime(startDate)}`;
    }

    return elem("li").classes("calendar-event").append(
        elem("span").classes("calendar-event-indicator").styles({ backgroundColor: event.color }),
        elem("span").classes("calendar-event-time", "size-h6", "color-subdue").text(timeText),
        elem("span").classes("calendar-event-title").text(event.summary).attr("title", event.summary),
    );
}

function buildEventIndex(events) {
    const byDay = new Map();

    const keyOf = (date, allDay) => allDay
        ? `${date.getUTCFullYear()}-${date.getUTCMonth()}-${date.getUTCDate()}`
        : `${date.getFullYear()}-${date.getMonth()}-${date.getDate()}`;

    for (const event of events) {
        const startDate = new Date(event.start * 1000);

        if (!event.allDay) {
            pushEvent(byDay, keyOf(startDate, false), event);
            continue;
        }

        const endDate = event.end && event.end !== event.start
            ? new Date(event.end * 1000)
            : null;

        let cursor = startDate;

        for (let i = 0; i < 366; i++) {
            pushEvent(byDay, keyOf(cursor, true), event);

            if (endDate === null || datesWithinSameUTCDay(cursor, endDate)) break;

            cursor = new Date(Date.UTC(
                cursor.getUTCFullYear(),
                cursor.getUTCMonth(),
                cursor.getUTCDate() + 1
            ));
        }
    }

    return byDay;
}

function pushEvent(byDay, key, event) {
    const eventsOfDay = byDay.get(key);

    if (eventsOfDay) {
        eventsOfDay.push(event);
    } else {
        byDay.set(key, [event]);
    }
}

function dayColors(dayKey, eventsByDay) {
    const colors = [];

    for (const event of eventsByDay.get(dayKey) ?? []) {
        if (!colors.includes(event.color)) colors.push(event.color);
        if (colors.length === MAX_DOTS_PER_DAY) break;
    }

    return colors;
}

function datesWithinSameUTCDay(d1, d2) {
    return d1.getUTCFullYear() === d2.getUTCFullYear()
        && d1.getUTCMonth() === d2.getUTCMonth()
        && d1.getUTCDate() === d2.getUTCDate();
}

function formatTime(date) {
    const hours = date.getHours();
    const minutes = date.getMinutes();

    return `${hours < 10 ? "0" : ""}${hours}:${minutes < 10 ? "0" : ""}${minutes}`;
}

function datesWithinSameMonth(d1, d2) {
    return d1.getFullYear() === d2.getFullYear() && d1.getMonth() === d2.getMonth();
}

function daysInMonth(year, month) {
    return new Date(year, month + 1, 0).getDate();
}

function msTillNextDay(now) {
    now = now || new Date();

    return 86_400_000 - (
      now.getMilliseconds() +
      now.getSeconds() * 1000 +
      now.getMinutes() * 60_000 +
      now.getHours() * 3_600_000
    );
}
