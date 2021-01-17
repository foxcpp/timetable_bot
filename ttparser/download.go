package ttparser

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/pkg/errors"
)

var tableUrl = `https://ies.unitech-mo.ru/schedule_list_groups`

type Cfg struct {
	Group int `yaml:"group"`
}

type RawEntry struct {
	Sequence  int
	Name      string
	Type      string
	Classroom string
	Lecturer  string
	Notes     string
}

func Download(from, to time.Time, cfg Cfg) (map[time.Time][]RawEntry, error) {
	path := fmt.Sprintf("%s?d=%s&g=%d", tableUrl, from.Format("02.01.2006")+"+-+"+to.Format("02.01.2006"), cfg.Group)
	resp, err := http.Get(path)
	if err != nil {
		return nil, errors.Wrap(err, "table get")
	}
	if resp.StatusCode != 200 {
		return nil, errors.New("HTTP status " + resp.Status)
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "new document")
	}

	from = time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)

	return readTable(from, doc)
}

func readTable(firstDay time.Time, doc *goquery.Document) (map[time.Time][]RawEntry, error) {
	result := make(map[time.Time][]RawEntry)

	doc.Find(".adopt_area_scrollable > table > tbody").Each(func(indx int, tbody *goquery.Selection) {
		day := firstDay.Add(24 * time.Hour * time.Duration(indx))
		dayEnts := make([]RawEntry, 0, 8)

		tbody.Find("tr").Each(func(lectureIndx int, tr *goquery.Selection) {
			ent := RawEntry{}
			ent.Sequence = lectureIndx + 1
			skipLine := false
			tr.Find("td").Each(func(colIndx int, td *goquery.Selection) {
				switch colIndx {
				case 2:
					text := strings.TrimSpace(td.Text())
					if text == "" {
						skipLine = true
						return
					}
					dashIndx := strings.LastIndex(text, " - ")
					ent.Type = strings.ReplaceAll(strings.TrimSpace(text[dashIndx+3:]), "(Д)", "")
					ent.Name = strings.TrimSpace(text[:dashIndx])
				case 3:
					ent.Classroom = strings.TrimSpace(td.Text())
				case 4:
					ent.Lecturer = strings.TrimSpace(td.Text())
				case 5:
					ent.Notes = strings.TrimSpace(td.Text())
				}
			})
			if skipLine {
				return
			}
			dayEnts = append(dayEnts, ent)
		})

		result[day] = dayEnts
	})

	return result, nil
}
