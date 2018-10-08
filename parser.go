package main

import (
	"github.com/pkg/errors"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var types = map[string]LessonType{
	"лб":           Lab,
	"лабараторная": Lab,
	"пз":           Practice,
	"практическое": Practice,
	"лк":           Lecture,
	"лекция":       Lecture,
}

func TimeSlotSet(t time.Time, slot TimeSlot) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), slot.Hour, slot.Minute, 0, 0, t.Location())
}

var (
	ErrTooManyEntires = errors.New("too many entries")
	ErrInvalidFormat  = errors.New("invalid entry format")
	ErrUnknownType    = errors.New("unknown lesson type")

	entryRegex = regexp.MustCompile(`(\d+)\. ([^ ]+) "([^"]+)" ([^ ]+) "([^"]+)"`)
)

func SplitEntry(in string, day time.Time) (Entry, error) {
	res := Entry{}
	submatches := entryRegex.FindStringSubmatch(in)
	if len(submatches) == 0 {
		return Entry{}, ErrInvalidFormat
	}
	n, err := strconv.Atoi(submatches[1])
	if err != nil {
		return Entry{}, ErrInvalidFormat
	}

	if n > len(config.TimeslotsBegin) {
		return Entry{}, ErrTooManyEntires
	}
	res.Time = TimeSlotSet(day, config.TimeslotsBegin[n-1])

	var prs bool
	res.Type, prs = types[strings.ToLower(submatches[4])]
	if !prs {
		log.Println()
		return Entry{}, ErrUnknownType
	}

	res.Classroom = submatches[2]
	res.Lecturer = submatches[5]
	res.Name = submatches[3]
	return res, nil
}
