package utils

import (
	"os"

	"github.com/sirupsen/logrus"
)

var Logger *logrus.Logger

func init() {
	Logger = logrus.New()
	Logger.Formatter = new(logrus.JSONFormatter)
	Logger.Formatter.(*logrus.TextFormatter).DisableColors = false
	Logger.Formatter.(*logrus.TextFormatter).DisableTimestamp = false
	Logger.Level = logrus.TraceLevel
	Logger.Out = os.Stdout
}

// func WithField(strings ...string) {

// }
