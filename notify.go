package main

import (
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

	entries, err := db.OnDay(now)
	if err != nil {
		log.Printf("ERROR: While querying entries for %v: %v.\n", now, err)
		return
	}
	if len(entries) != 0 &&
		now.Add(time.Minute * 25).Hour() == entries[0].Time.Hour() &&
		now.Add(time.Minute * 25).Minute() == entries[0].Time.Minute() {

		broadcastNotify(formatEntry(entries[0]))
	}

	entry, err := db.ExactGet(now.Add(time.Minute * time.Duration(config.NotifyInMins)))
	if err != nil {
		log.Printf("ERROR: While querying entry for %v: %v.\n", now.Add(time.Minute*time.Duration(config.NotifyInMins)), err)
		return
	}
	if entry != nil {
		broadcastNotify(formatEntry(*entry))
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
