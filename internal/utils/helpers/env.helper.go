package helpers

import (
	"os"
	"strconv"
	"strings"
)

func GetEnvAsInt(name string) int {
	if valueStr := os.Getenv(name); valueStr != "" {
		if value, err := strconv.Atoi(valueStr); err == nil {
			return value
		}
	}

	return 30
}

func GetEnvAsIntDefault(name string, defaultV int) int {
	if valueStr := os.Getenv(name); valueStr != "" {
		if value, err := strconv.Atoi(valueStr); err == nil {
			return value
		}
	}

	return defaultV
}

func GetEnvAsBool(name string, defaultV bool) bool {
	if valueStr := os.Getenv(name); valueStr != "" {
		return strings.ToLower(valueStr) == "true"
	}

	return defaultV
}
