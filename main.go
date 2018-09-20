package main

import (
	"fmt"
	"github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/jasonlvhit/gocron"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
	"hexawolf.me/git/foxcpp/timetable_bot/timetableparser"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

var bot *tgbotapi.BotAPI
var db *DB
var config Config

type TimeSlot struct {
	Hour, Minute int
}

var timetableBegin = []TimeSlot{
	TimeSlot{8, 00},
	TimeSlot{9, 45},
	TimeSlot{11, 45},
	TimeSlot{13, 30},
	TimeSlot{15, 15},
}
var timetableEnd = []TimeSlot{
	TimeSlot{9, 35},
	TimeSlot{11, 20},
	TimeSlot{13, 20},
	TimeSlot{15, 05},
	TimeSlot{16, 05},
}
var timetableBreak = []TimeSlot{
	TimeSlot{8, 45},
	TimeSlot{10, 30},
	TimeSlot{12, 30},
	TimeSlot{14, 15},
	TimeSlot{16, 00},
}

func ttindex(slot TimeSlot) int {
	for i, slotI := range timetableBegin {
		if slotI == slot {
			return i + 1
		}
	}
	return -1
}

type Config struct {
	Token         string  `yaml:"token"`
	DBfile        string  `yaml:"dbfile"`
	Admins        []int   `yaml:"admins"`
	NotifyChats   []int64 `yaml:"notify_chats"`
	NotifyInMins  int     `yaml:"notify_in_mins"`
	NotifyOnEnd   bool    `yaml:"notify_on_end"`
	NotifyOnBreak bool    `yaml:"notify_on_break"`

	Course  int `yaml:"course"`
	Faculty int `yaml:"faculty"`
	Group   int `yaml:"group"`

	GroupMembers []string `yaml:"group_members"`
}

func extractCommand(update *tgbotapi.Update) string {
	if update.Message == nil || update.Message.Text == "" {
		return ""
	}

	for _, entity := range *update.Message.Entities {
		if entity.Type != "bot_command" {
			return ""
		}
		if entity.Offset != 0 {
			return ""
		}
		fullCmd := update.Message.Text[:entity.Length]
		if strings.Contains(fullCmd, "@") && !strings.HasSuffix(fullCmd, bot.Self.UserName) {
			return ""
		}

		splitten := strings.Split(fullCmd, "@")
		return splitten[0][1:]
	}
	return ""
}

func updateNextWeekTimetable() {
	nextWeek := time.Now().In(timezone)
	for nextWeek.Weekday() != time.Monday {
		nextWeek = nextWeek.AddDate(0, 0, 1)
	}

	if err := updateTimetable(nextWeek, nextWeek.AddDate(0, 0, 7)); err != nil {
		log.Println("ERROR: while updating timetable", err)
	}
}

func formatEntry(entry Entry) string {
	ttindx := ttindex(TimeSlot{entry.Time.Hour(), entry.Time.Minute()})

	entryStr := fmt.Sprintf("*%d. Аудитория %s - %s*\n%s - %s, %s, %s",
		ttindx, entry.Classroom, entry.Name,
		entry.Time.Format("15:04"),
		TimeSlotSet(time.Now(), timetableEnd[ttindx-1]).Format("15:04"),
		typeStr[entry.Type], entry.Lecturer)
	return entryStr
}

func FromRaw(date time.Time, e []ttparser.RawEntry) []Entry {
	res := make([]Entry, len(e))
	for i, ent := range e {
		res[i] = Entry{
			TimeSlotSet(date, timetableBegin[ent.Sequence-1]),
			types[strings.ToLower(ent.Type)],
			ent.Classroom,
			ent.Lecturer,
			ent.Name,
		}
	}
	return res
}

func updateTimetable(from time.Time, to time.Time) error {
	entriesRawFull, err := ttparser.DownloadTable(from, to, config.Course, config.Faculty, config.Group)
	if err != nil {
		return errors.Wrapf(err, "table download %v-%v", from, to)
	}

	for date, entriesRaw := range entriesRawFull {
		entries := FromRaw(date, entriesRaw)
		y, err := db.BatchFillable(date)
		if err != nil {
			panic(err)
		}
		log.Println(date, entries)
		if y {
			if err := db.ReplaceDay(date, entries, true); err != nil {
				return errors.Wrapf(err, "db update %v", err)
			}
		}
	}
	return nil
}

func main() {
	confFile, err := ioutil.ReadFile("botconf.yml")
	if err != nil {
		log.Fatalln("Failed to read config file (botconf.yml):", err)
	}
	if err = yaml.UnmarshalStrict(confFile, &config); err != nil {
		log.Fatalln("Failed to decode config file (botconf.yml):", err)
	}

	log.Println("Configuration:")
	log.Println("- Token:", config.Token[:10]+"...")
	log.Println("- DB file:", config.DBfile)
	log.Println("- Admins:", config.Admins)
	log.Println("- Notify targets:", config.NotifyChats)
	log.Println("- Auto-update: Group -", config.Group, "  Faculty -", config.Faculty, "  Course -", config.Course)
	log.Println("- Group members:", len(config.GroupMembers), "people")
	log.Println("- Notify: in", config.NotifyInMins, "before begin; on end:", config.NotifyOnEnd, "; on break:", config.NotifyOnBreak)

	db, err = NewDB(config.DBfile)
	if err != nil {
		log.Fatalln("Failed to open DB:", err)
	}

	bot, err = tgbotapi.NewBotAPI(config.Token)
	if err != nil {
		log.Fatalln("Failed to init Bot API:", err)
	}

	gocron.Every(1).Minute().Do(checkNotifications)
	gocron.Every(1).Day().At("00:00").Do(updateNextWeekTimetable)
	gocron.Start()

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 25
	updates, err := bot.GetUpdatesChan(u)
	if err != nil {
		log.Fatalln("Failed to init. updates channel:", err)
	}

	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGHUP, os.Interrupt)

	log.Println("Started.")
	for true {
		select {
		case s := <-sig:
			log.Printf("%v; stopping...\n", s)
			return
		case update := <-updates:
			if update.CallbackQuery != nil {
				err = handleCallbackQuery(update.CallbackQuery)

				if err != nil {
					log.Printf("ERROR: while processing callback query id %v: %v\n",
						update.CallbackQuery.ID, err)
				}
			} else {
				command := extractCommand(&update)
				if command == "" {
					continue
				}

				var err error
				switch command {
				case "today":
					err = todayCmd(update.Message)
				case "tomorrow":
					err = tomorrowCmd(update.Message)
				case "next":
					err = nextCmd(update.Message)
				case "set":
					err = setCmd(update.Message)
				case "timetable":
					err = timetableCmd(update.Message)
				case "help":
					err = helpCmd(update.Message)
				case "adminhelp":
					err = adminHelpCmd(update.Message)
				case "schedule":
					err = scheduleCmd(update.Message)
				case "clear":
					err = clearCmd(update.Message)
				case "update":
					err = updateCmd(update.Message)
				case "books":
					err = booksCmd(update.Message)
				}

				if err != nil {
					log.Printf("ERROR: while processing command %s in chatid=%d,msgid=%d,uid=%d: %v\n",
						command, update.Message.Chat.ID, update.Message.MessageID, update.Message.From.ID, err)
				}
			}
		}
	}
}
