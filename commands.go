package main

import (
	"fmt"
	"github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/pkg/errors"
	"log"
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

var typeStr = map[LessonType]string {
	Lab: "Лабараторная",
	Practice: "Практическое занятие",
	Lecture: "Лекция",
}

const helpText = `/help  - _Этот текст_
/adminhelp  -  _Справка по админским командам_

*Команды для студентов*
/today  -  _Расписание на сегодня_
/tomorrow  -  _Расписание на завтра_
/schedule ДАТА  -  _Расписание на указанный день_
/next  -  _Показать информацию о следующуей паре_

Даты указываюстся в формате ` + "`ДЕНЬ.ЧИСЛОМЕСЯЦА.ГОД`"

const adminhelpText = `*Админские команды*
/set ДАТА РАСПИСАНИЕ -  _Задать расписание на определенную дату (см. ниже)_
/clear ДАТА -  _Удалить расписание на указаный день_

Даты указываюстся в формате ` + "`ДЕНЬ.ЧИСЛОМЕСЯЦА.ГОД`" + `.

Пункты расписания задаются в следующем формате:
` + "```" + `
ПОРЯДКОВЫЙ_НОМЕР. АУДИТОРИЯ "Название" ТИП "Предподаватель"
` + "```" + `
Кавычки *обязательны*. ТИП - лб/пз/лк или лабараторная/практическое/лекция.
Пример:
` + "```" + `
/set 12 12 18
1. 507 "Іноземна мова" пз "Миколайчук А.І. "
1. 326 "Програмування C+" лб "Золотухіна О.А. "

` + "```"

func reportError(e error, replyToTgt *tgbotapi.Message) {
	if _, err := replyTo(replyToTgt, fmt.Sprintf("*Что-то сломалось*\n```\n%s\n```", e)); err != nil{
		log.Println("ERROR:", err)
	}
}

func helpCmd(msg *tgbotapi.Message) error {
	_, err := replyTo(msg, helpText)
	return err
}

func adminHelpCmd(msg *tgbotapi.Message) error {
	_, err := replyTo(msg, adminhelpText)
	return err
}

func setCmd(msg *tgbotapi.Message) error {
	if !adminCheck(msg.From.ID) {
		if _, err := replyTo(msg, "У тебя нет прав это делать."); err != nil {
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
		}
	}

	lines := strings.Split(msg.Text, "\n")

	splittenFirst := strings.Split(lines[0], " ")
	if len(splittenFirst) != 2 {
		if _, err := replyTo(msg, "Использование: /set ДАТА РАСПИСАНИЕ. Напр. /set 12.09.19 ...."); err != nil {
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
		}
		return nil
	}

	day, err := time.ParseInLocation("02.01.06", splittenFirst[1], timezone)
	if err != nil {
		if _, err := replyTo(msg, "Некорректный формат даты. Пример: 12.09.18."); err != nil {
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
		}
		return nil
	}

	entries := make([]Entry, len(lines)-1)
	for i, line := range lines[1:] {
		entry, err := SplitEntry(line, day)
		if err != nil {
			errMsg := "Неожиданная ошибка: " + err.Error()
			switch err {
			case ErrInvalidFormat:
				errMsg = "Некорректный формат расписания. См. /adminhelp."
			case ErrTooManyEntires:
				errMsg = "Нельзя добавить больше 4 пар."
			case ErrUnknownType:
				errMsg = "Некорректный тип пары."
			}
			if _, err := replyTo(msg, errMsg); err != nil {
				return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
			}
			return nil
		}
		entries[i] = entry
	}
	if err := db.ReplaceDay(day, entries,false); err != nil {
		reportError(err, msg)
		return err
	}
	if _, err := replyTo(msg, "Расписание на " + day.Format("_2 January 2006") + " задано."); err != nil {
		return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
	}
	return nil
}

func clearCmd(msg *tgbotapi.Message) error {
	if !adminCheck(msg.From.ID) {
		if _, err := replyTo(msg, "У тебя нет прав это делать."); err != nil {
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
		}
	}

	splitten := strings.Split(msg.Text, " ")
	if len(splitten) != 2 {
		if _, err := replyTo(msg, "Использование: /clear ДАТА. Напр. /clear 12.09.19."); err != nil {
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
		}
		return nil
	}

	day, err := time.ParseInLocation("02.01.06", splitten[1], timezone)
	if err != nil {
		if _, err := replyTo(msg, "Некорректный формат даты. Пример: 12.09.18."); err != nil {
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
		}
		return nil
	}

	if err := db.ClearDay(day); err != nil {
		reportError(err, msg)
	}
	if _, err := replyTo(msg, "Расписание на " + day.Format("_2 January 2006") + " удалено."); err != nil {
		return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
	}
	return nil
}

func formatTimetable(date time.Time, entries []Entry) string {
	hdr := "*Расписание на " + date.Format("_2 January 2006") + "*\n\n"
	entriesStr := make([]string, len(entries))
	for i, entry := range entries {
		ttindx := ttindex(TimeSlot{entry.Time.Hour(), entry.Time.Minute()})

		entryStr := fmt.Sprintf("*%d. Аудитория %s - %s*\n%d:%d - %d:%d, %s, %s",
			ttindx, entry.Classroom, entry.Name,
			entry.Time.Hour(), entry.Time.Minute(),
			timetableEnd[ttindx-1].Hour, timetableEnd[ttindx-1].Minute,
			typeStr[entry.Type], entry.Lecturer)
		entriesStr[i] = entryStr
	}
	if len(entriesStr) == 0 {
		entriesStr = append(entriesStr, "_пусто_")
	}
	return hdr + strings.Join(entriesStr, "\n\n")
}

func scheduleCmd(msg *tgbotapi.Message) error {
	splitten := strings.Split(msg.Text, " ")
	if len(splitten) != 2 {
		if _, err := replyTo(msg, "Использование: /schedule ДАТА; Напр. /schedule 12.09.18."); err != nil {
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
		}
		return nil
	}

	day, err := time.ParseInLocation("02.01.06", splitten[1], timezone)
	if err != nil {
		if _, err := replyTo(msg, "Некорректный формат даты. Пример: 12.09.18."); err != nil {
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
		}
		return nil
	}

	entries, err := db.OnDay(day)
	if err != nil {
		reportError(err, msg)
		return err
	}

	if _, err := replyTo(msg, formatTimetable(day, entries)); err != nil {
		return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
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

	if _, err := replyTo(msg, formatTimetable(now, entries)); err != nil {
		return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
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

	if _, err := replyTo(msg, formatTimetable(tomorrow, entries)); err != nil {
		return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
	}
	return nil
}

func nextCmd(msg *tgbotapi.Message) error {
	now := time.Now().In(timezone)

	var entry *Entry
	for _, slot := range timetableBegin {
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
		if _, err := replyTo(msg, "Сегодня больше нет пар."); err != nil {
			return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
		}
		return nil
	}

	ttindx := ttindex(TimeSlot{entry.Time.Hour(), entry.Time.Minute()})
	entryStr := fmt.Sprintf("*%d. Аудитория %s - %s\n%d:%d - %d:%d, %s, %s\n",
		ttindx, entry.Classroom, entry.Name,
		entry.Time.Hour(), entry.Time.Minute(),
		timetableEnd[ttindx-1].Hour, timetableEnd[ttindx-1].Minute,
		typeStr[entry.Type], entry.Lecturer)

	if _, err := replyTo(msg, entryStr); err != nil {
		return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d", msg.Chat.ID, msg.MessageID)
	}
	return nil
}

func timetableCmd(msg *tgbotapi.Message) error {
	res := make([]string, len(timetableBegin))
	for i := 0; i < len(timetableBegin); i++ {
		res[i] = fmt.Sprintf("%d. %d:%d - %d:%d, перерыв в %d:%d.", i+1,
			timetableBegin[i].Hour, timetableBegin[i].Minute,
			timetableEnd[i].Hour, timetableEnd[i].Minute,
			timetableBreak[i].Hour, timetableBreak[i].Hour)
	}
	if _, err := replyTo(msg, strings.Join(res, "\n")); err != nil {
		return errors.Wrapf(err, "replyTo chatid=%d, msgid=%d: %v", msg.Chat.ID, msg.MessageID, err)
	}
	return nil
}