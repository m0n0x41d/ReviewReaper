package logs

import (
	"NaNameUz3r/ReviewReaper/utils"

	"github.com/hashicorp/go-hclog"
)

type Logger hclog.Logger

func NewLogger(appConfig utils.Config) hclog.Logger {
	Logger := hclog.New(&hclog.LoggerOptions{
		Name:  "NsInformer",
		Level: hclog.LevelFromString(appConfig.LogLevel),
	})

	return Logger
}
