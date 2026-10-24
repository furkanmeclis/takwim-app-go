package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/logging"
	branchadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/adapters"
	cashadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/adapters"
	customersadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/adapters"
	productsadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/adapters"
	reservationsadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/adapters"
	resourcesadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/resources/adapters"
	servicesadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/services/adapters"
	sessionpackagesadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sessionpackages/adapters"
	staffadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staff/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	searchadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine/adapters"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		_, _ = os.Stderr.WriteString("config load failed: " + err.Error() + "\n")
		os.Exit(1)
	}

	log := logging.New(cfg.Log.Level, cfg.Log.Format)
	if !cfg.Search.Enabled {
		log.Info("search_disabled")
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		log.Error("database_init_failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	queries := database.NewQueries(pool)
	client := searchengine.NewClient(cfg.Search, log)
	if client == nil || !client.Enabled() {
		log.Error("search_client_unavailable")
		os.Exit(1)
	}
	reg := searchengine.NewRegistry(
		searchadapters.NewUsers(queries),
		searchadapters.NewRoles(queries),
		branchadapters.NewSearch(queries),
		staffadapters.NewSearch(queries),
		resourcesadapters.NewSearch(queries),
		servicesadapters.NewSearch(queries),
		productsadapters.NewSearch(queries),
		sessionpackagesadapters.NewSearch(queries),
		customersadapters.NewSearch(queries),
		reservationsadapters.NewSearch(queries),
		cashadapters.NewSearch(queries),
	)
	indexer := searchengine.NewIndexer(client, reg, nil, log)
	if err := indexer.ProcessReindex(ctx, ""); err != nil {
		log.Error("search_reindex_failed", "error", err)
		os.Exit(1)
	}
	log.Info("search_reindex_completed")
}
