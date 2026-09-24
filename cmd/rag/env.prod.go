// ### go:build prod

package main

import (
	_ "embed"
	"log"
	"os"

	"github.com/joho/godotenv"
)

//go:embed .env
var embedEnv string

func loadEnv() {
	envMap, err := godotenv.Unmarshal(embedEnv)
	if err != nil {
		log.Fatalf("Failed to parse embedded .env: %v", err)
	}
	for k, v := range envMap {
		os.Setenv(k, v)
	}

	// Per-installation overrides (.env.local) win over the embedded defaults.
	applyLocalOverride()
}
