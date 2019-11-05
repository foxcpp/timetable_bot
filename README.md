# timetable_bot

Simple Telegram bot for university timetable-related notifications. 
Initially written for one university without extensbility in mind so you
are most likely need to modify it if you want to use it.

### Installation

Build using regular Go tools.
Populate config (see below).

### Configuration

botconf.yml should exist in current working directory.
[Documented example](botconf.example.yml) is included in repo.
You may specify the bot token via `TT_TOKEN` environment variable -
this overrides the config value.

### Auto-update

Bot can automatically download and update timetable for next week,
however you need to replace timetableparser package for this. This repo
contains implementation for DUT university (it downloads timetable
from http://e-rozklad.dut.edu.ua/timeTable/group).
