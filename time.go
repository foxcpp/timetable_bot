package main

import "time"

var timezone *time.Location

func init() {
	var err error
	timezone, err = time.LoadLocation("Europe/Kiev")
	if err != nil {
		panic(err)
	}
}