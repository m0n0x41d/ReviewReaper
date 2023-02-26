package utils

import (
	"github.com/spf13/viper"
)

type Config struct {
	WatchNamespaces []string
	RetentionDays   int
	RetentionHours  int

	MaintenanceWindow struct {
		NotBefore string
		NotAfter  string
		WeekDays  []string
	}

	DeletionBatch         int
	SleepSeconds          int
	isRemoveByRelease     bool
	DeletionAnnotationKey string
}

var MaintenanceWindow map[string]interface{}

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
	viper.SetDefault("deletion_windows.not_before", "05:00")
	viper.SetDefault("deletion_windows.not_after", "07:00")
	viper.SetDefault("deletion_windows.week_days", []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"})
	viper.SetDefault("annotation_key", "delete_after")

	config.WatchNamespaces = viper.GetStringSlice("namespaces")
	config.RetentionDays = viper.GetInt("retention.days")
	config.DeletionAnnotationKey = viper.GetString("annotation_key")

	config.MaintenanceWindow.NotBefore = viper.GetString("deletion_windows.not_before")
	config.MaintenanceWindow.NotAfter = viper.GetString("deletion_windows.not_after")
	config.MaintenanceWindow.WeekDays = viper.GetStringSlice("deletion_windows.week_days")

	return config, nil
}
