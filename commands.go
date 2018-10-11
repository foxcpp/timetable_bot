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

func replyTo(msg *tgbotapi.Message, text string) (tgbotapi.Message, error) {
	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ReplyToMessageID = msg.MessageID
	reply.ParseMode = "Markdown"
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
	if _, err := replyTo(replyToTgt, fmt.Sprintf("*Что-то сломалось*\n```\n%s\n```", e)); err != nil {
		log.Println("ERROR:", err)
	}
}

func helpCmd(msg *tgbotapi.Message) error {
	_, err := replyTo(msg, lang.Help)
	return err
}

func adminHelpCmd(msg *tgbotapi.Message) error {
	_, err := replyTo(msg, lang.AdminHelp)
	return err
}

func setCmd(msg *tgbotapi.Message) error {
	if !adminCheck(msg.From.ID) {
		if _, err := replyTo(msg, lang.Replies.MissingPermissions); err != nil {
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
		}
	}

	lines := strings.Split(msg.Text, "\n")

	splittenFirst := strings.Split(lines[0], " ")
	if len(splittenFirst) != 2 {
		if _, err := replyTo(msg, lang.Usage.Set); err != nil {
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
		}
		return nil
	}

	day, err := time.ParseInLocation("02.01.06", splittenFirst[1], timezone)
	if err != nil {
		if _, err := replyTo(msg, lang.Replies.InvalidDate); err != nil {
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
		}
		return nil
	}

	entries := make([]Entry, len(lines)-1)
	for i, line := range lines[1:] {
		entry, err := SplitEntry(line, day)
		if err != nil {
			errMsg := pyfmt.Must(lang.ParseErrors.UnexpectedError, err)
			switch err {
			case ErrInvalidFormat:
				errMsg = lang.ParseErrors.InvalidTimetableFormat
			case ErrTooManyEntires:
				errMsg = lang.ParseErrors.TooManyLessons
			case ErrUnknownType:
				errMsg = lang.ParseErrors.InvalidLessonType
			}
			if _, err := replyTo(msg, errMsg); err != nil {
				return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
			}
			return nil
		}
		entries[i] = entry
	}
	if err := db.ReplaceDay(day, entries, false); err != nil {
		reportError(err, msg)
		return err
	}
	if _, err := replyTo(msg, pyfmt.Must(lang.Replies.TimetableSet, map[string]interface{}{
		"date": day.Format("_2 January 2006"),
	})); err != nil {
		return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
	}
	return nil
}

func clearCmd(msg *tgbotapi.Message) error {
	if !adminCheck(msg.From.ID) {
		if _, err := replyTo(msg, lang.Replies.MissingPermissions); err != nil {
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
		}
	}

	splitten := strings.Split(msg.Text, " ")
	if len(splitten) != 2 {
		if _, err := replyTo(msg, lang.Usage.Clear); err != nil {
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
		}
		return nil
	}

	day, err := time.ParseInLocation("02.01.06", splitten[1], timezone)
	if err != nil {
		if _, err := replyTo(msg, lang.Replies.InvalidDate); err != nil {
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
		}
		return nil
	}

	if err := db.ClearDay(day); err != nil {
		reportError(err, msg)
	}
	if _, err := replyTo(msg, pyfmt.Must(lang.Replies.TimetableClear, map[string]interface{}{
		"date": day.Format("_2 January 2006"),
	})); err != nil {
		return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
	}
	return nil
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
		if _, err := replyTo(msg, lang.Usage.Schedule); err != nil {
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
		}
		return nil
	}

	day, err := time.ParseInLocation("02.01.06", splitten[1], timezone)
	if err != nil {
		if _, err := replyTo(msg, lang.Replies.InvalidDate); err != nil {
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
		}
		return nil
	}

	entries, err := db.OnDay(day)
	if err != nil {
		reportError(err, msg)
		return err
	}

	m, err := replyTo(msg, formatTimetable(day, entries))
	if err != nil {
		return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
	}

	if _, err := bot.Send(tgbotapi.NewEditMessageReplyMarkup(m.Chat.ID, m.MessageID, makeSchedButtons(day))); err != nil {
		return errors.Wrapf(err, "editMessageReplyMarkup chatid=%d, msgid=%d", m.Chat.ID, m.MessageID)
	}

	return nil
}

func todayCmd(msg *tgbotapi.Message) error {
	now := time.Now().In(timezone)
	entries, err := db.OnDay(now)
	if err != nil {
		reportError(err, msg)
		return err
	}

	m, err := replyTo(msg, formatTimetable(now, entries))
	if err != nil {
		return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
	}

	if _, err := bot.Send(tgbotapi.NewEditMessageReplyMarkup(m.Chat.ID, m.MessageID, makeSchedButtons(now))); err != nil {
		return errors.Wrapf(err, "editMessageReplyMarkup chatid=%d, msgid=%d", m.Chat.ID, m.MessageID)
	}

	return nil
}

func tomorrowCmd(msg *tgbotapi.Message) error {
	tomorrow := time.Now().In(timezone).AddDate(0, 0, 1)
	entries, err := db.OnDay(tomorrow)
	if err != nil {
		reportError(err, msg)
		return err
	}

	m, err := replyTo(msg, formatTimetable(tomorrow, entries))
	if err != nil {
		return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
	}

	if _, err := bot.Send(tgbotapi.NewEditMessageReplyMarkup(m.Chat.ID, m.MessageID, makeSchedButtons(tomorrow))); err != nil {
		return errors.Wrapf(err, "editMessageReplyMarkup chatid=%d, msgid=%d", m.Chat.ID, m.MessageID)
	}

	return nil
}

func nextCmd(msg *tgbotapi.Message) error {
	now := time.Now().In(timezone)

	var entry *Entry
	for _, slot := range config.TimeslotsBegin {
		if TimeSlotSet(now, slot).After(now) {
			var err error
			entry, err = db.ExactGet(TimeSlotSet(now, slot))
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
		if _, err := replyTo(msg, lang.Replies.NoMoreLessonsToday); err != nil {
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
		}
		return nil
	}

	if _, err := replyTo(msg, formatEntry(*entry)); err != nil {
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
	if _, err := replyTo(msg, strings.Join(res, "\n")); err != nil {
		return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d: %v", msg.Chat.ID, msg.MessageID, err)
	}
	return nil
}

func updateCmd(msg *tgbotapi.Message) error {
	if !adminCheck(msg.From.ID) {
		if _, err := replyTo(msg, lang.Replies.MissingPermissions); err != nil {
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
		}
	}

	splitten := strings.Split(msg.Text, " ")
	if len(splitten) != 3 {
		if _, err := replyTo(msg, lang.Usage.Update); err != nil {
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
		}
		return nil
	}

	from, err := time.ParseInLocation("02.01.06", splitten[1], timezone)
	if err != nil {
		if _, err := replyTo(msg, lang.Replies.InvalidDate); err != nil {
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
		}
		return nil
	}
	to, err := time.ParseInLocation("02.01.06", splitten[2], timezone)
	if err != nil {
		if _, err := replyTo(msg, lang.Replies.InvalidDate); err != nil {
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
		}
		return nil
	}

	if err := updateTimetable(from, to); err != nil {
		reportError(err, msg)
		return err
	}
	if _, err := replyTo(msg, "Done."); err != nil {
		return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
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

	entries, err := db.OnDay(date)
	if err != nil {
		return errors.Wrap(err, "db query")
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

func booksCmd(msg *tgbotapi.Message) error {
	if len(config.GroupMembers) == 0 {
		if _, err := replyTo(msg, lang.Replies.BooksCommandDisabled); err != nil {
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
		}
	}

	var r1, r2 int
	for r1 == r2 {
		r1 = rand.Intn(len(config.GroupMembers))
		if r1 == 24 {
			if rand.Intn(4) != 1 {
				r1 = rand.Intn(len(config.GroupMembers))
			}
		}
		r2 = rand.Intn(len(config.GroupMembers))
		if r2 == 24 {
			if rand.Intn(4) != 1 {
				r2 = rand.Intn(len(config.GroupMembers))
			}
		}
	}

	reply := fmt.Sprintf("%s и %s", config.GroupMembers[r1], config.GroupMembers[r2])
	if _, err := replyTo(msg, reply); err != nil {
		return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
	}
	return nil
}

func init() {
	rand.Seed(time.Now().Unix())
}
