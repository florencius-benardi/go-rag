//###### go:build prod || staging

package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// localEnvFile is the per-installation override file. It sits next to the
// binary (or in the working directory) and is NOT embedded, so an operator can
// edit it on site without rebuilding.
const localEnvFile = ".env.local"

var customerOverrideKeys = map[string]bool{
	"GIN_PORT":                false,
	"GIN_URL":                 false,
	"DB_CONNECTION":           false,
	"DB_HOST":                 false,
	"DB_PORT":                 false,
	"DB_DATABASE":             false,
	"DB_USERNAME":             false,
	"DB_PASSWORD":             true,
	"EMBEDDING_BASE_URL":      false,
	"EMBEDDING_PROVIDER":      false,
	"EMBEDDING_API_KEY":       true,
	"EMBEDDING_MODEL":         false,
	"EMBEDDING_CHUNK_SIZE":    false,
	"EMBEDDING_CHUNK_OVERLAP": false,
	"EMBEDDING_SYNC_DEBUG":    false,
	"LLM_BASE_URL":            false,
	"LLM_API_KEY":             true,
	"LLM_MODEL":               false,
}

// applyLocalOverride reads .env.local and overwrites only the whitelisted keys
// that were already loaded from the embedded .env. Missing file is not an
// error — it is the normal case for a default installation.
func applyLocalOverride() {
	path, ok := resolveLocalEnvPath()
	if !ok {
		return
	}

	envMap, err := godotenv.Read(path)
	if err != nil {
		log.Printf("Warning: failed to read %s: %v (using embedded config)", path, err)
		return
	}

	applied := make([]string, 0, len(envMap))
	for key, val := range envMap {
		allowEmpty, whitelisted := customerOverrideKeys[key]
		if !whitelisted {
			log.Printf("Warning: %s key %q is not overridable, ignored", localEnvFile, key)
			continue
		}
		if val == "" && !allowEmpty {
			continue
		}
		os.Setenv(key, val)
		applied = append(applied, key)
	}

	if len(applied) > 0 {
		log.Printf("Loaded local override from %s: %v", path, applied)
	}
}

// resolveLocalEnvPath looks for .env.local next to the executable first, then
// in the current working directory. The exe folder takes priority because
// under the Windows SCM the working directory is C:\Windows\System32 at the
// time loadEnv() runs — chdirToExe() (see service.go) only happens later.
func resolveLocalEnvPath() (string, bool) {
	candidates := make([]string, 0, 2)

	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		candidates = append(candidates, filepath.Join(filepath.Dir(exe), localEnvFile))
	}
	candidates = append(candidates, localEnvFile)

	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, true
		}
	}
	return "", false
}
