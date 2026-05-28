package config

import "os"

type Config struct {
	Port string
}

func Load() *Config {
	cfg := &Config{Port: "8080"}
	if port := os.Getenv("HTTP_PORT"); port != "" {
		cfg.Port = port
	}
	return cfg
}
