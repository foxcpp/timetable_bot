package main

import (
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/foxcpp/timetable_bot/timetableparser"
	"github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/jasonlvhit/gocron"
	"github.com/pkg/errors"
	"github.com/slongfield/pyfmt"
	"gopkg.in/yaml.v2"
)

var bot *tgbotapi.BotAPI
var db *DB
var config Config
var lang LangStrings

type TimeSlot struct {
	Hour, Minute int
}

func (ts *TimeSlot) UnmarshalYAML(unmarshal func(interface{}) error) error {
	str := ""
	if err := unmarshal(&str); err != nil {

	}
	splitten := strings.Split(str, ":")
	if len(splitten) != 2 {
		return errors.New("invalid timeslot format")
	}
	hourStr := splitten[0]
	minuteStr := splitten[1]
	if hour, err := strconv.Atoi(hourStr); err != nil {
		return errors.Wrap(err, "invalid hour value")
	} else {
		ts.Hour = hour
	}
	if minute, err := strconv.Atoi(minuteStr); err != nil {
		return errors.Wrap(err, "invalid minute value")
	} else {
		ts.Minute = minute
	}
	return nil
}

func ttindex(slot TimeSlot) int {
	for i, slotI := range config.TimeslotsBegin {
		if slotI == slot {
			return i + 1
		}
	}
	return -1
}

type Config struct {
	Lang              string `yaml:"lang"`
	Token             string `yaml:"token"`
	DBfile            string `yaml:"dbfile"`
	CmdProcGoroutines int    `yaml:"cmd_processing_goroutines"`

	Admins []int `yaml:"admins"`

	NotifyChats   []int64 `yaml:"notify_chats"`
	NotifyInMins  int     `yaml:"notify_in_mins"`
	NotifyOnEnd   bool    `yaml:"notify_on_end"`
	NotifyOnBreak bool    `yaml:"notify_on_break"`

	TimeZone       string     `yaml:"timezone"`
	TimeslotsBegin []TimeSlot `yaml:"timeslots_begin"`
	TimeslotsBreak []TimeSlot `yaml:"timeslots_break"`
	TimeslotsEnd   []TimeSlot `yaml:"timeslots_end"`

	AutoUpdate   ttparser.AutoUpdateCfg `yaml:"autoupdate"`
	GroupMembers []string               `yaml:"group_members"`
}

type LangStrings struct {
	LessonTypes    map[LessonType]string `yaml:"lesson_types"`
	LessonTypeStrs map[string]LessonType `yaml:"lesson_types_short"`
	Help           string                `yaml:"help"`
	AdminHelp      string                `yaml:"adminhelp"`
	Usage          struct {
		Set      string `yaml:"set"`
		Clear    string `yaml:"clear"`
		Schedule string `yaml:"schedule"`
		Update   string `yaml:"update"`
	} `yaml:"usage"`
	Replies struct {
		SomethingBroke       string `yaml:"something_broke"`
		MissingPermissions   string `yaml:"missing_permissions"`
		InvalidDate          string `yaml:"invalid_date"`
		TimetableSet         string `yaml:"timetable_set"`
		TimetableClear       string `yaml:"timetable_clear"`
		TimetableHeader      string `yaml:"timetable_header"`
		Empty                string `yaml:"empty"`
		BooksCommandDisabled string `yaml:"books_command_disabled"`
		BooksCommand         string `yaml:"books_command"`
		NoMoreLessonsToday   string `yaml:"no_more_lessons_today"`
	} `yaml:"replies"`
	ParseErrors struct {
		UnexpectedError        string `yaml:"unexpected_error"`
		InvalidTimetableFormat string `yaml:"invalid_timetable_format"`
		TooManyLessons         string `yaml:"too_many_lessons"`
		InvalidLessonType      string `yaml:"invalid_lesson_type"`
	} `yaml:"parse_errors"`
	EntryTemplate   string `yaml:"entry_template"`
	LessonEndNotify string `yaml:"lesson_end_notify"`
	BreakNotify     string `yaml:"break_notify"`
	TimeslotFormat  string `yaml:"timeslot_format"`
}

func extractCommand(update *tgbotapi.Update) string {
	if update == nil || update.Message == nil || update.Message.Text == "" {
		return ""
	}
	if update.Message.Entities == nil {
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

	log.Println("Updating next-week timetable")

	if err := updateTimetable(nextWeek, nextWeek.AddDate(0, 0, 6)); err != nil {
		log.Println("ERROR: while updating timetable", err)
	}
}

func formatEntry(entry Entry) string {
	ttindx := ttindex(TimeSlot{entry.Time.Hour(), entry.Time.Minute()})

	return pyfmt.Must(lang.EntryTemplate, map[string]interface{}{
		"num":       ttindx,
		"classroom": entry.Classroom,
		"name":      entry.Name,
		"startTime": entry.Time.Format("15:04"),
		"endTime":   TimeSlotSet(time.Now(), config.TimeslotsEnd[ttindx-1]).Format("15:04"),
		"type":      lang.LessonTypes[entry.Type],
		"lecturer":  entry.Lecturer,
	})
}

func FromRaw(date time.Time, e []ttparser.RawEntry) []Entry {
	res := make([]Entry, len(e))
	for i, ent := range e {
		res[i] = Entry{
			TimeSlotSet(date, config.TimeslotsBegin[ent.Sequence-1]),
			types[strings.ToLower(ent.Type)],
			ent.Classroom,
			ent.Lecturer,
			ent.Name,
		}
	}
	return res
}

func updateTimetable(from time.Time, to time.Time) error {
	entriesRawFull, err := ttparser.DownloadTable(from, to, config.AutoUpdate)
	if err != nil {
		return errors.Wrapf(err, "table download %v-%v", from, to)
	}

	for date, entriesRaw := range entriesRawFull {
		entries := FromRaw(date, entriesRaw)
		y, err := db.BatchFillable(date)
		if err != nil {
			panic(err)
		}
		if y {
			if err := db.ReplaceDay(date, entries, true); err != nil {
				return errors.Wrapf(err, "db update %v", err)
			}
		}
	}
	return nil
}

func processUpdates(updates <-chan tgbotapi.Update) {
	for {
		update := <-updates
		if update.CallbackQuery != nil {
			err := handleCallbackQuery(update.CallbackQuery)

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

func main() {
	if os.Getenv("USING_SYSTEMD") == "1" {
		// Don't log timestamp since journald records it anyway.
		log.SetFlags(0)
	}

	if len(os.Args) != 3 {
		log.Fatalln("Usage:", os.Args[0], "<config file> <db file>")
	}
	configPath := os.Args[1]
	DBFile := os.Args[2]

	confFile, err := ioutil.ReadFile(configPath)
	if err != nil {
		log.Fatalln("Failed to read config file (botconf.yml):", err)
	}
	if err = yaml.UnmarshalStrict(confFile, &config); err != nil {
		log.Fatalln("Failed to decode config file (botconf.yml):", err)
	}

	langFile, err := ioutil.ReadFile(config.Lang)
	if err != nil {
		log.Fatalln("Failed read lang file:", err)
	}
	if err = yaml.UnmarshalStrict(langFile, &lang); err != nil {
		log.Fatalln("Failed to decode lang file:", err)
	}

	timezone, err = time.LoadLocation(config.TimeZone)
	if err != nil {
		log.Fatalln("Failed to set timezone:", err)
	}

	log.Println("Configuration:")
	log.Println("- Lang file:", config.Lang)
	log.Println("- Token:", config.Token[:10]+"...")
	log.Println("- Timezone:", timezone)
	log.Println("- DB file:", DBFile)
	log.Println("- Admins:", config.Admins)
	log.Println("- Notify targets:", config.NotifyChats)
	log.Printf("- Auto-update: %+v\n", config.AutoUpdate)
	log.Println("- Group members:", len(config.GroupMembers), "people")
	log.Println("- Notify: in", config.NotifyInMins, "before begin; on end:", config.NotifyOnEnd, "; on break:", config.NotifyOnBreak)

	db, err = NewDB(DBFile)
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

	if os.Getenv("USING_SYSTEMD") == "1" {
		cmd := exec.Command("systemd-notify", "--ready", `--status=Listening for updates`)
		if out, err := cmd.Output(); err != nil {
			log.Println("Failed to notify systemd about successful startup:", err)
			log.Println(string(out))
		}
	}

	for i := 0; i < config.CmdProcGoroutines; i++ {
		go processUpdates(updates)
	}

	s := <-sig
	log.Printf("%v; stopping...\n", s)
}
