package logs

import (
	"NaNameUz3r/ReviewReaper/utils"
	"fmt"
	"reflect"

	"github.com/hashicorp/go-hclog"
)

type Logger hclog.Logger

func NewLogger(appConfig utils.Config) hclog.Logger {
	Logger := hclog.New(&hclog.LoggerOptions{
		Name:  "NamespaceController",
		Level: hclog.LevelFromString(appConfig.LogLevel),
	})

	return Logger
}

func StartUp(appConfig utils.Config, logger hclog.Logger) {
	logger.Info(printConfig(appConfig))
}

func printConfig(s interface{}) string {
	hiddenFields := []string{"DeletionRegexp"}
	structType := reflect.TypeOf(s)
	structValue := reflect.ValueOf(s)

	config := fmt.Sprintf("\n\n%v:\n", "Loaded config")
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		value := structValue.Field(i)
		if utils.IsContains(hiddenFields, field.Name) {
			continue
		}
		config += fmt.Sprintf("\t%s: %v\n", field.Name, value.Interface())
	}

	return config
}
