# timetable_bot

Simple Telegram bot for university timetable-related notifications. 
Initially written for one university without extensbility in mind so you
are most likely need to modify it if you want to use it.

## Installation

Build using regular Go tools.
Populate config (see below).

## Configuration

botconf.yml should exist in current working directory.
[Documented example](botconf.example.yml) is included in repo.
You may specify the bot token via `TT_TOKEN` environment variable -
this overrides the config value.

### Webhook

This bot supports a webhook-based updates delivery. To use it, make sure
the `webhook` config variable is set, as well as `port` for binding the
HTTP server. You may also set those via environment using `TT_WEBHOOK` for
URL and `TT_PORT` for the server port.

## Auto-update

Bot can automatically download and update timetable for next week,
however you need to replace timetableparser package for this. This repo
contains implementation for DUT university (it downloads timetable
from http://e-rozklad.dut.edu.ua/timeTable/group).
