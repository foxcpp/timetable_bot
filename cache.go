package main

import (
	"log"
	"strings"
	"sync"
	"time"

	"github.com/foxcpp/timetable_bot/ttparser"
	"github.com/pkg/errors"
)

const maxCacheAge = time.Hour

type LessonType int

const (
	Lab      LessonType = 0
	Practice            = 1
	Lecture             = 2
)

type Entry struct {
	Time      time.Time
	Type      LessonType
	Classroom string
	Lecturer  string
	Name      string
}

type cachedEntries struct {
	entries     []Entry
	retrievedOn time.Time
}

type Cache struct {
	cacheLck sync.RWMutex
	cache    map[time.Time]cachedEntries

	cleanUpTicker *time.Ticker
	tickerStop    chan bool
}

func NewCache() *Cache {
	c := new(Cache)

	c.cache = make(map[time.Time]cachedEntries)
	c.cleanUpTicker = time.NewTicker(15 * time.Minute)
	c.tickerStop = make(chan bool)
	go c.cleanUpTick()
	return c
}

func (c *Cache) Close() error {
	c.tickerStop <- true
	<-c.tickerStop
	c.cleanUpTicker.Stop()
	return nil
}

func (c *Cache) ExactGet(t time.Time) (*Entry, error) {
	day, err := c.OnDay(StripTime(t, t.Location()))
	if err != nil {
		return nil, err
	}

	for _, ent := range day {
		if ent.Time.Truncate(time.Minute) == t.Truncate(time.Minute) {
			return &ent, nil
		}
	}
	return nil, nil
}

func (c *Cache) cleanUpTick() {
	for {
		select {
		case <-c.cleanUpTicker.C:
			c.cleanUp()
		case <-c.tickerStop:
			c.tickerStop <- true
			return
		}
	}
}

func (c *Cache) OnDay(day time.Time) ([]Entry, error) {
	day = StripTime(day, day.Location())

	c.cacheLck.RLock()
	entries, prs := c.cache[day]
	c.cacheLck.RUnlock()

	if !prs || entries.retrievedOn.Add(maxCacheAge).Before(time.Now()) {
		if err := c.downloadWeek(day); err != nil {
			return nil, err
		}
		return c.cache[day].entries, nil
	}

	return entries.entries, nil
}

func (c *Cache) downloadWeek(day time.Time) error {
	fromDay := day
	toDay := day
	for fromDay.Weekday() != time.Monday {
		fromDay = fromDay.Add(-24 * time.Hour)
	}
	for toDay.Weekday() != time.Sunday {
		toDay = toDay.Add(24 * time.Hour)
	}

	log.Printf("Downloading table for %s-%s...\n", fromDay.Format("02.01.2006"), toDay.Format("02.01.2006"))
	rawTable, err := ttparser.Download(fromDay, toDay, config.SourceCfg)
	if err != nil {
		return errors.Wrap(err, "table download")
	}

	c.cacheLck.Lock()
	defer c.cacheLck.Unlock()
	for fromDay.Before(toDay.Add(24 * time.Hour)) {
		c.cache[fromDay] = cachedEntries{
			entries:     FromRaw(fromDay, rawTable[StripTime(fromDay, time.UTC)]),
			retrievedOn: time.Now(),
		}
		fromDay = fromDay.Add(24 * time.Hour)
	}
	return nil
}

func (c *Cache) cleanUp() {
	c.cacheLck.Lock()
	defer c.cacheLck.Unlock()

	totalRemoved := 0
	for k, ent := range c.cache {
		if ent.retrievedOn.Add(maxCacheAge).Before(time.Now()) {
			totalRemoved += 1
			delete(c.cache, k)
		}
	}

	for len(c.cache) > 100 {
		oldestStamp := time.Now()
		oldestDay := time.Time{}
		for k, ent := range c.cache {
			if ent.retrievedOn.Before(oldestStamp) {
				oldestStamp = ent.retrievedOn
				oldestDay = k
			}
		}
		delete(c.cache, oldestDay)
		totalRemoved += 1
	}
	if totalRemoved != 0 {
		log.Printf("Removed %d stalled entires from cache.\n", totalRemoved)
	}
}

func (c *Cache) Evict(date time.Time) {
	day := StripTime(date, date.Location())
	c.cacheLck.Lock()
	defer c.cacheLck.Unlock()

	delete(c.cache, day)
}

func FromRaw(date time.Time, e []ttparser.RawEntry) []Entry {
	res := make([]Entry, len(e))
	for i, ent := range e {
		res[i] = Entry{
			TimeSlotSet(date, config.TimeslotsBegin[ent.Sequence-1]),
			lang.LessonTypeStrs[strings.ToLower(ent.Type)],
			ent.Classroom,
			ent.Lecturer,
			ent.Name,
		}
	}
	return res
}

func StripTime(t time.Time, tz *time.Location) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, tz)
}

func TimeSlotSet(t time.Time, slot TimeSlot) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), slot.Hour, slot.Minute, 0, 0, t.Location())
}
