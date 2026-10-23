package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/cache"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/logging"
	branchadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/adapters"
	branchrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/repository"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	bulkusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/bulk/usecase"
	customersadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/adapters"
	customersrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/repository"
	customersusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/usecase"
	exportusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/exports/usecase"
	importusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/imports/usecase"
	integrationsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations"
	integrationjobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	logsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/logs/usecase"
	notifmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/providers"
	notifusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/notifications/usecase"
	orgadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/adapters"
	orgrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/repository"
	orgusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/organizations/usecase"
	resourcesadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/resources/adapters"
	resourcesrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/resources/repository"
	resourcesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/resources/usecase"
	servicesadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/services/adapters"
	servicesrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/services/repository"
	servicesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/services/usecase"
	staffadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staff/adapters"
	staffrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staff/repository"
	staffusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staff/usecase"
	tenantmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tenant"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/observability"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	bulkadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/events"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	ioadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/mail"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx/orgcache"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/outbox"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	searchadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/queue"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/realtime"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		_, _ = os.Stderr.WriteString("config load failed: " + err.Error() + "\n")
		os.Exit(1)
	}

	log := logging.New(cfg.Log.Level, cfg.Log.Format)
	flushSentry, err := observability.Init(cfg.Sentry, "worker")
	if err != nil {
		log.Warn("sentry_init_failed", "error", err)
	} else if observability.Enabled() {
		log.Info("sentry_enabled", "environment", cfg.Sentry.Environment, "traces_sample_rate", cfg.Sentry.TracesSampleRate)
	}
	defer flushSentry()
	// exit flushes pending error events before leaving (os.Exit skips defers).
	exit := func(code int) {
		flushSentry()
		os.Exit(code)
	}
	log = slog.New(observability.NewSlogHandler(log.Handler()))
	if !cfg.Queue.Enabled {
		log.Error("queue_disabled")
		exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		log.Error("database_init_failed", "error", err)
		exit(1)
	}
	defer pool.Close()

	store, err := storage.NewFromConfig(ctx, cfg.Storage)
	if err != nil {
		log.Error("storage_init_failed", "error", err)
		exit(1)
	}

	var publisher realtime.Publisher
	if cfg.Centrifugo.Enabled {
		publisher = realtime.NewCentrifugoClient(cfg.Centrifugo)
	} else {
		publisher = realtime.NoopPublisher{}
	}

	queries := database.NewQueries(pool)
	mailer := mail.NewSMTPSender(cfg.SMTP)
	provs := []providers.Provider{
		providers.InappProvider{},
		providers.EmailProvider{Mail: mailer},
		providers.RealtimeProvider{Pub: publisher},
		providers.NoopProvider{Name: "sms", Log: log},
		providers.NoopProvider{Name: "push", Log: log},
	}
	notifSvc := notifusecase.New(queries, nil, provs, log)
	notifSvc.WithVAPID(notifusecase.VAPIDConfig{
		PublicKey:  cfg.VAPID.PublicKey,
		PrivateKey: cfg.VAPID.PrivateKey,
		Subject:    cfg.VAPID.Subject,
	})
	notifSvc.WithExpo(notifusecase.ExpoConfig{AccessToken: cfg.Expo.AccessToken})

	eventBus := events.NewBus(log)
	notifmodule.RegisterEventHandlers(eventBus, notifSvc, log)
	outboxStore := outbox.NewStore(pool, queries)
	outboxPub := outbox.NewPublisher(outboxStore, eventBus, log)
	outboxStop := outboxPub.StartRun(ctx)
	defer outboxStop()

	activityRec := activity.NewRecorder(queries, log)
	// Tenant resources: adapters read the organization from the job row.
	branchSvc := branchusecase.New(branchrepository.NewPostgres(queries), nil)
	staffSvc := staffusecase.New(staffrepository.NewPostgres(pool, queries), nil)
	resourcesSvc := resourcesusecase.New(resourcesrepository.NewPostgres(pool, queries), nil)
	servicesSvc := servicesusecase.New(servicesrepository.NewPostgres(pool, queries), nil)
	customersSvc := customersusecase.New(customersrepository.NewPostgres(pool, queries), nil)
	// Platform organizations (export, bulk activate / suspend). Status
	// changes invalidate the API's organization cache when Redis is up.
	jobScopes := orgusecase.New(orgrepository.NewPostgres(pool, queries), nil)
	if rdb, err := cache.NewRedisClient(ctx, cfg.Redis); err == nil {
		defer func() { _ = rdb.Close() }()
		jobScopes.SetInvalidator(orgcache.New(jobScopes, rdb, cfg.Tenancy.CacheTTL, cfg.App.Env))
	} else {
		log.Warn("worker_redis_unavailable", "error", err)
	}
	ioReg := ioengine.NewRegistry(
		ioadapters.NewUsers(queries),
		ioadapters.NewRoles(queries),
		ioadapters.NewNotifications(queries),
		ioadapters.NewActivity(queries),
		branchadapters.NewIO(branchSvc),
		staffadapters.NewIO(staffSvc),
		resourcesadapters.NewIO(resourcesSvc),
		servicesadapters.NewIO(servicesSvc),
		customersadapters.NewIO(customersSvc),
		orgadapters.NewIO(jobScopes),
	)
	exportSvc := exportusecase.New(queries, store, ioReg, nil, notifSvc, activityRec, log)
	importSvc := importusecase.New(queries, store, ioReg, nil, notifSvc, activityRec, log)
	bulkReg := bulkengine.NewRegistry(
		bulkadapters.NewUsers(queries),
		bulkadapters.NewRoles(queries),
		branchadapters.NewBulk(branchSvc),
		staffadapters.NewBulk(staffSvc),
		resourcesadapters.NewBulk(resourcesSvc),
		servicesadapters.NewBulk(servicesSvc),
		customersadapters.NewBulk(customersSvc),
		orgadapters.NewBulk(jobScopes),
	)
	bulkSvc := bulkusecase.New(queries, bulkReg, nil, notifSvc, activityRec, cfg.Bulk, log)
	// Tenant jobs run with the requester's current membership (permissions
	// and branch scope), re-resolved from the job's actor.
	exportSvc.SetJobScopeLoader(jobScopes)
	importSvc.SetJobScopeLoader(jobScopes)
	bulkSvc.SetJobScopeLoader(jobScopes)
	logsSvc := logsusecase.New(queries)
	searchReg := searchengine.NewRegistry(
		searchadapters.NewUsers(queries),
		searchadapters.NewRoles(queries),
		branchadapters.NewSearch(queries),
		staffadapters.NewSearch(queries),
		resourcesadapters.NewSearch(queries),
		servicesadapters.NewSearch(queries),
		customersadapters.NewSearch(queries),
	)
	searchClient := searchengine.NewClient(cfg.Search, log)
	searchIndexer := searchengine.NewIndexer(searchClient, searchReg, nil, log)
	// Deliveries run inline here; mobile pushes go through the queue (retries).
	queueClient := queue.NewClient(cfg.Redis)
	defer func() { _ = queueClient.Close() }()
	notifSvc.WithPushQueue(queueClient)
	persist := logging.Attach(log, logsSvc)
	log = persist.Logger()
	defer persist.Close()
	// Bulk / import jobs finish here: tell open lists what changed.
	resourceEvents := resourceevents.New(publisher, log)
	defer resourceEvents.Wait()
	// Tenant jobs carry only the organization id: scope their events.
	resourceEvents.AddResolver(tenantmodule.ChannelResolver(tenantmodule.QueriesLookup(queries)))
	importSvc.SetResourceEvents(resourceEvents)
	bulkSvc.SetResourceEvents(resourceEvents)

	// Integration jobs (TEC-880): queue steps and the minute sweep of due
	// steps (lost tasks, delivery reports, backoff retries).
	fieldCipher, err := crypto.NewFieldCipher(cfg.Encryption.Key)
	if err != nil {
		log.Error("field_cipher_failed", "error", err)
		exit(1)
	}
	integrationKit, err := integrationsmodule.New(queries, fieldCipher, cfg.App.Env, log)
	if err != nil {
		log.Error("integrations_failed", "error", err)
		exit(1)
	}
	integrationKit.Runner.SetDispatcher(queue.IntegrationDispatcher{Client: queueClient})
	integrationKit.Runner.SetResourceEvents(resourceEvents)

	worker := queue.NewWorker(cfg, log, notifSvc.Deliver).
		WithExport(exportSvc.ProcessExport).
		WithImport(importSvc.ProcessImport).
		WithBulk(bulkSvc.ProcessBulk).
		WithLogPurge(logsSvc.ApplyDueRules).
		WithMobilePush(notifSvc.SendMobilePush, notifSvc.ProcessPushReceipts).
		WithIntegrations(func(ctx context.Context, orgID, jobID int64) error {
			_, err := integrationKit.Runner.Process(ctx, orgID, jobID, integrationjobsusecase.TriggerWorker)
			if errors.Is(err, integrationjobsusecase.ErrNotFound) || errors.Is(err, integrationjobsusecase.ErrBusy) {
				// Deleted with its organization / another step holds it.
				return nil
			}
			return err
		}, integrationKit.Runner.RunDue).
		WithSearch(
			searchIndexer.ProcessUpsert,
			searchIndexer.ProcessDelete,
			searchIndexer.ProcessReindex,
		)

	if searchIndexer.Enabled() {
		go searchIndexer.Bootstrap(ctx)
	}

	scheduler, err := queue.StartLogPurgeScheduler(cfg, log)
	if err != nil {
		log.Error("log_purge_scheduler_failed", "error", err)
		exit(1)
	}
	if err := queue.RegisterPushReceiptsSchedule(scheduler); err != nil {
		log.Error("push_receipts_scheduler_failed", "error", err)
		exit(1)
	}
	if err := queue.RegisterIntegrationSweepSchedule(scheduler); err != nil {
		log.Error("integration_sweep_scheduler_failed", "error", err)
		exit(1)
	}
	go func() {
		if err := scheduler.Run(); err != nil {
			log.Error("log_purge_scheduler_stopped", "error", err)
		}
	}()

	log.Info("worker_started",
		"app", cfg.App.Name,
		"env", cfg.App.Env,
		"concurrency", cfg.Queue.Concurrency,
	)

	if n, err := notifSvc.ReclaimStuck(ctx, notifusecase.DefaultStuckProcessingMinutes); err != nil {
		log.Error("notification_reclaim_failed", "error", err)
	} else if n > 0 {
		log.Info("notification_reclaim_completed", "count", n)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- worker.Start()
	}()

	select {
	case <-ctx.Done():
		log.Info("worker_shutdown_signal")
		worker.Shutdown()
	case err := <-errCh:
		if err != nil {
			log.Error("worker_failed", "error", err)
			exit(1)
		}
	}
	log.Info("worker_stopped")
}
