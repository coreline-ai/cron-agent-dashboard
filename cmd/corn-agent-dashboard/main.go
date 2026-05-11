package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/coreline-ai/cron-agent-dashboard/internal/config"
	"github.com/coreline-ai/cron-agent-dashboard/internal/db"
	"github.com/coreline-ai/cron-agent-dashboard/internal/httpapi"
	"github.com/coreline-ai/cron-agent-dashboard/internal/store"
)

func main() {
	cmd := "serve"
	args := os.Args[1:]
	if len(args) > 0 && args[0] != "--help" && args[0] != "-h" && args[0][0] != '-' {
		cmd = args[0]
		args = args[1:]
	}
	cfg, _, err := config.Load(args)
	if err != nil {
		log.Fatal(err)
	}
	if err := config.EnsureDirs(cfg); err != nil {
		log.Fatal(err)
	}
	database, err := db.OpenAndMigrate(cfg.DBPath)
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()
	st := store.New(database)
	if _, err := st.RecoverOrphanRuns(ctx()); err != nil {
		log.Fatal(err)
	}
	switch cmd {
	case "init":
		fmt.Printf("initialized %s\n", cfg.DBPath)
	case "serve":
		log.Printf("corn-agent-dashboard listening on http://%s", cfg.Bind)
		if err := http.ListenAndServe(cfg.Bind, httpapi.New(st, cfg)); err != nil {
			log.Fatal(err)
		}
	default:
		log.Fatalf("unknown command %q (expected serve or init)", cmd)
	}
}

func ctx() context.Context { return context.Background() }
