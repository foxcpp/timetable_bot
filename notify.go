package main

import (
	"log"
	"time"

	"github.com/SevereCloud/vksdk/v2/api/params"
)

func checkNotifications() {
	now := time.Now().In(timezone)

	if config.NotifyOnEnd {
		for _, slot := range config.TimeslotsEnd {
			if slot == (TimeSlot{now.Hour(), now.Minute()}) {
				broadcastNotify(lang.LessonEndNotify)
			}
		}
	}
	if config.NotifyOnBreak {
		for _, slot := range config.TimeslotsEnd {
			if slot == (TimeSlot{now.Hour(), now.Minute()}) {
				broadcastNotify(lang.BreakNotify)
			}
		}
	}

	entries, err := cache.OnDay(now)
	if err != nil {
		log.Printf("ERROR: While querying entries for %v: %v.\n", now, err)
		return
	}
	if len(entries) != 0 &&
		now.Add(time.Minute*25).Hour() == entries[0].Time.Hour() &&
		now.Add(time.Minute*25).Minute() == entries[0].Time.Minute() {

		broadcastNotify(formatEntry(entries[0]))
	}

	entry, err := cache.ExactGet(now.Add(time.Minute * time.Duration(config.NotifyInMins)))
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
		b := params.NewMessagesSendBuilder()
		b.PeerID(int(chat))
		b.RandomID(0)
		b.Message(notifyStr)
		if _, err := bot.MessagesSend(b.Params); err != nil {
			log.Printf("ERROR: Failed to send notification to chatid=%d: %v", chat, err)
		}
	}
}
