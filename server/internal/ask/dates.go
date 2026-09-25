package ask

import (
	"strconv"
	"time"
)

var months = map[string]time.Month{
	"january": 1, "februari": 2, "february": 2, "march": 3, "maret": 3, "april": 4, "may": 5, "mei": 5,
	"june": 6, "juni": 6, "july": 7, "juli": 7, "august": 8, "agustus": 8, "september": 9,
	"october": 10, "oktober": 10, "november": 11, "december": 12, "desember": 12, "januari": 1,
}

// dates reads the §11.2 date phrases in both languages: "since January" /
// "sejak Januari", "in 2025" / "tahun 2025", "last month" / "bulan lalu",
// "this year" / "tahun ini", "Q1 2026", ISO dates and "between March and May".
// A month without a year means its latest past occurrence. To is inclusive.
func dates(toks []string, now time.Time) (from, to *time.Time) {
	for i := 0; i < len(toks); i++ {
		switch toks[i] {
		case "since", "sejak":
			if f, _, _, ok := period(toks, i+1, now, true); ok {
				return &f, nil
			}
		case "between", "antara":
			f, _, n, ok := period(toks, i+1, now, true)
			if !ok || i+1+n >= len(toks) || (toks[i+1+n] != "and" && toks[i+1+n] != "dan") {
				continue
			}
			if _, t, _, ok := period(toks, i+2+n, now, true); ok {
				return &f, &t
			}
		}
	}
	for i := range toks {
		cued := i > 0 && (toks[i-1] == "in" || toks[i-1] == "pada" || toks[i-1] == "during" || toks[i-1] == "selama")
		if f, t, _, ok := period(toks, i, now, cued); ok {
			return &f, &t
		}
	}
	return nil, nil
}

// period reads one period at toks[i] and returns its first and last day and
// the tokens it used. Without a cue, "may" needs a year, as in "may 2026", so
// the English verb is not taken for the month.
func period(toks []string, i int, now time.Time, cued bool) (from, to time.Time, used int, ok bool) {
	if i >= len(toks) {
		return
	}
	loc := now.Location()
	day := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, loc) }
	next := ""
	if i+1 < len(toks) {
		next = toks[i+1]
	}
	year, yearErr := strconv.Atoi(next)
	hasYear := yearErr == nil && year >= 2000 && year <= 2099
	switch t := toks[i]; {
	case (t == "last" && next == "month") || (t == "bulan" && next == "lalu"):
		first := day(now.Year(), now.Month(), 1).AddDate(0, -1, 0)
		return first, first.AddDate(0, 1, -1), 2, true
	case (t == "this" && next == "year") || (t == "tahun" && next == "ini"):
		return day(now.Year(), 1, 1), day(now.Year(), now.Month(), now.Day()), 2, true
	case (t == "last" && next == "year") || (t == "tahun" && next == "lalu"):
		return day(now.Year()-1, 1, 1), day(now.Year()-1, 12, 31), 2, true
	case t == "tahun" && hasYear:
		return day(year, 1, 1), day(year, 12, 31), 2, true
	case len(t) == 2 && t[0] == 'q' && t[1] >= '1' && t[1] <= '4' && hasYear:
		first := day(year, time.Month(3*int(t[1]-'1')+1), 1)
		return first, first.AddDate(0, 3, -1), 2, true
	case len(t) == 10 && t[4] == '-':
		d, err := time.ParseInLocation(time.DateOnly, t, loc)
		return d, d, 1, err == nil
	case len(t) == 4:
		if y, err := strconv.Atoi(t); err == nil && y >= 2000 && y <= 2099 && cued {
			return day(y, 1, 1), day(y, 12, 31), 1, true
		}
	}
	m, isMonth := months[toks[i]]
	if !isMonth || (toks[i] == "may" && !cued && !hasYear) {
		return
	}
	used = 1
	y := now.Year()
	if hasYear {
		y, used = year, 2
	} else if m > now.Month() {
		y-- // the month's latest past occurrence
	}
	first := day(y, m, 1)
	return first, first.AddDate(0, 1, -1), used, true
}
