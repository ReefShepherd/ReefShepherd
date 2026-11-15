package schedule

import (
	"math/rand"
	"testing"
	"time"
)

type dueCase struct {
	name    string
	s       Schedule
	loc     string
	last    string // RFC 3339, empty when never done
	created string // RFC 3339, defaults to a time long before everything
	want    string // RFC 3339, empty when no due time
}

func runDueCases(t *testing.T, cases []dueCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			loc := loadLoc(t, tc.loc)
			var last *time.Time
			if tc.last != "" {
				l := ts(t, tc.last)
				last = &l
			}
			created := ts(t, "2020-01-01T00:00:00Z")
			if tc.created != "" {
				created = ts(t, tc.created)
			}
			got, ok := NextDue(tc.s, last, created, loc)
			if tc.want == "" {
				if ok {
					t.Fatalf("NextDue = %s, want no due time", got.Format(time.RFC3339))
				}
				return
			}
			if !ok {
				t.Fatal("NextDue reported no due time")
			}
			if want := ts(t, tc.want); !got.Equal(want) {
				t.Errorf("NextDue = %s, want %s", got.In(loc).Format(time.RFC3339), tc.want)
			}
		})
	}
}

const (
	ny  = "America/New_York"
	lon = "Europe/London"
	syd = "Australia/Sydney"
	utc = "UTC"
)

func TestNextDueAfterLast(t *testing.T) {
	h9, h130, h230 := tod(9, 0), tod(1, 30), tod(2, 30)
	runDueCases(t, []dueCase{
		{name: "seven days after", s: afterLast(7, h9), loc: ny, last: "2026-10-24T10:00:00-04:00", want: "2026-10-31T09:00:00-04:00"},
		{name: "one day", s: afterLast(1, h9), loc: ny, last: "2026-10-24T10:00:00-04:00", want: "2026-10-25T09:00:00-04:00"},
		{name: "late evening counts as that local day", s: afterLast(7, h9), loc: ny, last: "2026-10-24T23:30:00-04:00", want: "2026-10-31T09:00:00-04:00"},
		{name: "same instant is the next UTC day in UTC", s: afterLast(7, h9), loc: utc, last: "2026-10-24T23:30:00-04:00", want: "2026-11-01T09:00:00Z"},
		{name: "month end", s: afterLast(3, h9), loc: ny, last: "2026-01-30T12:00:00-05:00", want: "2026-02-02T09:00:00-05:00"},
		{name: "year end", s: afterLast(5, h9), loc: utc, last: "2026-12-30T12:00:00Z", want: "2027-01-04T09:00:00Z"},
		{name: "leap day reached", s: afterLast(2, h9), loc: utc, last: "2028-02-27T12:00:00Z", want: "2028-02-29T09:00:00Z"},
		{name: "no leap day", s: afterLast(2, h9), loc: utc, last: "2027-02-27T12:00:00Z", want: "2027-03-01T09:00:00Z"},
		{name: "leap day to march", s: afterLast(1, h9), loc: utc, last: "2028-02-29T12:00:00Z", want: "2028-03-01T09:00:00Z"},
		{name: "year long interval", s: afterLast(365, h9), loc: utc, last: "2027-03-01T12:00:00Z", want: "2028-02-29T09:00:00Z"},
		{name: "ny autumn change counts calendar days", s: afterLast(3, h9), loc: ny, last: "2026-10-30T12:00:00-04:00", want: "2026-11-02T09:00:00-05:00"},
		{name: "ny spring change counts calendar days", s: afterLast(3, h9), loc: ny, last: "2026-03-06T12:00:00-05:00", want: "2026-03-09T09:00:00-04:00"},
		{name: "ny due time in gap", s: afterLast(2, h230), loc: ny, last: "2026-03-06T12:00:00-05:00", want: "2026-03-08T03:30:00-04:00"},
		{name: "ny due time in overlap takes first", s: afterLast(2, h130), loc: ny, last: "2026-10-30T12:00:00-04:00", want: "2026-11-01T01:30:00-04:00"},
		{name: "london gap", s: afterLast(2, h130), loc: lon, last: "2026-03-27T12:00:00Z", want: "2026-03-29T02:30:00+01:00"},
		{name: "london overlap", s: afterLast(2, h130), loc: lon, last: "2026-10-23T12:00:00+01:00", want: "2026-10-25T01:30:00+01:00"},
		{name: "sydney gap", s: afterLast(2, h230), loc: syd, last: "2026-10-02T12:00:00+10:00", want: "2026-10-04T03:30:00+11:00"},
		{name: "sydney overlap", s: afterLast(2, h230), loc: syd, last: "2027-04-02T12:00:00+11:00", want: "2027-04-04T02:30:00+11:00"},
		{name: "sydney into daylight time", s: afterLast(1, h9), loc: syd, last: "2026-10-03T23:00:00+10:00", want: "2026-10-04T09:00:00+11:00"},
		{name: "never done is due immediately", s: afterLast(7, h9), loc: ny, created: "2026-10-24T15:00:00-04:00", want: "2026-10-24T15:00:00-04:00"},
		{name: "done in the distant past is overdue", s: afterLast(7, h9), loc: utc, last: "2020-01-01T00:00:00Z", want: "2020-01-08T09:00:00Z"},
	})
}

func TestNextDueEveryNDays(t *testing.T) {
	h9 := tod(9, 0)
	grid3 := func() Schedule { return everyNDays(t, 3, "2026-11-01", h9) } // 11-01, 11-04, 11-07 ...
	runDueCases(t, []dueCase{
		{name: "never done waits for anchor", s: grid3(), loc: ny, created: "2026-10-20T08:00:00-04:00", want: "2026-11-01T09:00:00-05:00"},
		{name: "created before the due time on anchor day", s: grid3(), loc: ny, created: "2026-11-01T08:59:00-05:00", want: "2026-11-01T09:00:00-05:00"},
		{name: "created exactly at due time", s: grid3(), loc: ny, created: "2026-11-01T09:00:00-05:00", want: "2026-11-01T09:00:00-05:00"},
		{name: "created just after due time", s: grid3(), loc: ny, created: "2026-11-01T09:01:00-05:00", want: "2026-11-04T09:00:00-05:00"},
		{name: "created between grid days", s: grid3(), loc: ny, created: "2026-11-02T08:00:00-05:00", want: "2026-11-04T09:00:00-05:00"},
		{name: "created after due time on grid day", s: grid3(), loc: ny, created: "2026-11-04T10:00:00-05:00", want: "2026-11-07T09:00:00-05:00"},
		{name: "done on the day", s: grid3(), loc: ny, last: "2026-11-04T12:00:00-05:00", want: "2026-11-07T09:00:00-05:00"},
		{name: "done exactly at the occurrence", s: grid3(), loc: ny, last: "2026-11-04T09:00:00-05:00", want: "2026-11-07T09:00:00-05:00"},
		{name: "done a day after the earlier occurrence", s: grid3(), loc: ny, last: "2026-11-02T08:00:00-05:00", want: "2026-11-04T09:00:00-05:00"},
		{name: "done late the day after the earlier occurrence still counts for it", s: grid3(), loc: ny, last: "2026-11-02T22:00:00-05:00", want: "2026-11-04T09:00:00-05:00"},
		{name: "done midway ties to the earlier occurrence", s: everyNDays(t, 4, "2026-11-01", h9), loc: ny, last: "2026-11-03T22:00:00-05:00", want: "2026-11-05T09:00:00-05:00"},
		{name: "daily, done late in the evening still makes tomorrow due", s: everyNDays(t, 1, "2026-01-01", h9), loc: ny, last: "2026-10-24T21:30:00-04:00", want: "2026-10-25T09:00:00-04:00"},
		{name: "daily, done just after midnight counts for that day", s: everyNDays(t, 1, "2026-01-01", h9), loc: ny, last: "2026-10-25T00:10:00-04:00", want: "2026-10-26T09:00:00-04:00"},
		{name: "done the day before an occurrence", s: grid3(), loc: ny, last: "2026-11-03T12:00:00-05:00", want: "2026-11-07T09:00:00-05:00"},
		{name: "done just before the anchor, grid extends backwards", s: grid3(), loc: ny, last: "2026-10-30T12:00:00-04:00", want: "2026-11-01T09:00:00-05:00"},
		{name: "done the day before the anchor satisfies it", s: grid3(), loc: ny, last: "2026-10-31T12:00:00-04:00", want: "2026-11-04T09:00:00-05:00"},
		{name: "done long before the anchor waits for the anchor", s: grid3(), loc: ny, last: "2026-06-01T12:00:00-04:00", want: "2026-11-01T09:00:00-05:00"},
		{name: "done long ago on a running grid is overdue", s: everyNDays(t, 3, "2026-01-01", h9), loc: ny, last: "2026-08-01T12:00:00-04:00", want: "2026-08-05T09:00:00-04:00"},
		{name: "daily, done before the due time", s: everyNDays(t, 1, "2026-01-01", h9), loc: ny, last: "2026-10-24T08:00:00-04:00", want: "2026-10-25T09:00:00-04:00"},
		{name: "daily, done after the due time", s: everyNDays(t, 1, "2026-01-01", h9), loc: ny, last: "2026-10-24T10:00:00-04:00", want: "2026-10-25T09:00:00-04:00"},
		{name: "month end crossing", s: everyNDays(t, 7, "2026-01-31", h9), loc: utc, created: "2026-01-31T10:00:00Z", want: "2026-02-07T09:00:00Z"},
		{name: "month end anchor day itself", s: everyNDays(t, 7, "2026-01-31", h9), loc: utc, created: "2026-01-01T00:00:00Z", want: "2026-01-31T09:00:00Z"},
		{name: "thirty days over february", s: everyNDays(t, 30, "2026-01-01", h9), loc: utc, last: "2026-01-31T09:00:00Z", want: "2026-03-02T09:00:00Z"},
		{name: "leap day on the grid", s: everyNDays(t, 14, "2028-02-01", h9), loc: utc, last: "2028-02-15T09:00:00Z", want: "2028-02-29T09:00:00Z"},
		{name: "no leap day", s: everyNDays(t, 14, "2027-02-01", h9), loc: utc, last: "2027-02-15T09:00:00Z", want: "2027-03-01T09:00:00Z"},
		{name: "yearly grid", s: everyNDays(t, 365, "2026-01-01", h9), loc: utc, created: "2026-02-01T00:00:00Z", want: "2027-01-01T09:00:00Z"},
		{name: "ny spring change, 23 hour day", s: everyNDays(t, 1, "2026-03-07", h9), loc: ny, last: "2026-03-07T10:00:00-05:00", want: "2026-03-08T09:00:00-04:00"},
		{name: "ny due time in gap", s: everyNDays(t, 1, "2026-03-06", tod(2, 30)), loc: ny, created: "2026-03-07T12:00:00-05:00", want: "2026-03-08T03:30:00-04:00"},
		{name: "ny done at the shifted gap occurrence", s: everyNDays(t, 1, "2026-03-06", tod(2, 30)), loc: ny, last: "2026-03-08T03:30:00-04:00", want: "2026-03-09T02:30:00-04:00"},
		{name: "ny done at the second pass of the overlap", s: everyNDays(t, 1, "2026-10-30", tod(1, 30)), loc: ny, last: "2026-11-01T01:30:00-05:00", want: "2026-11-02T01:30:00-05:00"},
		{name: "london across clocks going back", s: everyNDays(t, 2, "2026-10-21", h9), loc: lon, last: "2026-10-25T12:00:00Z", want: "2026-10-27T09:00:00Z"},
		{name: "sydney across clocks going forward", s: everyNDays(t, 2, "2026-10-02", h9), loc: syd, last: "2026-10-04T12:00:00+11:00", want: "2026-10-06T09:00:00+11:00"},
	})
}

func TestNextDueWeekly(t *testing.T) {
	h9 := tod(9, 0)
	sunday := func() Schedule { return weekly(t, Sun, 1, "", h9) }
	mwf := func() Schedule { return weekly(t, Mon|Wed|Fri, 1, "", h9) }
	fortnightSat := func() Schedule { return weekly(t, Sat, 2, "2026-10-24", h9) } // Sat 10-24, 11-07, 11-21 ...
	runDueCases(t, []dueCase{
		{name: "sunday, created midweek", s: sunday(), loc: ny, created: "2026-10-21T12:00:00-04:00", want: "2026-10-25T09:00:00-04:00"},
		{name: "sunday, created before due time that day", s: sunday(), loc: ny, created: "2026-10-25T08:00:00-04:00", want: "2026-10-25T09:00:00-04:00"},
		{name: "sunday, created after due time that day", s: sunday(), loc: ny, created: "2026-10-25T10:00:00-04:00", want: "2026-11-01T09:00:00-05:00"},
		{name: "sunday, done the day before satisfies that sunday", s: sunday(), loc: ny, last: "2026-10-24T18:00:00-04:00", want: "2026-11-01T09:00:00-05:00"},
		{name: "sunday, done on sunday", s: sunday(), loc: ny, last: "2026-10-25T20:00:00-04:00", want: "2026-11-01T09:00:00-05:00"},
		{name: "sunday, done tuesday satisfies the sunday just gone", s: sunday(), loc: ny, last: "2026-10-27T12:00:00-04:00", want: "2026-11-01T09:00:00-05:00"},
		{name: "sunday, done wednesday is still closer to the past", s: sunday(), loc: ny, last: "2026-10-28T12:00:00-04:00", want: "2026-11-01T09:00:00-05:00"},
		{name: "sunday, done thursday is closer to the next", s: sunday(), loc: ny, last: "2026-10-29T12:00:00-04:00", want: "2026-11-08T09:00:00-05:00"},
		{name: "mwf, created saturday", s: mwf(), loc: ny, created: "2026-10-24T12:00:00-04:00", want: "2026-10-26T09:00:00-04:00"},
		{name: "mwf, done tuesday ties to the earlier monday", s: mwf(), loc: ny, last: "2026-10-27T10:00:00-04:00", want: "2026-10-28T09:00:00-04:00"},
		{name: "mwf, done saturday noon is nearer friday", s: mwf(), loc: ny, last: "2026-10-24T12:00:00-04:00", want: "2026-10-26T09:00:00-04:00"},
		{name: "mwf, done thursday 09:00 ties to the earlier", s: mwf(), loc: ny, last: "2026-10-29T09:00:00-04:00", want: "2026-10-30T09:00:00-04:00"},
		{name: "every day, done before due time", s: weekly(t, EveryDay, 1, "", h9), loc: ny, last: "2026-10-24T08:00:00-04:00", want: "2026-10-25T09:00:00-04:00"},
		{name: "every 2nd saturday, created on the day before due time", s: fortnightSat(), loc: ny, created: "2026-10-24T08:00:00-04:00", want: "2026-10-24T09:00:00-04:00"},
		{name: "every 2nd saturday, created after due time", s: fortnightSat(), loc: ny, created: "2026-10-24T10:00:00-04:00", want: "2026-11-07T09:00:00-05:00"},
		{name: "every 2nd saturday, created before anchor", s: fortnightSat(), loc: ny, created: "2026-10-01T10:00:00-04:00", want: "2026-10-24T09:00:00-04:00"},
		{name: "every 2nd saturday, done on the anchor", s: fortnightSat(), loc: ny, last: "2026-10-24T09:00:00-04:00", want: "2026-11-07T09:00:00-05:00"},
		{name: "every 2nd saturday, done in the off week satisfies the next one", s: fortnightSat(), loc: ny, last: "2026-11-03T12:00:00-05:00", want: "2026-11-21T09:00:00-05:00"},
		{name: "every 2nd saturday, off-week saturday ties to the earlier", s: fortnightSat(), loc: ny, last: "2026-10-17T12:00:00-04:00", want: "2026-10-24T09:00:00-04:00"},
		{name: "mon and tue, done monday late evening makes tuesday due", s: weekly(t, Mon|Tue, 1, "", h9), loc: ny, last: "2026-10-26T22:00:00-04:00", want: "2026-10-27T09:00:00-04:00"},
		{name: "weekly, done three weeks before the anchor", s: fortnightSat(), loc: ny, last: "2026-10-03T12:00:00-04:00", want: "2026-10-24T09:00:00-04:00"},
		{name: "every 2nd saturday, done on a week before the anchor", s: fortnightSat(), loc: ny, last: "2026-10-10T12:00:00-04:00", want: "2026-10-24T09:00:00-04:00"},
		{name: "anchor on a different weekday, anchor week counts", s: weekly(t, Sun, 2, "2026-10-21", h9), loc: ny, created: "2026-10-21T08:00:00-04:00", want: "2026-10-25T09:00:00-04:00"},
		{name: "anchor on a different weekday, next on-week", s: weekly(t, Sun, 2, "2026-10-21", h9), loc: ny, last: "2026-10-25T10:00:00-04:00", want: "2026-11-08T09:00:00-05:00"},
		{name: "occurrence before the anchor day is skipped", s: weekly(t, Mon, 2, "2026-10-21", h9), loc: ny, created: "2026-10-20T08:00:00-04:00", want: "2026-11-02T09:00:00-05:00"},
		{name: "sunday anchor ends its week", s: weekly(t, Sun, 2, "2026-10-25", h9), loc: utc, created: "2026-10-25T00:00:00Z", want: "2026-10-25T09:00:00Z"},
		{name: "sunday anchor, created after", s: weekly(t, Sun, 2, "2026-10-25", h9), loc: utc, created: "2026-10-26T00:00:00Z", want: "2026-11-08T09:00:00Z"},
		{name: "every 3 weeks over february", s: weekly(t, Fri, 3, "2026-01-02", h9), loc: utc, created: "2026-01-03T00:00:00Z", want: "2026-01-23T09:00:00Z"},
		{name: "every 3 weeks, month end", s: weekly(t, Fri, 3, "2026-01-02", h9), loc: utc, last: "2026-02-13T09:00:00Z", want: "2026-03-06T09:00:00Z"},
		{name: "two days a fortnight, done between", s: weekly(t, Mon|Thu, 2, "2026-10-26", h9), loc: utc, last: "2026-10-30T12:00:00Z", want: "2026-11-09T09:00:00Z"},
		{name: "two days a fortnight, done in the off week", s: weekly(t, Mon|Thu, 2, "2026-10-26", h9), loc: utc, last: "2026-11-08T12:00:00Z", want: "2026-11-12T09:00:00Z"},
		{name: "every 52 weeks", s: weekly(t, Mon, 52, "2026-01-05", h9), loc: utc, created: "2026-01-06T00:00:00Z", want: "2027-01-04T09:00:00Z"},
		{name: "ny spring change shortens the day", s: sunday(), loc: ny, last: "2026-03-07T12:00:00-05:00", want: "2026-03-15T09:00:00-04:00"},
		{name: "ny due time in gap", s: weekly(t, Sun, 1, "", tod(2, 30)), loc: ny, created: "2026-03-02T00:00:00-05:00", want: "2026-03-08T03:30:00-04:00"},
		{name: "ny gap occurrence done, next week is normal", s: weekly(t, Sun, 1, "", tod(2, 30)), loc: ny, last: "2026-03-08T03:30:00-04:00", want: "2026-03-15T02:30:00-04:00"},
		{name: "ny due time in overlap takes first", s: weekly(t, Sun, 1, "", tod(1, 30)), loc: ny, created: "2026-10-26T00:00:00-04:00", want: "2026-11-01T01:30:00-04:00"},
		{name: "london gap", s: weekly(t, Sun, 1, "", tod(1, 30)), loc: lon, created: "2026-03-23T00:00:00Z", want: "2026-03-29T02:30:00+01:00"},
		{name: "london overlap", s: weekly(t, Sun, 1, "", tod(1, 30)), loc: lon, created: "2026-10-19T00:00:00+01:00", want: "2026-10-25T01:30:00+01:00"},
		{name: "london week across the change", s: weekly(t, Sat|Sun, 1, "", h9), loc: lon, last: "2026-10-24T12:00:00+01:00", want: "2026-10-25T09:00:00Z"},
		{name: "sydney gap", s: weekly(t, Sun, 1, "", tod(2, 30)), loc: syd, created: "2026-09-28T00:00:00+10:00", want: "2026-10-04T03:30:00+11:00"},
		{name: "sydney overlap", s: weekly(t, Sun, 1, "", tod(2, 30)), loc: syd, created: "2027-03-29T00:00:00+11:00", want: "2027-04-04T02:30:00+11:00"},
		{name: "sydney saturday across the change", s: weekly(t, Sat, 1, "", h9), loc: syd, last: "2026-10-03T20:00:00+10:00", want: "2026-10-10T09:00:00+11:00"},
	})
}

func TestNextDueFarFromEpoch(t *testing.T) {
	h9 := tod(9, 0)
	runDueCases(t, []dueCase{
		{name: "anchor 1900, never done", s: everyNDays(t, 7, "1900-01-01", h9), loc: utc, created: "2026-10-24T00:00:00Z", want: "2026-10-26T09:00:00Z"},
		{name: "anchor 1900, done", s: everyNDays(t, 7, "1900-01-01", h9), loc: utc, last: "2026-10-26T09:00:00Z", want: "2026-11-02T09:00:00Z"},
		{name: "fortnightly anchor 1900", s: weekly(t, Mon, 2, "1900-01-01", h9), loc: utc, created: "2026-10-24T00:00:00Z", want: "2026-11-02T09:00:00Z"},
		{name: "anchor 2200 is far in the future", s: everyNDays(t, 7, "2200-12-31", h9), loc: utc, created: "2026-10-24T00:00:00Z", want: "2200-12-31T09:00:00Z"},
	})
}

func TestNextDueMonthly(t *testing.T) {
	h9 := tod(9, 0)
	runDueCases(t, []dueCase{
		{name: "31st, created mid month", s: monthly(31, h9), loc: utc, created: "2026-01-15T00:00:00Z", want: "2026-01-31T09:00:00Z"},
		{name: "31st clamps to february", s: monthly(31, h9), loc: utc, created: "2026-01-31T10:00:00Z", want: "2026-02-28T09:00:00Z"},
		{name: "31st after february", s: monthly(31, h9), loc: utc, created: "2026-02-28T10:00:00Z", want: "2026-03-31T09:00:00Z"},
		{name: "31st clamps to leap february", s: monthly(31, h9), loc: utc, created: "2028-02-01T00:00:00Z", want: "2028-02-29T09:00:00Z"},
		{name: "31st clamps to april", s: monthly(31, h9), loc: utc, created: "2026-04-01T00:00:00Z", want: "2026-04-30T09:00:00Z"},
		{name: "30th clamps to february", s: monthly(30, h9), loc: utc, created: "2026-02-01T00:00:00Z", want: "2026-02-28T09:00:00Z"},
		{name: "29th in a common year", s: monthly(29, h9), loc: utc, created: "2027-02-01T00:00:00Z", want: "2027-02-28T09:00:00Z"},
		{name: "29th in a leap year", s: monthly(29, h9), loc: utc, created: "2028-02-01T00:00:00Z", want: "2028-02-29T09:00:00Z"},
		{name: "1st rolls into the next year", s: monthly(1, h9), loc: utc, created: "2026-12-15T00:00:00Z", want: "2027-01-01T09:00:00Z"},
		{name: "created exactly at the due time", s: monthly(15, h9), loc: utc, created: "2026-10-15T09:00:00Z", want: "2026-10-15T09:00:00Z"},
		{name: "created a minute after the due time", s: monthly(15, h9), loc: utc, created: "2026-10-15T09:01:00Z", want: "2026-11-15T09:00:00Z"},
		{name: "1st, done at month end is nearer the next", s: monthly(1, h9), loc: utc, last: "2026-10-30T12:00:00Z", want: "2026-12-01T09:00:00Z"},
		{name: "1st, done early in the month", s: monthly(1, h9), loc: utc, last: "2026-10-10T12:00:00Z", want: "2026-11-01T09:00:00Z"},
		{name: "1st, done just before midpoint", s: monthly(1, h9), loc: utc, last: "2026-10-16T09:00:00Z", want: "2026-11-01T09:00:00Z"},
		{name: "1st, done just after midpoint", s: monthly(1, h9), loc: utc, last: "2026-10-17T09:00:00Z", want: "2026-12-01T09:00:00Z"},
		{name: "1st, done on new year's eve", s: monthly(1, h9), loc: utc, last: "2026-12-30T12:00:00Z", want: "2027-02-01T09:00:00Z"},
		{name: "31st, done before the clamped february date", s: monthly(31, h9), loc: utc, last: "2026-02-27T12:00:00Z", want: "2026-03-31T09:00:00Z"},
		{name: "31st, done just after the clamped february date", s: monthly(31, h9), loc: utc, last: "2026-03-01T12:00:00Z", want: "2026-03-31T09:00:00Z"},
		{name: "31st, done on the day", s: monthly(31, h9), loc: utc, last: "2026-12-31T12:00:00Z", want: "2027-01-31T09:00:00Z"},
		{name: "29th, done on leap day", s: monthly(29, h9), loc: utc, last: "2028-02-29T12:00:00Z", want: "2028-03-29T09:00:00Z"},
		{name: "ny across autumn change", s: monthly(15, h9), loc: ny, last: "2026-10-15T12:00:00-04:00", want: "2026-11-15T09:00:00-05:00"},
		{name: "ny due time in gap", s: monthly(8, tod(2, 30)), loc: ny, created: "2026-03-01T00:00:00-05:00", want: "2026-03-08T03:30:00-04:00"},
		{name: "london due time in overlap", s: monthly(25, tod(1, 30)), loc: lon, created: "2026-10-01T00:00:00+01:00", want: "2026-10-25T01:30:00+01:00"},
		{name: "sydney local date differs from utc date", s: monthly(1, tod(0, 30)), loc: syd, created: "2026-10-31T12:00:00Z", want: "2026-11-01T00:30:00+11:00"},
	})
}

func TestNextDueNoDueTime(t *testing.T) {
	zero := Schedule{}
	runDueCases(t, []dueCase{
		{name: "none", s: Schedule{Kind: None, IntervalWeeks: 1, DueTime: tod(9, 0)}, loc: utc, last: "2026-01-01T00:00:00Z"},
		{name: "none never done", s: Schedule{Kind: None, IntervalWeeks: 1, DueTime: tod(9, 0)}, loc: utc},
		{name: "zero value", s: zero, loc: utc},
		{name: "invalid interval", s: Schedule{Kind: AfterLast, IntervalDays: 0, DueTime: tod(9, 0)}, loc: utc, last: "2026-01-01T00:00:00Z"},
		{name: "invalid mask", s: Schedule{Kind: Weekly, Weekdays: 0, IntervalWeeks: 1}, loc: utc},
		{name: "monthly day 40", s: monthly(40, tod(9, 0)), loc: utc},
	})
}

func TestNextDueNilLocationIsUTC(t *testing.T) {
	got, ok := NextDue(afterLast(1, tod(9, 0)), nil, ts(t, "2026-10-24T15:00:00Z"), nil)
	if !ok || !got.Equal(ts(t, "2026-10-24T15:00:00Z")) {
		t.Errorf("never done: %v %v", got, ok)
	}
	last := ts(t, "2026-10-24T23:30:00-04:00")
	got, ok = NextDue(afterLast(1, tod(9, 0)), &last, time.Time{}, nil)
	if !ok || !got.Equal(ts(t, "2026-10-26T09:00:00Z")) {
		t.Errorf("done: %v %v", got, ok)
	}
}

func TestNextDueDoesNotMutateInputs(t *testing.T) {
	s := weekly(t, Sat, 2, "2026-10-24", tod(9, 0))
	before := s
	last := ts(t, "2026-11-03T12:00:00-05:00")
	lastCopy := last
	NextDue(s, &last, time.Time{}, loadLoc(t, ny))
	if s != before || !last.Equal(lastCopy) {
		t.Error("NextDue changed its arguments")
	}
}

func TestOccurrencesOn(t *testing.T) {
	h9 := tod(9, 0)
	nyLoc, lonLoc, sydLoc := loadLoc(t, ny), loadLoc(t, lon), loadLoc(t, syd)
	tests := []struct {
		name string
		s    Schedule
		loc  *time.Location
		day  string
		want []string
	}{
		{"weekly matching day", weekly(t, Mon|Wed|Fri, 1, "", h9), nyLoc, "2026-10-26", []string{"2026-10-26T09:00:00-04:00"}},
		{"weekly other day", weekly(t, Mon|Wed|Fri, 1, "", h9), nyLoc, "2026-10-27", nil},
		{"weekly every day", weekly(t, EveryDay, 1, "", h9), nyLoc, "2026-10-27", []string{"2026-10-27T09:00:00-04:00"}},
		{"fortnight on week", weekly(t, Sat, 2, "2026-10-24", h9), nyLoc, "2026-11-07", []string{"2026-11-07T09:00:00-05:00"}},
		{"fortnight off week", weekly(t, Sat, 2, "2026-10-24", h9), nyLoc, "2026-10-31", nil},
		{"fortnight before anchor", weekly(t, Sat, 2, "2026-10-24", h9), nyLoc, "2026-10-10", nil},
		{"fortnight anchor day", weekly(t, Sat, 2, "2026-10-24", h9), nyLoc, "2026-10-24", []string{"2026-10-24T09:00:00-04:00"}},
		{"every n days on grid", everyNDays(t, 3, "2026-11-01", h9), nyLoc, "2026-11-07", []string{"2026-11-07T09:00:00-05:00"}},
		{"every n days off grid", everyNDays(t, 3, "2026-11-01", h9), nyLoc, "2026-11-08", nil},
		{"every n days before anchor", everyNDays(t, 3, "2026-11-01", h9), nyLoc, "2026-10-29", nil},
		{"every n days anchor", everyNDays(t, 3, "2026-11-01", h9), nyLoc, "2026-11-01", []string{"2026-11-01T09:00:00-05:00"}},
		{"every n days across leap day", everyNDays(t, 14, "2028-02-01", h9), time.UTC, "2028-02-29", []string{"2028-02-29T09:00:00Z"}},
		{"monthly exact", monthly(15, h9), time.UTC, "2026-10-15", []string{"2026-10-15T09:00:00Z"}},
		{"monthly other day", monthly(15, h9), time.UTC, "2026-10-16", nil},
		{"monthly clamped", monthly(31, h9), time.UTC, "2026-02-28", []string{"2026-02-28T09:00:00Z"}},
		{"monthly clamped, not on the 27th", monthly(31, h9), time.UTC, "2026-02-27", nil},
		{"monthly clamped leap", monthly(31, h9), time.UTC, "2028-02-29", []string{"2028-02-29T09:00:00Z"}},
		{"monthly clamped leap, not the 28th", monthly(31, h9), time.UTC, "2028-02-28", nil},
		{"monthly 30 day month", monthly(31, h9), time.UTC, "2026-04-30", []string{"2026-04-30T09:00:00Z"}},
		{"after_last has none", afterLast(7, h9), time.UTC, "2026-10-24", nil},
		{"none has none", Schedule{Kind: None, IntervalWeeks: 1, DueTime: h9}, time.UTC, "2026-10-24", nil},
		{"invalid has none", Schedule{Kind: Weekly}, time.UTC, "2026-10-24", nil},
		{"nil location is UTC", weekly(t, EveryDay, 1, "", h9), nil, "2026-10-24", []string{"2026-10-24T09:00:00Z"}},
		{"ny gap day has one", weekly(t, Sun, 1, "", tod(2, 30)), nyLoc, "2026-03-08", []string{"2026-03-08T03:30:00-04:00"}},
		{"ny overlap day has one", weekly(t, Sun, 1, "", tod(1, 30)), nyLoc, "2026-11-01", []string{"2026-11-01T01:30:00-04:00"}},
		{"london gap day", weekly(t, Sun, 1, "", tod(1, 30)), lonLoc, "2026-03-29", []string{"2026-03-29T02:30:00+01:00"}},
		{"london overlap day", weekly(t, Sun, 1, "", tod(1, 30)), lonLoc, "2026-10-25", []string{"2026-10-25T01:30:00+01:00"}},
		{"sydney gap day", weekly(t, Sun, 1, "", tod(2, 30)), sydLoc, "2026-10-04", []string{"2026-10-04T03:30:00+11:00"}},
		{"sydney overlap day", weekly(t, Sun, 1, "", tod(2, 30)), sydLoc, "2027-04-04", []string{"2027-04-04T02:30:00+11:00"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := OccurrencesOn(tc.s, date(t, tc.day), tc.loc)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i, w := range tc.want {
				if !got[i].Equal(ts(t, w)) {
					t.Errorf("[%d] = %s, want %s", i, got[i].Format(time.RFC3339), w)
				}
			}
		})
	}
}

// refOccurs answers "is d an occurrence" straight from the definition in each schedule kind.
func refOccurs(s Schedule, d Date) bool {
	switch s.Kind {
	case EveryNDays:
		diff := d.DaysSince(s.AnchorDate)
		return (diff%s.IntervalDays+s.IntervalDays)%s.IntervalDays == 0
	case Weekly:
		if s.Weekdays&weekdayBit(d.Weekday()) == 0 {
			return false
		}
		weekStart := s.AnchorDate.AddDays(-((int(s.AnchorDate.Weekday()) + 6) % 7))
		weeks := floorDiv(d.DaysSince(weekStart), 7)
		return (weeks%s.IntervalWeeks+s.IntervalWeeks)%s.IntervalWeeks == 0
	default:
		last := Date{d.Year, d.Month + 1, 1}.AddDays(-1)
		return d.Day == min(s.MonthDay, last.Day)
	}
}

// refNextDue walks day by day through a window around the completion to find the occurrence nearest its local date.
func refNextDue(s Schedule, last time.Time, loc *time.Location) time.Time {
	day := DateOf(last, loc)
	var occ []Date
	for i := -500; i <= 500; i++ {
		if d := day.AddDays(i); refOccurs(s, d) {
			occ = append(occ, d)
		}
	}
	// Index of the last occurrence on or before the completion day.
	idx := -1
	for i, o := range occ {
		if !o.Before(day) && o != day {
			break
		}
		idx = i
	}
	prev, next := occ[idx], occ[idx+1]
	if day == prev || day.DaysSince(prev) <= next.DaysSince(day) {
		return occ[idx+1].At(s.DueTime, loc)
	}
	return occ[idx+2].At(s.DueTime, loc)
}

func refFirst(s Schedule, created time.Time, loc *time.Location) time.Time {
	day := DateOf(created, loc)
	for i := 0; ; i++ {
		d := day.AddDays(i)
		if !s.AnchorDate.IsZero() && (s.Kind == EveryNDays || s.IntervalWeeks > 1) && d.Before(s.AnchorDate) {
			continue
		}
		if refOccurs(s, d) {
			if at := d.At(s.DueTime, loc); !at.Before(created) {
				return at
			}
		}
	}
}

// TestNextDueAgainstReference compares NextDue with a brute-force day walk on random schedules in three zones.
func TestNextDueAgainstReference(t *testing.T) {
	rng := rand.New(rand.NewSource(20261024))
	zones := []*time.Location{loadLoc(t, ny), loadLoc(t, lon), loadLoc(t, syd)}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 6000; i++ {
		loc := zones[i%len(zones)]
		at := TimeOfDay{rng.Intn(24), []int{0, 15, 30, 45}[rng.Intn(4)]}
		anchor := DateOf(base, time.UTC).AddDays(rng.Intn(60))
		var s Schedule
		switch rng.Intn(3) {
		case 0:
			s = Schedule{Kind: EveryNDays, IntervalDays: 1 + rng.Intn(45), IntervalWeeks: 1, AnchorDate: anchor, DueTime: at}
		case 1:
			s = Schedule{Kind: Weekly, Weekdays: 1 + rng.Intn(127), IntervalWeeks: 1 + rng.Intn(5), AnchorDate: anchor, DueTime: at}
			if s.IntervalWeeks == 1 {
				s.AnchorDate = Date{}
			}
		default:
			s = Schedule{Kind: Monthly, MonthDay: 1 + rng.Intn(31), IntervalWeeks: 1, DueTime: at}
		}
		if err := s.Validate(); err != nil {
			t.Fatalf("generated invalid schedule %+v: %v", s, err)
		}
		// Completions after the anchor keep the reference window free of the grid's start.
		last := anchor.AddDays(60+rng.Intn(500)).At(TimeOfDay{rng.Intn(24), rng.Intn(60)}, loc).Add(time.Duration(rng.Intn(3600)) * time.Second)
		got, ok := NextDue(s, &last, time.Time{}, loc)
		if !ok {
			t.Fatalf("%+v: no due time", s)
		}
		if want := refNextDue(s, last, loc); !got.Equal(want) {
			t.Fatalf("%+v loc=%s last=%s\n got  %s\n want %s", s, loc, last.In(loc).Format(time.RFC3339), got.In(loc).Format(time.RFC3339), want.In(loc).Format(time.RFC3339))
		}
		if !got.After(last) {
			t.Fatalf("%+v: due %s not after last %s", s, got, last)
		}

		created := base.Add(time.Duration(rng.Intn(200*24*3600)) * time.Second)
		got, _ = NextDue(s, nil, created, loc)
		if want := refFirst(s, created, loc); !got.Equal(want) {
			t.Fatalf("%+v loc=%s created=%s\n got  %s\n want %s", s, loc, created.In(loc).Format(time.RFC3339), got.In(loc).Format(time.RFC3339), want.In(loc).Format(time.RFC3339))
		}
		if occ := OccurrencesOn(s, DateOf(got, loc), loc); len(occ) != 1 || !occ[0].Equal(got) {
			t.Fatalf("%+v: first due %s is not reported by OccurrencesOn: %v", s, got, occ)
		}
	}
}
