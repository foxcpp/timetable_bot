package main

import (
	"fmt"
	"github.com/go-telegram-bot-api/telegram-bot-api"
	"gopkg.in/yaml.v2"
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
}
var timetableEnd = []TimeSlot{
	TimeSlot{9, 35},
	TimeSlot{11, 20},
	TimeSlot{13, 20},
	TimeSlot{15, 05},
}
var timetableBreak = []TimeSlot{
	TimeSlot{8, 45},
	TimeSlot{10, 30},
	TimeSlot{12, 30},
	TimeSlot{14, 15},
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
	Token string `yaml:"token"`
	DBfile string `yaml:"dbfile"`
	Admins []int `yaml:"admins"`
	NotifyChats []int64 `yaml:"notify_chats"`
	NotifyInMins int `yaml:"notify_in_mins"`
	NotifyOnEnd bool `yaml:"notify_on_end"`
	NotifyOnBreak bool `yaml:"notify_on_break"`
}

func checkNotifications() {
	now := time.Now().In(timezone)

	if config.NotifyOnEnd {
		for _, slot := range timetableEnd {
			if slot == (TimeSlot{now.Hour(), now.Minute()}) {
				broadcastNotify("Конец пары!")
			}
		}
	}
	if config.NotifyOnBreak {
		for _, slot := range timetableEnd {
			if slot == (TimeSlot{now.Hour(), now.Minute()}) {
				broadcastNotify("Перерыв!")
			}
		}
	}

	entry, err := db.ExactGet(now.Add(time.Minute * 12))
	if err != nil {
		log.Printf("ERROR: While querying entry for %v: %v.\n", now.Add(time.Minute * 12), err)
	}
	if entry != nil {
		ttindx := ttindex(TimeSlot{entry.Time.Hour(), entry.Time.Minute()})
		entryStr := fmt.Sprintf("*%d. Аудитория %s - %s\n%d:%d - %d:%d, %s, %s\n",
			ttindx, entry.Classroom, entry.Name,
			entry.Time.Hour(), entry.Time.Minute(),
			timetableEnd[ttindx-1].Hour, timetableEnd[ttindx-1].Minute,
			typeStr[entry.Type], entry.Lecturer)

		broadcastNotify(entryStr)
	}
}
func broadcastNotify(notifyStr string) {
	for _, chat := range config.NotifyChats {
		msg := tgbotapi.NewMessage(chat, notifyStr)
		msg.ParseMode = "Markdown"
		if _, err := bot.Send(msg); err != nil {
			log.Printf("ERROR: Failed to send notification to chatid=%d: %v", chat, err)
		}
	}
}

func extractCommand(update *tgbotapi.Update) string {
	if update.Message == nil || !bot.IsMessageToMe(*update.Message) || update.Message.Text == "" {
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

func notifier() {
	time.Sleep(time.Second * time.Duration(60 - time.Now().Unix() % 60))
	t := time.NewTicker(time.Second * 60)

	for true {
		<-t.C
		checkNotifications()
	}
}

func main() {
	confFile, err := ioutil.ReadFile("botconf.yml")
	if err != nil {
		log.Fatalln("Failed to write config file (botconf.yml):", err)
	}
	if err = yaml.Unmarshal(confFile, &config); err != nil {
		log.Fatalln("Failed to decode config file (botconf.yml):", err)
	}

	db, err = NewDB(config.DBfile)
	if err != nil {
		log.Fatalln("Failed to open DB:", err)
	}

	bot, err = tgbotapi.NewBotAPI(config.Token)
	if err != nil {
		log.Fatalln("Failed to init Bot API:", err)
	}

	go notifier()

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
				}

				if err != nil {
					log.Printf("ERROR: while processing command %s in chatid=%d,msgid=%d,uid=%d: %v\n",
						command, update.Message.Chat.ID, update.Message.MessageID, update.Message.From.ID, err)
				}
		}
	}
}
