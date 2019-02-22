package ttparser

import (
	"bytes"
	"github.com/pkg/errors"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

var tableUrl = `http://e-rozklad.dut.edu.ua/timeTable/groupExcel?type=0`

type Cfg struct {
	Course  int `yaml:"course"`
	Faculty int `yaml:"faculty"`
	Group   int `yaml:"group"`
}

func Download(from, to time.Time, cfg Cfg) (map[time.Time][]RawEntry, error) {
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

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "body read")
	}

	xls, err := OpenXLS(bytes.NewReader(body))
	if err != nil {
		return nil, errors.Wrap(err, "xls parse")
	}

	res, err := ReadEntries(xls)
	if err != nil {
		return res, errors.Wrap(err, "table parse")
	}
	return res, nil
}
