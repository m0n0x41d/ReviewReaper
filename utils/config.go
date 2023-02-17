package utils

import (
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	WatchNamespaces        []string
	RetentionTime          int
	MaintenanceWindowStart time.Time
	MaintenanceWindowEnd   time.Time
	DeletionBatch          int
	SleepSeconds           int
	isRemoveByRelease      bool
	DeletionAnnotationKey  string
}

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

	config.WatchNamespaces = viper.GetStringSlice("namespaces")
	config.RetentionTime = viper.GetInt("retention")
	config.DeletionAnnotationKey = viper.GetString("annotation_key")
	return config, nil
}
