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

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "embed-catalogs":
			err = runEmbedCatalogs(cfg)
		case "embed-docs":
			err = runEmbedDocs(cfg, os.Args[2:])
		case "chat":
			err = runChatCLI(cfg, os.Args[2:], os.Stdin, os.Stdout)
		default:
			err = fmt.Errorf("unknown command %q (use chat, embed-catalogs, or embed-docs)", os.Args[1])
		}
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
