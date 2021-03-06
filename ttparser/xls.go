package ttparser

import (
	"github.com/extrame/xls"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
	"log"
)

var dateRegex = regexp.MustCompile(`\d\d\.\d\d.\d\d\d\d`)
var timeslotRegex = regexp.MustCompile(`(\d+) пара: \d\d:\d\d-\d\d:\d\d`)
var entryRegexp = regexp.MustCompile(`(.+)\[(Лк|Пз|Лб|Зач|Экз|Сем)\] .+\nауд\. (.+)\n(.+)`)

type RawEntry struct {
	Sequence  int
	Name      string
	Type      string
	Classroom string
	Lecturer  string
}

func OpenXLS(in io.ReadSeeker) (*xls.WorkBook, error) {
	return xls.OpenReader(in, "utf-8")
}

// Parse XLS sheet with timetable from DUT.
// Note: It will parse correctly only one week of input. Second week and etc will be ignored.
func ReadEntries(book *xls.WorkBook) (map[time.Time][]RawEntry, error) {
	res := make(map[time.Time][]RawEntry)
	sheet := book.GetSheet(0)
	entries := []RawEntry(nil)
	curDate := time.Time{}
	err := error(nil)
	for i := 0; i <= int(sheet.MaxRow); i++ {
		if dateRegex.MatchString(sheet.Row(i).Col(1)) {
			if !curDate.IsZero() && len(entries) != 0 {
				res[curDate] = entries
				curDate = time.Time{}
				entries = []RawEntry(nil)
			}

			curDate, err = time.Parse("02.01.2006", sheet.Row(i).Col(1))
			if err != nil {
				continue
			}
		}

		if !curDate.IsZero() {
			timeslotMatch := timeslotRegex.FindStringSubmatch(sheet.Row(i).Col(0))
			if timeslotMatch == nil {
				continue
			}
			n, _ := strconv.Atoi(timeslotMatch[1])

			entryMatch := entryRegexp.FindStringSubmatch(sheet.Row(i).Col(1))
			if entryMatch == nil {
				continue
			}

			entries = append(entries, RawEntry{
				n, entryMatch[1],
				strings.ToLower(entryMatch[2]), entryMatch[3],
				entryMatch[4],
			})
		}
	}
	if !curDate.IsZero() && len(entries) != 0 {
		res[curDate] = entries
	}

	log.Printf("Raw entries: %+v", res)

	return res, nil
}
