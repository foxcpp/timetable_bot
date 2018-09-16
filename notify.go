package main

import (
	"fmt"
	"github.com/go-telegram-bot-api/telegram-bot-api"
	"log"
	"time"
)

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

	entry, err := db.ExactGet(now.Add(time.Minute * time.Duration(config.NotifyInMins)))
	if err != nil {
		log.Printf("ERROR: While querying entry for %v: %v.\n", now.Add(time.Minute*time.Duration(config.NotifyInMins)), err)
		return
	}
	if entry != nil {
		ttindx := ttindex(TimeSlot{entry.Time.Hour(), entry.Time.Minute()})
		entryStr := fmt.Sprintf("*%d. Аудитория %s - %s*\n%s - %s, %s, %s\n",
			ttindx, entry.Classroom, entry.Name,
			entry.Time.Format("15:04"),
			TimeSlotSet(now, timetableEnd[ttindx-1]).Format("15:04"),
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
