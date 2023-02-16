package logs

import (
	"os"

	"github.com/sirupsen/logrus"
)

type any = interface{}
type Fields = logrus.Fields

type Logger interface {
	WithFields(fields Fields) Logger
	WithError(err error) Logger
	WithField(key string, arg any) Logger
	Error(args ...any)
	Info(args ...any)
	Fatal(args ...any)
}

type logger struct {
	loggerEntry *logrus.Entry
}

func NewLogger() Logger {
	logrusLogger := logrus.New()
	logrusLogger.SetFormatter(&logrus.JSONFormatter{})
	// logrusLogger.Formatter.(*logrus.TextFormatter).DisableColors = false
	logrusLogger.SetLevel(logrus.TraceLevel)
	logrusLogger.SetOutput(os.Stdout)
	return newLogger(logrus.NewEntry(logrusLogger))
}

func newLogger(entry *logrus.Entry) Logger {
	return &logger{
		loggerEntry: entry,
	}
}

func (l *logger) WithFields(fields Fields) Logger {
	return newLogger(l.loggerEntry.WithFields(fields))
}

func (l *logger) WithField(key string, arg any) Logger {
	return newLogger(l.loggerEntry.WithField(key, arg))
}

func (l *logger) WithError(err error) Logger {
	return newLogger(l.loggerEntry.WithError(err))
}

func (l *logger) Error(args ...any) {
	l.loggerEntry.Errorln(args...)
}

func (l *logger) Info(args ...any) {
	l.loggerEntry.Info(args...)
}

func (l *logger) Fatal(args ...any) {
	l.loggerEntry.Fatalln(args...)
}
