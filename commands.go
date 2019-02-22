package main

import (
	"fmt"
	"github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/pkg/errors"
	"github.com/slongfield/pyfmt"
	"log"
	"math/rand"
	"strings"
	"time"
)

func replyTo(msg *tgbotapi.Message, text string, markup interface{}) (tgbotapi.Message, error) {
	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ReplyToMessageID = msg.MessageID
	reply.ParseMode = "Markdown"
	reply.ReplyMarkup = markup
	return bot.Send(reply)
}

func adminCheck(uid int) bool {
	for _, id := range config.Admins {
		if id == uid {
			return true
		}
	}
	return false
}

func reportError(e error, replyToTgt *tgbotapi.Message) {
	if _, err := replyTo(replyToTgt, fmt.Sprintf("*Что-то сломалось*\n```\n%s\n```", e), nil); err != nil {
		log.Println("ERROR:", err)
	}
}

func helpCmd(msg *tgbotapi.Message) error {
	_, err := replyTo(msg, lang.Help, nil)
	return err
}

func adminHelpCmd(msg *tgbotapi.Message) error {
	_, err := replyTo(msg, lang.AdminHelp, nil)
	return err
}

func formatTimetable(date time.Time, entries []Entry) string {
	hdr := pyfmt.Must(lang.Replies.TimetableHeader, map[string]interface{}{
		"date": date.Format("_2 January  2006"),
	})
	entriesStr := make([]string, len(entries))
	for i, entry := range entries {
		entriesStr[i] = formatEntry(entry)
	}
	if len(entriesStr) == 0 {
		entriesStr = append(entriesStr, lang.Replies.Empty)
	}
	return hdr + strings.Join(entriesStr, "\n\n")
}

func makeSchedButtons(date time.Time) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.NewInlineKeyboardMarkup([]tgbotapi.InlineKeyboardButton{
		tgbotapi.NewInlineKeyboardButtonData("\u25C0",
			date.AddDate(0, 0, -1).Format("02.01.06")),
		tgbotapi.NewInlineKeyboardButtonData("\u25B6",
			date.AddDate(0, 0, 1).Format("02.01.06"))})
}

func scheduleCmd(msg *tgbotapi.Message) error {
	splitten := strings.Split(msg.Text, " ")
	if len(splitten) != 2 {
		if _, err := replyTo(msg, lang.Usage.Schedule, nil); err != nil {
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
		}
		return nil
	}

	day, err := time.ParseInLocation("02.01.06", splitten[1], timezone)
	if err != nil {
		if _, err := replyTo(msg, lang.Replies.InvalidDate, nil); err != nil {
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
		}
		return nil
	}

	entries, err := cache.OnDay(day)
	if err != nil {
		reportError(err, msg)
		return err
	}

	_, err = replyTo(msg, formatTimetable(day, entries), makeSchedButtons(day))
	if err != nil {
		return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
	}

	return nil
}

func todayCmd(msg *tgbotapi.Message) error {
	now := time.Now().In(timezone)
	entries, err := cache.OnDay(now)
	if err != nil {
		reportError(err, msg)
		return err
	}

	_, err = replyTo(msg, formatTimetable(now, entries), makeSchedButtons(now))
	if err != nil {
		return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
	}

	return nil
}

func tomorrowCmd(msg *tgbotapi.Message) error {
	tomorrow := time.Now().In(timezone).AddDate(0, 0, 1)
	entries, err := cache.OnDay(tomorrow)
	if err != nil {
		reportError(err, msg)
		return err
	}

	_, err = replyTo(msg, formatTimetable(tomorrow, entries), makeSchedButtons(tomorrow))
	if err != nil {
		return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
	}

	return nil
}

func nextCmd(msg *tgbotapi.Message) error {
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
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
		}
		return nil
	}

	if _, err := replyTo(msg, formatEntry(*entry), nil); err != nil {
		return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
	}
	return nil
}

func timetableCmd(msg *tgbotapi.Message) error {
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
		return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d: %v", msg.Chat.ID, msg.MessageID, err)
	}
	return nil
}

func handleCallbackQuery(query *tgbotapi.CallbackQuery) error {
	if query.Data == "" {
		return errors.New("no data in callback query")
	}
	if query.Message == nil {
		return errors.New("message too old")
	}
	date, err := time.ParseInLocation("02.01.06", query.Data, timezone)
	if err != nil {
		return errors.Wrap(err, "parse data date")
	}

	entries, err := cache.OnDay(date)
	if err != nil {
		return errors.Wrap(err, "cache query")
	}

	cfg := tgbotapi.NewEditMessageText(query.Message.Chat.ID, query.Message.MessageID, formatTimetable(date, entries))
	newReplyMarkup := makeSchedButtons(date)
	cfg.ParseMode = "Markdown"
	cfg.ReplyMarkup = &newReplyMarkup

	if _, err := bot.Send(cfg); err != nil {
		return errors.Wrap(err, "edit msg text")
	}

	if _, err := bot.AnswerCallbackQuery(tgbotapi.NewCallback(query.ID, "")); err != nil {
		return errors.Wrapf(err, "answerCallbackQuery %v", query.ID)
	}
	return nil
}

func evictCmd(msg *tgbotapi.Message) error {
	splitten := strings.Split(msg.Text, " ")
	if len(splitten) != 2 {
		if _, err := replyTo(msg, lang.Usage.Evict, nil); err != nil {
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
		}
		return nil
	}

	day, err := time.ParseInLocation("02.01.06", splitten[1], timezone)
	if err != nil {
		if _, err := replyTo(msg, lang.Replies.InvalidDate, nil); err != nil {
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
		}
		return nil
	}

	cache.Evict(day)
	if _, err := replyTo(msg, "OK!", nil); err != nil {
		return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
	}
	return nil
}

func booksCmd(msg *tgbotapi.Message) error {
	if len(config.GroupMembers) == 0 {
		if _, err := replyTo(msg, lang.Replies.BooksCommandDisabled, nil); err != nil {
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
		}
	}

	var r1, r2 int
	for r1 == r2 {
		r1 = rand.Intn(len(config.GroupMembers))
		r2 = rand.Intn(len(config.GroupMembers))
	}

	reply := fmt.Sprintf("%s и %s", config.GroupMembers[r1], config.GroupMembers[r2])
	if _, err := replyTo(msg, reply, nil); err != nil {
		return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
	}
	return nil
}

func init() {
	rand.Seed(time.Now().Unix())
}
