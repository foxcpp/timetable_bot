package main

import (
	"context"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/SevereCloud/vksdk/v2/api"
	"github.com/SevereCloud/vksdk/v2/events"
	"github.com/SevereCloud/vksdk/v2/longpoll-bot"
	"github.com/foxcpp/timetable_bot/ttparser"
	"github.com/jasonlvhit/gocron"
	"github.com/pkg/errors"
	"github.com/slongfield/pyfmt"
	"gopkg.in/yaml.v2"
)

var bot *api.VK
var cache *Cache
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
	GroupID           int    `yaml:"group_id"`
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

	SourceCfg    ttparser.Cfg `yaml:"source_cfg"`
	GroupMembers []string     `yaml:"group_members"`
}

type LangStrings struct {
	LessonTypes    map[LessonType]string `yaml:"lesson_types"`
	LessonTypeStrs map[string]LessonType `yaml:"lesson_types_short"`
	Help           string                `yaml:"help"`
	AdminHelp      string                `yaml:"adminhelp"`
	Usage          struct {
		Schedule string `yaml:"schedule"`
		Evict    string `yaml:"evict"`
	} `yaml:"usage"`
	Replies struct {
		SomethingBroke       string `yaml:"something_broke"`
		MissingPermissions   string `yaml:"missing_permissions"`
		InvalidDate          string `yaml:"invalid_date"`
		TimetableHeader      string `yaml:"timetable_header"`
		TimetableHeaderStale string `yaml:"timetable_header_stale"`
		Empty                string `yaml:"empty"`
		NoMoreLessonsToday   string `yaml:"no_more_lessons_today"`
	} `yaml:"replies"`
	EntryTemplate   string `yaml:"entry_template"`
	LessonEndNotify string `yaml:"lesson_end_notify"`
	BreakNotify     string `yaml:"break_notify"`
	TimeslotFormat  string `yaml:"timeslot_format"`
}

func extractCommand(msg events.MessageNewObject) string {
	if !strings.HasPrefix(msg.Message.Text, "!") {
		return ""
	}

	splitten := strings.Split(msg.Message.Text, " ")
	return splitten[0][1:]
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

func handleMessage(_ context.Context, msg_ events.MessageNewObject) {
	log.Printf("Received %d '%s'", msg_.Message.PeerID, msg_.Message.Text)
	command := extractCommand(msg_)
	if command == "" {
		return

	}

	msg := &msg_

	var err error
	switch command {
	case "today":
		err = todayCmd(msg)
	case "tomorrow":
		err = tomorrowCmd(msg)
	case "next":
		err = nextCmd(msg)
	case "timetable":
		err = timetableCmd(msg)
	case "help":
		err = helpCmd(msg)
	case "adminhelp":
		err = adminHelpCmd(msg)
	case "schedule":
		err = scheduleCmd(msg)
	case "evict":
		err = evictCmd(msg)
	}

	if err != nil {
		log.Printf("ERROR: while processing command %s in chatid=%d,msgid=%d,uid=%d: %v\n",
			command, msg.Message.PeerID, msg.Message.ID, msg.Message.FromID, err)
	}
}

func main() {
	if os.Getenv("USING_SYSTEMD") == "1" {
		// Don't log timestamp since journald records it anyway.
		log.SetFlags(0)
	}

	if len(os.Args) != 2 {
		log.Fatalln("Usage:", os.Args[0], "<config file>")
	}
	configPath := os.Args[1]

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
	log.Println("- Admins:", config.Admins)
	log.Println("- Notify targets:", config.NotifyChats)
	log.Printf("- Source: %+v\n", config.SourceCfg)
	log.Println("- Group members:", len(config.GroupMembers), "people")
	log.Println("- Notify: in", config.NotifyInMins, "before begin; on end:", config.NotifyOnEnd, "; on break:", config.NotifyOnBreak)

	cache = NewCache()

	bot = api.NewVK(config.Token)

	gocron.Every(1).Minute().Do(checkNotifications)
	gocron.Start()

	u, err := longpoll.NewLongPoll(bot, config.GroupID)
	if err != nil {
		log.Fatalln("Failed to init. updates channel:", err)
	}

	u.MessageNew(handleMessage)
	u.MessageEvent(handleCallbackQuery)
	go func() {
		err := u.Run()
		if err != nil {
			log.Println("Longpool error:", err)
		}
	}()

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

	s := <-sig
	log.Printf("%v; stopping...\n", s)
	u.Shutdown()
}
