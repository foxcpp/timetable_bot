package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/SevereCloud/vksdk/v2/api/params"
	"github.com/SevereCloud/vksdk/v2/events"
	"github.com/SevereCloud/vksdk/v2/object"
	"github.com/pkg/errors"
	"github.com/slongfield/pyfmt"
)

func replyTo(msg *events.MessageNewObject, text string, keyboard interface{}) (int, error) {
	b := params.NewMessagesSendBuilder()
	b.Message(text)
	b.RandomID(0)
	b.PeerID(msg.Message.PeerID)
	if keyboard != nil {
		b.Keyboard(keyboard)
	}

	return bot.MessagesSend(b.Params)
}

func adminCheck(uid int) bool {
	for _, id := range config.Admins {
		if id == uid {
			return true
		}
	}
	return false
}

func reportError(e error, replyToTgt *events.MessageNewObject) {
	if _, err := replyTo(replyToTgt, fmt.Sprintf("*Что-то сломалось*\n```\n%s\n```", e), nil); err != nil {
		log.Println("ERROR:", err)
	}
}

func helpCmd(msg *events.MessageNewObject) error {
	_, err := replyTo(msg, lang.Help, nil)
	return err
}

func adminHelpCmd(msg *events.MessageNewObject) error {
	_, err := replyTo(msg, lang.AdminHelp, nil)
	return err
}

func formatTimetable(date time.Time, entries []Entry, staleEntries bool) string {
	var hdr string
	if staleEntries {
		hdr = pyfmt.Must(lang.Replies.TimetableHeaderStale, map[string]interface{}{
			"date": date.Format("_2 January  2006"),
		})
	} else {
		hdr = pyfmt.Must(lang.Replies.TimetableHeader, map[string]interface{}{
			"date": date.Format("_2 January  2006"),
		})
	}
	entriesStr := make([]string, len(entries))
	for i, entry := range entries {
		entriesStr[i] = formatEntry(entry)
	}
	if len(entriesStr) == 0 {
		entriesStr = append(entriesStr, lang.Replies.Empty)
	}
	return hdr + strings.Join(entriesStr, "\n\n")
}

func makeSchedButtons(date time.Time) object.MessagesKeyboard {
	kb := &object.MessagesKeyboard{}
	kb.Inline = true
	kb = kb.AddRow()
	kb = kb.AddCallbackButton("\u25C0", date.AddDate(0, 0, -1).Format("02.01.06"), "secondary")
	kb = kb.AddCallbackButton("\u25B6", date.AddDate(0, 0, 1).Format("02.01.06"), "secondary")
	return *kb
}

func scheduleCmd(msg *events.MessageNewObject) error {
	splitten := strings.Split(msg.Message.Text, " ")
	if len(splitten) != 2 {
		if _, err := replyTo(msg, lang.Usage.Schedule, nil); err != nil {
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Message.PeerID, msg.Message.ID)
		}
		return nil
	}

	day, err := time.ParseInLocation("02.01.06", splitten[1], timezone)
	if err != nil {
		if _, err := replyTo(msg, lang.Replies.InvalidDate, nil); err != nil {
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Message.PeerID, msg.Message.ID)
		}
		return nil
	}

	entries, err := cache.OnDay(day)
	staleEntries := false
	if err != nil {
		if _, ok := err.(StaleEntriesError); ok {
			staleEntries = true
			log.Printf("ERROR: sending stale entries for %v to chatid=%d,msgid=%d,uid=%d: %v\n",
				day, msg.Message.PeerID, msg.Message.ID, msg.Message.FromID, err)
		} else {
			reportError(err, msg)
			return err
		}
	}

	_, err = replyTo(msg, formatTimetable(day, entries, staleEntries), makeSchedButtons(day))
	if err != nil {
		return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Message.PeerID, msg.Message.ID)
	}

	return nil
}

func todayCmd(msg *events.MessageNewObject) error {
	now := time.Now().In(timezone)
	entries, err := cache.OnDay(now)
	staleEntries := false
	if err != nil {
		if _, ok := err.(StaleEntriesError); ok {
			staleEntries = true
			log.Printf("ERROR: sending stale entries for %v to chatid=%d,msgid=%d,uid=%d: %v\n",
				now, msg.Message.PeerID, msg.Message.ID, msg.Message.FromID, err)
		} else {
			reportError(err, msg)
			return err
		}
	}

	_, err = replyTo(msg, formatTimetable(now, entries, staleEntries), makeSchedButtons(now))
	if err != nil {
		return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Message.PeerID, msg.Message.ID)
	}

	return nil
}

func tomorrowCmd(msg *events.MessageNewObject) error {
	tomorrow := time.Now().In(timezone).AddDate(0, 0, 1)
	entries, err := cache.OnDay(tomorrow)
	staleEntries := false
	if err != nil {
		if _, ok := err.(StaleEntriesError); ok {
			staleEntries = true
			log.Printf("ERROR: sending stale entries for %v to chatid=%d,msgid=%d,uid=%d: %v\n",
				tomorrow, msg.Message.PeerID, msg.Message.ID, msg.Message.FromID, err)
		} else {
			reportError(err, msg)
			return err
		}
	}

	_, err = replyTo(msg, formatTimetable(tomorrow, entries, staleEntries), makeSchedButtons(tomorrow))
	if err != nil {
		return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Message.PeerID, msg.Message.ID)
	}

	return nil
}

func nextCmd(msg *events.MessageNewObject) error {
	now := time.Now().In(timezone)

	var entry *Entry
	for _, slot := range config.TimeslotsBegin {
		if TimeSlotSet(now, slot).After(now) {
			var err error
			entry, err = cache.ExactGet(TimeSlotSet(now, slot))
			if err != nil {
				reportError(err, msg)
				return err
			}
			if entry != nil {
				break
			}
		}
	}
	if entry == nil {
		if _, err := replyTo(msg, lang.Replies.NoMoreLessonsToday, nil); err != nil {
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Message.PeerID, msg.Message.ID)
		}
		return nil
	}

	if _, err := replyTo(msg, formatEntry(*entry), nil); err != nil {
		return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Message.PeerID, msg.Message.ID)
	}
	return nil
}

func timetableCmd(msg *events.MessageNewObject) error {
	res := make([]string, len(config.TimeslotsBegin))
	for i := 0; i < len(config.TimeslotsBegin); i++ {
		res[i] = pyfmt.Must(lang.TimeslotFormat, map[string]interface{}{
			"num":   i + 1,
			"start": TimeSlotSet(time.Now().In(timezone), config.TimeslotsBegin[i]).Format("15:04"),
			"end":   TimeSlotSet(time.Now().In(timezone), config.TimeslotsEnd[i]).Format("15:04"),
			"break": TimeSlotSet(time.Now().In(timezone), config.TimeslotsBreak[i]).Format("15:04"),
		})
	}
	if _, err := replyTo(msg, strings.Join(res, "\n"), nil); err != nil {
		return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Message.PeerID, msg.Message.ID)
	}
	return nil
}

func handleCallbackQuery(_ context.Context, query events.MessageEventObject) {
	var dateRaw string
	json.Unmarshal(query.Payload, &dateRaw)

	date, err := time.ParseInLocation("02.01.06", dateRaw, timezone)
	if err != nil {
		log.Println("handleCallbackQuery: cache error: ", err)
		return
	}

	entries, err := cache.OnDay(date)
	staleEntries := false
	if err != nil {
		if _, ok := err.(StaleEntriesError); ok {
			staleEntries = true
			log.Printf("ERROR: sending stale entries for %v to chatid=%d,msgid=%d,uid=%d: %v\n",
				date, query.PeerID, query.ConversationMessageID, query.UserID, err)
		} else {
			log.Println("handleCallbackQuery: cache error: ", err)
			return
		}
	}

	b := params.NewMessagesEditBuilder()
	b.PeerID(query.PeerID)
	b.Message(formatTimetable(date, entries, staleEntries))
	b.MessageID(query.ConversationMessageID)
	b.Params["keyboard"] = makeSchedButtons(date)
	if _, err := bot.MessagesEdit(b.Params); err != nil {
		log.Println("handleCallbackQuery: ", err)
		return
	}

	ans := params.NewMessagesSendMessageEventAnswerBuilder()
	ans.UserID(query.UserID)
	ans.PeerID(query.PeerID)
	ans.EventID(query.EventID)
	if _, err := bot.MessagesSendMessageEventAnswer(b.Params); err != nil {
		log.Println("handleCallbackQuery: ", err)
		return
	}
}

func evictCmd(msg *events.MessageNewObject) error {
	splitten := strings.Split(msg.Message.Text, " ")
	if len(splitten) != 2 {
		if _, err := replyTo(msg, lang.Usage.Evict, nil); err != nil {
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Message.PeerID, msg.Message.ID)
		}
		return nil
	}

	day, err := time.ParseInLocation("02.01.06", splitten[1], timezone)
	if err != nil {
		if _, err := replyTo(msg, lang.Replies.InvalidDate, nil); err != nil {
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Message.PeerID, msg.Message.ID)
		}
		return nil
	}

	cache.Evict(day)
	if _, err := replyTo(msg, "OK!", nil); err != nil {
		return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Message.PeerID, msg.Message.ID)
	}
	return nil
}
