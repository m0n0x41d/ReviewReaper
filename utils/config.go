package utils

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

var (
	defaultWeekDays      = []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	NsPreserveAnnotation = "review-reaper-protected"
)

// TODO: Check if rest of the fields also can be validated. It's probably worth implementing a custom validation function and removing the validator.
// TODO: Add ignored_namespaces parameter to preserve some namespaces, like ReviewReaper on its own, if it deployed by helm release and namespace named reviewreaper, fxmpl xDDD
type Config struct {
	NsNameDeletionRegexp string `validate:"required"`
	DeletionRegexp       *regexp.Regexp
	RetentionDays        int `validate:"gte=0"`
	RetentionHours       int `validate:"gte=0"`
	DeletionBatchSize    int `validate:"gte=0"`
	DeletionNapSeconds   int `validate:"gte=0"`
	IsUninstallReleases  bool
	PostponeDeletion     bool
	AnnotationKey        string
	NsPreserveAnnotation string
	DeletionWindow       struct {
		NotBefore string
		NotAfter  string
		WeekDays  []string
	}

	LogLevel string
	DryRun   bool
}

var validate = validator.New()

func LoadConfig() (config Config, err error) {

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath("/etc/app/")
	viper.AddConfigPath("/app")
	viper.AddConfigPath(".")
	err = viper.ReadInConfig()
	if err != nil {
		return Config{}, err
	}

	viper.SetDefault("retention.days", 7)
	viper.SetDefault("retention.hours", 0)
	viper.SetDefault("DeletionBatchSize", 0)
	viper.SetDefault("DeletionNapSeconds", 0)
	viper.SetDefault("IsUninstallReleases", false)
	viper.SetDefault("DeletionWindow.NotBefore", "00:00")
	viper.SetDefault("DeletionWindow.NotAfter", "06:00")
	viper.SetDefault("DeletionWindow.WeekDays", defaultWeekDays)
	viper.SetDefault("AnnotationKey", "delete_after")
	viper.SetDefault("PostoneNsDeletionByHelmDeploy", false)
	viper.SetDefault("LogLevel", "INFO")
	viper.SetDefault("DryRun", false)
	config.NsPreserveAnnotation = NsPreserveAnnotation

	config.NsNameDeletionRegexp = viper.GetString("NsNameDeletionRegexp")
	config.RetentionDays = viper.GetInt("Retention.Days")
	config.RetentionHours = viper.GetInt("Retention.Hours")

	config.DeletionBatchSize = viper.GetInt("DeletionBatchSize")
	config.DeletionNapSeconds = viper.GetInt("DeletionNapSeconds")

	config.IsUninstallReleases = viper.GetBool("IsUninstallReleases")
	config.AnnotationKey = viper.GetString("AnnotationKey")

	config.DeletionWindow.NotBefore = viper.GetString("DeletionWindow.NotBefore")
	config.DeletionWindow.NotAfter = viper.GetString("DeletionWindow.NotAfter")
	config.DeletionWindow.WeekDays = viper.GetStringSlice("DeletionWindow.WeekDays")
	config.PostponeDeletion = viper.GetBool("PostoneNsDeletionByHelmDeploy")

	config.LogLevel = viper.GetString("LogLevel")
	config.DryRun = viper.GetBool("DryRun")

	// safeChecks
	err = validate.Struct(config)
	if err != nil {
		return Config{}, err
	}

	config.DeletionRegexp, err = regexp.Compile(config.NsNameDeletionRegexp)
	if err != nil {
		return Config{}, errors.New("Unable to compile regexp")
	}

	return config, validateConfig(config)
}

func validateConfig(c Config) (err error) {
	validationFuncs := []func(Config) error{
		validateWeekDays,
	}

	for _, f := range validationFuncs {
		if err := f(c); err != nil {
			return err
		}
	}

	return nil
}

func validateWeekDays(c Config) error {
	err := fmt.Errorf("Invalid weekdays in config DeletionWindow.WeekDays")
	validWeekdays := map[string]bool{
		"Mon": true,
		"Tue": true,
		"Wed": true,
		"Thu": true,
		"Fri": true,
		"Sat": true,
		"Sun": true,
	}

	for _, day := range c.DeletionWindow.WeekDays {
		if len(day) != 3 || !validWeekdays[day] {
			return err
		}
	}

	return nil
}
