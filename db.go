package main

import (
	"database/sql"
	"github.com/pkg/errors"
	"time"
)
import _ "github.com/mattn/go-sqlite3"

type LessonType int
const (
	Lab LessonType = iota
	Practice
	Lecture
)

type Entry struct {
	Time time.Time
	Type LessonType
	Classroom string
	Lecturer string
	Name string
}

type DB struct {
	d *sql.DB

	addEntry *sql.Stmt
	clearDay *sql.Stmt
	batchFillable *sql.Stmt
	onDay *sql.Stmt
	exactGet *sql.Stmt
}

func NewDB(path string) (*DB, error) {
	db := new(DB)
	var err error
	db.d, err = sql.Open("sqlite3", path + "?_journal=WAL&cache=shared")
	if err != nil {
		return nil, errors.Wrap(err, "open")
	}

	if _, err := db.d.Exec(`PRAGMA journal_mode = WAL`); err != nil {
		return nil, errors.Wrap(err, "set pragma journal mode")
	}
	if _, err := db.d.Exec(`PRAGMA synchronous = NORMAL`); err != nil {
		return nil, errors.Wrap(err, "set pragma synchronous")
	}
	if _, err := db.d.Exec(`
            CREATE TABLE IF NOT EXISTS events (
                year INT NOT NULL
                CHECK(year >= 2018),
                month INT NOT NULL
                CHECK(month >= 1 AND month <= 12),
                day INT NOT NULL
                CHECK(day >= 1 AND day <= 31),
                hour INT NOT NULL
                CHECK(hour >= 0 AND hour <= 23),
                minute INT NOT NULL
                CHECK(minute >= 0 AND minute <= 59),

                type INT NOT NULL,
                classroom TEXT NOT NULL,
                lecturer TEXT NOT NULL,
                name TEXT NOT NULL,

                batchfilled INT NOT NULL DEFAULT 0,

                UNIQUE (year, month, day, hour, minute)
            )
	`); err != nil {
		return nil, errors.Wrap(err, "create table")
	}

	db.addEntry, err = db.d.Prepare(`
		INSERT INTO events VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	db.clearDay, err = db.d.Prepare(`
		DELETE FROM events 
		WHERE year = ? AND month = ? AND day = ?`)
	if err != nil {
		return nil, errors.Wrap(err, "prepare clearDay")
	}
	db.batchFillable, err = db.d.Prepare(`
		SELECT batchfilled
		FROM events
		WHERE year = ? AND month = ? AND day = ?
		LIMIT 1`)
	if err != nil {
		return nil, errors.Wrap(err, "prepare batchFillable")
	}
	db.onDay, err = db.d.Prepare(`
		SELECT hour, minute, type, classroom, lecturer, name
		FROM events
		WHERE year = ? AND month = ? AND day = ?`)
	if err != nil {
		return nil, errors.Wrap(err, "prepare onDay")
	}
	db.exactGet, err = db.d.Prepare(`
		SELECT type, classroom, lecturer, name
		FROM events
		WHERE year = ? AND month = ? AND day = ? AND hour = ? AND minute = ? 
	`)
	if err != nil {
		return nil, errors.Wrap(err, "prepare exactGet")
	}
	return db, nil
}

func (db *DB) Close() error {
	db.clearDay.Close()
	db.batchFillable.Close()
	db.onDay.Close()
	db.exactGet.Close()
	return db.d.Close()
}

func (db *DB) ReplaceDay(day time.Time, entries []Entry, batch bool) error {
	tx, err := db.d.Begin()
	if err != nil {
		return errors.Wrap(err, "tx begin")
	}
	defer tx.Rollback()

	if _, err := db.clearDay.Exec(day.Year(), day.Month(), day.Day()); err != nil {
		return errors.Wrapf(err, "clearDay %v", day)
	}
	for _, entry := range entries {
		if _, err := db.addEntry.Exec(
			day.Year(), day.Month(), day.Day(),
			entry.Time.Hour(), entry.Time.Minute(),
			entry.Type, entry.Classroom, entry.Lecturer,
			entry.Name, batch); err != nil {
			return errors.Wrapf(err, "addEntry %v", entry)
		}
	}
	if err := tx.Commit(); err != nil {
		return errors.Wrap(err, "tx commit")
	}
	return nil
}

func (db *DB) BatchFillable(day time.Time) (bool, error) {
	row := db.batchFillable.QueryRow(day.Year(), day.Month(), day.Day())
	res := true
	return res, row.Scan(&res)
}

func (db *DB) ClearDay(day time.Time) error {
	_, err := db.clearDay.Exec(day.Year(), day.Month(), day.Day())
	return err
}

func (db *DB) ExactGet(t time.Time) (*Entry, error) {
	row := db.exactGet.QueryRow(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute())
	res := Entry{}
	res.Time = t
	return &res, row.Scan(&res.Type, &res.Classroom, &res.Lecturer, &res.Name)
}

func (db *DB) OnDay(day time.Time) ([]Entry, error) {
	rows, err := db.onDay.Query(day.Year(), day.Month(), day.Day())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := []Entry(nil)
	for rows.Next() {
		entry := Entry{}
		hour, minute := 0, 0
		if err := rows.Scan(&hour, &minute, &entry.Type, &entry.Classroom, &entry.Lecturer, &entry.Name); err != nil {
			panic(err)
		}
		entry.Time = TimeSlotSet(day, TimeSlot{hour, minute})
		res = append(res, entry)
	}
	return res, nil
}