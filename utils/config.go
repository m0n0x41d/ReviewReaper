package utils

import (
	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
)

type Config struct {
	NamespacePrefixes  []string
	RetentionDays      int `validate:"gte=0"`
	RetentionHours     int `validate:"gte=0"`
	DeletionBatchSize  int `validate:"gte=0"`
	DeletionNapSeconds int `validate:"gte=0"`
	IsDeleteByRelease  bool
	AnnotationKey      string
	DeletionWindow     struct {
		NotBefore string
		NotAfter  string
		WeekDays  []string
	}
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
	viper.SetDefault("deletion_windows.not_before", "05:00")
	viper.SetDefault("deletion_windows.not_after", "07:00")
	viper.SetDefault("deletion_windows.week_days", []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"})
	viper.SetDefault("annotation_key", "delete_after")

	config.NamespacePrefixes = viper.GetStringSlice("namespace_prefixes")
	config.RetentionDays = viper.GetInt("retention.days")
	config.RetentionHours = viper.GetInt("retention.hours")

	config.IsDeleteByRelease = viper.GetBool("delete_by_release")
	config.AnnotationKey = viper.GetString("annotation_key")

	config.DeletionWindow.NotBefore = viper.GetString("deletion_windows.not_before")
	config.DeletionWindow.NotAfter = viper.GetString("deletion_windows.not_after")
	config.DeletionWindow.WeekDays = viper.GetStringSlice("deletion_windows.week_days")

	// safeChecks

	err = validate.Struct(config)
	if err != nil {
		return Config{}, err
	}

	return config, nil
}
