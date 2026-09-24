package main

import (
	"fmt"
	"go-rag/internal/configs"
	"go-rag/internal/infrastructure"
	"os"
	"runtime"
)

func init() {
	loadEnv()
	runtime.GOMAXPROCS(runtime.NumCPU())
}

func main() {
	cfg, err := configs.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	if len(os.Args) > 1 && os.Args[1] == "embed-catalogs" {
		err = runEmbedCatalogs(cfg)
	} else {
		err = run(cfg)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "app run: %v\n", err)
		os.Exit(1)
	}

}

func run(cfg *configs.Config) error {
	app, err := infrastructure.NewApp(cfg, cfg.App.IsProduction())
	if err != nil {
		return fmt.Errorf("Application Initializing : %w", err)
	}
	return app.Run()
}
