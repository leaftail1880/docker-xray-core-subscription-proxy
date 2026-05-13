package logger

import (
	"log"
	"os"
)

var (
	Info  = log.New(os.Stdout, colorGreen("[INFO] "), log.Ldate|log.Ltime|log.Lshortfile)
	Warn  = log.New(os.Stdout, colorYellow("[WARN] "), log.Ldate|log.Ltime|log.Lshortfile)
	Error = log.New(os.Stderr, colorRed("[ERROR] "), log.Ldate|log.Ltime|log.Lshortfile)
	Debug = log.New(os.Stdout, colorCyan("[DEBUG] "), log.Ldate|log.Ltime|log.Lshortfile)
)

const (
	reset  = "\033[0m"
	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	cyan   = "\033[36m"
)

func colorRed(s string) string    { return red + s + reset }
func colorGreen(s string) string  { return green + s + reset }
func colorYellow(s string) string { return yellow + s + reset }
func colorCyan(s string) string   { return cyan + s + reset }
