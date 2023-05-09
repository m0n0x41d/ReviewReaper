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
	logger.Info("Verifying connection and attempting to initiate reconciliation loop...")
}

func printConfig(s interface{}) string {
	hiddenFields := []string{"DeletionRegexp"}
	structValue := reflect.ValueOf(s)

	config := fmt.Sprintf("\n\n%v:\n", "Loaded config")
	printFields(structValue, hiddenFields, "\t", &config)

	config += "\nThe config options is described in the repository's README.md\n"
	return config
}

func printFields(value reflect.Value, hiddenFields []string, indent string, config *string) {
	structType := value.Type()
	for i := 0; i < structType.NumField(); i++ {
		field := structType.Field(i)
		if utils.IsContains(hiddenFields, field.Name) {
			continue
		}
		fieldValue := value.Field(i)
		switch fieldValue.Kind() {
		case reflect.Struct:
			*config += fmt.Sprintf("%s%s:\n", indent, field.Name)
			printFields(fieldValue, hiddenFields, indent+"\t", config)
		default:
			*config += fmt.Sprintf("%s%s: %v\n", indent, field.Name, fieldValue.Interface())
		}
	}
}
