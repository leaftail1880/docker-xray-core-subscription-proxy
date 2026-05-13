package logger

import (
	"log"
	"os"
)

var (
	Info  *log.Logger
	Warn  *log.Logger
	Error *log.Logger
	Debug *log.Logger
)

const (
	reset  = "\033[0m"
	Red    = "\033[31m"
	Green  = "\033[32m"
	Yellow = "\033[33m"
	Cyan   = "\033[36m"
)

func init() {
	flags := log.Ldate | log.Ltime | log.Lshortfile
	Info = log.New(os.Stdout, Green+"[INFO] "+reset, flags)
	Warn = log.New(os.Stdout, Yellow+"[WARN] "+reset, flags)
	Error = log.New(os.Stderr, Red+"[ERROR] "+reset, flags)
	Debug = log.New(os.Stdout, Cyan+"[DEBUG] "+reset, flags)
}
