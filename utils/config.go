package utils

import (
	"fmt"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/util/validation"
)

var (
	validweekdays = []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
)

// TODO: Check if rest of the fields also can be validated. It's probably worth implementing a custom validation function and removing the validator.
type Config struct {
	NamespacePrefixes  []string `validate:"required"`
	RetentionDays      int      `validate:"gte=0"`
	RetentionHours     int      `validate:"gte=0"`
	DeletionBatchSize  int      `validate:"gte=0"`
	DeletionNapSeconds int      `validate:"gte=0"`
	IsDeleteByRelease  bool
	PostponeDeletion   bool
	AnnotationKey      string
	DeletionWindow     struct {
		NotBefore string
		NotAfter  string
		WeekDays  []string
	}

	LogLevel string
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
	viper.SetDefault("deletion_batch_size", 0)
	viper.SetDefault("deletion_nap_seconds", 0)
	viper.SetDefault("delete_by_release", false)
	viper.SetDefault("deletion_windows.not_before", "00:00")
	viper.SetDefault("deletion_windows.not_after", "06:00")
	viper.SetDefault("deletion_windows.week_days", validweekdays)
	viper.SetDefault("postpone_deletion", false)
	viper.SetDefault("annotation_key", "delete_after")
	viper.SetDefault("postpone_deletion_if_active", false)
	viper.SetDefault("log_level", "INFO")

	config.NamespacePrefixes = viper.GetStringSlice("namespace_prefixes")
	config.RetentionDays = viper.GetInt("retention.days")
	config.RetentionHours = viper.GetInt("retention.hours")

	config.DeletionBatchSize = viper.GetInt("deletion_batch_size")
	config.DeletionNapSeconds = viper.GetInt("deletion_nap_seconds")

	config.IsDeleteByRelease = viper.GetBool("delete_by_release")
	config.AnnotationKey = viper.GetString("annotation_key")

	config.DeletionWindow.NotBefore = viper.GetString("deletion_windows.not_before")
	config.DeletionWindow.NotAfter = viper.GetString("deletion_windows.not_after")
	config.DeletionWindow.WeekDays = viper.GetStringSlice("deletion_windows.week_days")
	config.PostponeDeletion = viper.GetBool("postpone_deletion_if_active")

	config.LogLevel = viper.GetString("log_level")

	// safeChecks
	err = validate.Struct(config)
	if err != nil {
		return Config{}, err
	}

	// validateConfig(config)
	return config, nil
}

func validateConfig(c Config) (err error) {
	err = validatePrefixes(c.NamespacePrefixes)

	return nil
}

func validatePrefixes(s []string) error {

	for _, prefix := range s {
		errs := validation.IsDNS1123Label(s[0])
		if len(errs) > 0 {
			return fmt.Errorf("namespace prefix '%s' not a lowercase RFC 1123 label", prefix)
		}
	}
	return nil
}
