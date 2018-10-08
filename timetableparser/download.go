package ttparser

import (
	"bytes"
	"github.com/pkg/errors"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

var tableUrl = `http://e-rozklad.dut.edu.ua/timeTable/groupExcel?type=0`

type AutoUpdateCfg struct {
	Course  int `yaml:"course"`
	Faculty int `yaml:"faculty"`
	Group   int `yaml:"group"`
}

func DownloadTable(from, to time.Time, cfg AutoUpdateCfg) (map[time.Time][]RawEntry, error) {
	form := url.Values{
		"timeTable":              {"0"},
		"TimeTableForm[course]":  {strconv.Itoa(cfg.Course)},
		"TimeTableForm[date1]":   {from.Format("02.01.2006")},
		"TimeTableForm[date2]":   {to.Format("02.01.2006")},
		"TimeTableForm[group]":   {strconv.Itoa(cfg.Group)},
		"TimeTableForm[faculty]": {strconv.Itoa(cfg.Faculty)},
		"TimeTableForm[r11]":     {"5"},
	}
	resp, err := http.PostForm(tableUrl, form)
	if err != nil {
		return nil, errors.Wrap(err, "table get")
	}

	if resp.StatusCode != 200 {
		return nil, errors.New("HTTP status " + resp.Status)
	}
	if resp.ContentLength < 0 {
		return nil, errors.New("no data")
	}

	buf := bytes.NewBuffer(make([]byte, 0, resp.ContentLength))
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, errors.Wrap(err, "table read")
	}

	xls, err := OpenXLS(bytes.NewReader(buf.Bytes()))
	if err != nil {
		return nil, errors.Wrap(err, "xls parse")
	}

	res, err := ReadEntries(xls)
	if err != nil {
		return res, errors.Wrap(err, "table parse")
	}
	return res, nil
}
