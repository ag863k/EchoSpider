package main

import (
	"encoding/json"
	"os"
)

type Config struct {
	MaxDepth      int    `json:"maxDepth"`
	MaxWorkers    int    `json:"maxWorkers"`
	TimeoutSec    int    `json:"timeoutSec"`
	RespectRobots bool   `json:"respectRobots"`
	UserAgent     string `json:"userAgent"`
	OutputFormat  string `json:"outputFormat"`
}

func LoadConfig(filename string) (*Config, error) {
	config := &Config{
		MaxDepth:      2,
		MaxWorkers:    10,
		TimeoutSec:    10,
		RespectRobots: false,
		UserAgent:     "EchoSpider/1.0",
		OutputFormat:  "console",
	}

	if _, err := os.Stat(filename); os.IsNotExist(err) {
		return config, nil
	}

	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	err = decoder.Decode(config)
	if err != nil {
		return nil, err
	}

	return config, nil
}

func (c *Config) Save(filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(c)
}
