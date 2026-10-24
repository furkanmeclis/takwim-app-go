// Command demo-seed writes the demo businesses of the six sectors with
// branches, staff, services, packages, products, resources, past and
// upcoming visits, and their ready-to-use accounts (TEC-928). See
// docs/design/demo-data.md.
//
// Usage (repo root, loads .env):
//
//	make demo-seed                       # random password, printed once
//	make demo-seed DEMO_PASSWORD='...'   # or DEMO_PASSWORD in the env
//	make demo-seed DEMO_RESET=1          # delete the demo businesses first
//
// It refuses to run with APP_ENV=production unless -allow-production is
// given.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/tabwriter"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/demo"
)

func main() {
	reset := flag.Bool("reset", false, "delete the existing demo businesses first")
	pass := flag.String("password", "", "password of every demo account (default: $DEMO_PASSWORD, else generated)")
	history := flag.Int("history-days", demo.DefaultHistoryDays, "days of past visits")
	future := flag.Int("future-days", demo.DefaultFutureDays, "days of upcoming bookings")
	only := flag.String("only", "", "comma-separated business keys ("+strings.Join(demo.Keys(), ", ")+")")
	allowProd := flag.Bool("allow-production", false, "run even with APP_ENV=production")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		fatal("config: %v", err)
	}
	if cfg.App.Env == "production" && !*allowProd {
		fatal("APP_ENV is production; demo data is for local, staging and pilot environments (pass -allow-production to override)")
	}
	password := *pass
	if password == "" {
		password = os.Getenv("DEMO_PASSWORD")
	}
	var keys []string
	for _, k := range strings.Split(*only, ",") {
		if k = strings.TrimSpace(k); k != "" {
			keys = append(keys, k)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		fatal("database: %v", err)
	}
	defer pool.Close()

	res, err := demo.New(pool).Run(ctx, demo.Options{
		Password: password, Reset: *reset, HistoryDays: *history, FutureDays: *future, Only: keys, Log: os.Stdout,
	})
	if err != nil {
		fatal("%v", err)
	}

	fmt.Println()
	fmt.Println("Demo businesses:")
	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "  SLUG\tNAME\tSECTOR\tRESERVATIONS\tCOMPLETED\tUPCOMING")
	for _, b := range res.Businesses {
		_, _ = fmt.Fprintf(w, "  %s\t%s\t%s\t%d\t%d\t%d\n", b.Slug, b.Name, b.Sector, b.Reservations, b.Completed, b.Upcoming)
	}
	_ = w.Flush()
	fmt.Println()
	fmt.Println("Accounts (sign in at /login):")
	w = tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "  EMAIL\tROLE\tBUSINESS\tBRANCHES")
	for _, a := range res.Accounts {
		_, _ = fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n", a.Email, a.Role, a.Business, a.Branches)
	}
	_ = w.Flush()
	fmt.Println()
	if res.GeneratedPassword {
		fmt.Printf("Password of every demo account (generated, shown once): %s\n", res.Password)
	} else {
		fmt.Println("Password of every demo account: the one you passed.")
	}
	fmt.Printf("Customer panel (/panel): phone %s; outside production the sign-in code is written to the API log.\n", res.PanelPhone)
	fmt.Println("Search index: run `make search-reindex` when Meilisearch is on.")
}

func fatal(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, "demo-seed: "+format+"\n", args...)
	os.Exit(1)
}
