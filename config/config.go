package config

import (
	"fmt"
	"github.com/go-playground/validator/v10"
	"github.com/spf13/viper"
	"log"
	"os"
)

type Config struct {
	Proxying Proxying `mapstructure:"proxying" validate:"required"`
	Logging  Logging  `mapstructure:"logging" validate:"required"`
	Dimming  Dimming  `mapstructure:"dimming" validate:"required"`
}

type Proxying struct {
	FrontendPort *int    `mapstructure:"frontendPort" validate:"required"`
	BackendHost  *string `mapstructure:"backendHost" validate:"required"`
	BackendPort  *int    `mapstructure:"backendPort" validate:"required"`
}

type Logging struct {
	Driver   *string  `mapstructure:"driver" validate:"oneof=noop stdout influxdb"`
	InfluxDB InfluxDB `mapstructure:"influxdb" validate:"required_if=Driver influxdb"`
}

type InfluxDB struct {
	Host   *string `mapstructure:"host" validate:"required"`
	Token  *string `mapstructure:"token" validate:"required"`
	Org    *string `mapstructure:"org" validate:"required"`
	Bucket *string `mapstructure:"bucket" validate:"required"`
}

type Dimming struct {
	Enabled            *bool               `mapstructure:"enabled" validate:"required"`
	DimmableComponents []DimmableComponent `mapstructure:"dimmableComponents" validate:"required"`
	Controller         Controller          `mapstructure:"controller" validate:"required"`
	Profiler           Profiler            `mapstructure:"profiler" validate:"required"`
}

type DimmableComponent struct {
	Method MatchableMethod `mapstructure:"method" validate:"required"`
	Path   *string         `mapstructure:"path" validate:"required"`
	// Probability is a pointer as probabilities will be set to an external
	// default if it is nil.
	Probability *float64     `mapstructure:"probability"`
	Exclusions  []Exclusions `mapstructure:"exclusions"`
}

type MatchableMethod struct {
	ShouldMatchAll *bool `mapstructure:"shouldMatchAll" validate:"required_without=Method"`
	// Method must be set if ShouldMatchAll is false. If ShouldMatchAll is true,
	// Method is ignored.
	Method *string `mapstructure:"method" validate:"required_without=ShouldMatchAll required_if=ShouldMatchAll false"`
}

type Exclusions struct {
	Method    *string `mapstructure:"method" validate:"required"`
	Substring *string `mapstructure:"substring" validate:"required"`
}

type Controller struct {
	SamplePeriod *float64 `mapstructure:"samplePeriod" validate:"required"`
	Percentile   *string  `mapstructure:"percentile" validate:"oneof=p50 p75 p95"`
	Setpoint     *float64 `mapstructure:"setpoint" validate:"required"`
	Kp           *float64 `mapstructure:"kp" validate:"required"`
	Ki           *float64 `mapstructure:"ki" validate:"required"`
	Kd           *float64 `mapstructure:"kd" validate:"required"`
}

type Profiler struct {
	Enabled       *bool         `mapstructure:"enabled" validate:"required"`
	SessionCookie *string       `mapstructure:"sessionCookie" validate:"required"`
	InfluxDB      InfluxDB      `mapstructure:"influxdb" validate:"required"`
	Redis         Redis         `mapstructure:"redis" validate:"required"`
	Probabilities Probabilities `mapstructure:"probabilities" validate:"required"`
}

type Redis struct {
	Addr         *string `mapstructure:"addr" validate:"required"`
	Password     *string `mapstructure:"password" validate:"required"`
	PrioritiesDB *int    `mapstructure:"prioritiesDB" validate:"required"`
	QueueDB      *int    `mapstructure:"queueDB" validate:"required"`
}

type Probabilities struct {
	High           *float64 `mapstructure:"high" validate:"required"`
	HighMultiplier *float64 `mapstructure:"highMultiplier" validate:"required"`
	Low            *float64 `mapstructure:"low" validate:"required"`
	LowMultiplier  *float64 `mapstructure:"lowMultiplier" validate:"required"`
}

func setDefaults() {
	viper.SetDefault("Proxying.BackendHost", "localhost")
	viper.SetDefault("Logging.Driver", "noop")

	viper.SetDefault("Dimming.Controller.SamplePeriod", 1)
	viper.SetDefault("Dimming.Controller.Percentile", "p95")
	viper.SetDefault("Dimming.Controller.Setpoint", 3)
	viper.SetDefault("Dimming.Controller.Kp", 2)
	viper.SetDefault("Dimming.Controller.Ki", 0.2)
	viper.SetDefault("Dimming.Controller.Kd", 0)

	viper.SetDefault("Dimming.Profiler.Enabled", false)
	viper.SetDefault("Dimming.Profiler.Probabilities.High", 0.01)
	viper.SetDefault("Dimming.Profiler.Probabilities.HighMultiplier", 1)
	viper.SetDefault("Dimming.Profiler.Probabilities.Low", 0.99)
	viper.SetDefault("Dimming.Profiler.Probabilities.LowMultiplier", 1)
}

func ReadConfig() *Config {
	viper.AutomaticEnv()
	setDefaults()

	viper.SetConfigType("yaml")
	viper.SetConfigName("config")
	viper.AddConfigPath(".")
	viper.AddConfigPath("/app")
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Fatalf("error: /app/config.yaml not found. Are you sure you have configured the ConfigMap?\nerr = %s", err)
		} else {
			log.Fatalf("error when reading config file at /app/config.yaml: err = %s", err)
		}
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		log.Fatalf("error occured while reading configuration file: err = %s", err)
	}
	validate := validator.New()
	err := validate.Struct(&config)
	if err != nil {
		if _, ok := err.(*validator.InvalidValidationError); ok {
			log.Printf("unable to validate config: err = %s", err)
		}

		log.Printf("encountered validation errors:\n")

		for _, err := range err.(validator.ValidationErrors) {
			fmt.Printf("\t%s\n", err.Error())
		}

		fmt.Println("Check your configuration file and try again.")
		os.Exit(1)
	}

	return &config
}
