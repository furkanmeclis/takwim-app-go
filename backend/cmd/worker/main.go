package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/cache"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/config"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/logging"
	availabilityusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/availability/usecase"
	branchadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/adapters"
	branchrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/repository"
	branchusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/branches/usecase"
	bulkusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/bulk/usecase"
	cashadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/adapters"
	cashrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/repository"
	cashusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/cash/usecase"
	classesadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/classes/adapters"
	classesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/classes/usecase"
	confirmationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/confirmations/usecase"
	customernotifyadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/adapters"
	customernotifycatalog "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/catalog"
	customernotifyusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customernotify/usecase"
	customerpanelusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customerpanel/usecase"
	customersadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/adapters"
	customersrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/repository"
	customersusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/customers/usecase"
	entitlementsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/entitlements"
	exportusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/exports/usecase"
	financeadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/adapters"
	financerepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/repository"
	financeusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/finance/usecase"
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
	orgsettingsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/repository"
	orgsettingsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/orgsettings/usecase"
	paymentsmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments"
	paymentsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/payments/usecase"
	productsadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/adapters"
	productsrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/repository"
	productsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/products/usecase"
	reservationsadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/adapters"
	reservationsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/reservations/usecase"
	resourcesadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/resources/adapters"
	resourcesrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/resources/repository"
	resourcesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/resources/usecase"
	salesmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales"
	salesrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/repository"
	salesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sales/usecase"
	servicesadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/services/adapters"
	servicesrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/services/repository"
	servicesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/services/usecase"
	sessionpackagesadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sessionpackages/adapters"
	sessionpackagesrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sessionpackages/repository"
	sessionpackagesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/sessionpackages/usecase"
	staffadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staff/adapters"
	staffrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staff/repository"
	staffusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staff/usecase"
	staffsharesadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staffshares/adapters"
	staffsharesrepository "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staffshares/repository"
	staffsharesusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/staffshares/usecase"
	tenantmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/tenant"
	voicecallmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/voicecall"
	voicecallusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/voicecall/usecase"
	whatsappmodule "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/whatsapp"
	whatsappusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/whatsapp/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/observability"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/activity"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine"
	bulkadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/bulkengine/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/events"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine"
	ioadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/ioengine/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/mail"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/orgctx/orgcache"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/outbox"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/resourceevents"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine"
	searchadapters "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/searchengine/adapters"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/sms"
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
	productsSvc := productsusecase.New(productsrepository.NewPostgres(pool, queries), nil)
	customersSvc := customersusecase.New(customersrepository.NewPostgres(pool, queries), nil)
	// Reservation exports and bulk transitions (TEC-963) run with the
	// business settings (payment hold times on accept).
	orgSettings := orgsettingsusecase.New(orgsettingsrepository.NewPostgres(pool, queries))
	reservationHolds := availabilityusecase.NewHolds(pool, queries, orgSettings)
	// Customer notifications (TEC-911): reservation changes made here (bulk
	// transitions, expiries) write their notices like the API's; this
	// process sends every business's due notices and plans the reminders.
	notifyEngine := customernotifyusecase.New(pool, queries, strings.TrimRight(cfg.Auth.FrontendURL, "/")+"/panel")
	whatsappClient := whatsappmodule.Client(cfg)
	whatsappSvc := whatsappusecase.New(queries, whatsappClient)
	whatsappSvc.SetLogger(log)
	voiceClient := voicecallmodule.Client(cfg)
	voiceSvc := voicecallusecase.New(queries, voiceClient, voicecallmodule.WebhookBase(cfg))
	voiceSvc.SetLogger(log)
	notifyEngine.SetLogger(log)
	notifyEngine.Register(
		// The SMS provider is not chosen yet (TEC-884): the log sender
		// counts as not ready, so nothing is recorded as sent through it.
		customernotifyadapters.SMS{Sender: sms.NewLogSender(log, cfg.App.Env != "production")},
		customernotifyadapters.Email{Mail: mailer, Configured: cfg.SMTP.Driver == "smtp" && cfg.SMTP.Host != ""},
		// WhatsApp (TEC-922): the confirmation request through the business's
		// own Cloud API account (accounts plugged in with the integration kit).
		whatsappusecase.NewChannel(whatsappSvc),
		// Voice calls (TEC-923): the confirmation call through the business's
		// own Netgsm account; its result comes to the API's webhook.
		voicecallusecase.NewChannel(voiceSvc),
	)
	usage := entitlementsmodule.New(queries)
	notifyEngine.SetUsage(func(ctx context.Context, orgID int64, timezone, metric string) {
		if err := usage.AddUsage(ctx, orgctx.Scope{ID: orgID, Timezone: timezone}, metric, 1); err != nil {
			log.Warn("customer_notice_usage_failed", "organization", orgID, "metric", metric, "error", err)
		}
	})
	// Customer confirmation (TEC-921): changes made here plan or follow
	// their tasks like the API's; this process sends the due requests and
	// applies the deadlines (cancel through the reservation transition).
	confirmEngine := confirmationsusecase.New(pool, queries, notifyEngine)
	confirmEngine.SetLogger(log)
	notifyEngine.SetDispatchGuard(confirmEngine.Guard)
	reservationNotifier := reservationsusecase.Notifiers(notifyEngine, confirmEngine)
	reservationHolds.SetExpiryHook(reservationsusecase.ExpireRequestsNotifying(reservationNotifier))
	reservationsSvc := reservationsusecase.New(reservationHolds, queries, orgSettings)
	reservationsSvc.SetNotifier(reservationNotifier)
	confirmEngine.SetReservations(reservationsSvc)
	cashSvc := cashusecase.New(cashrepository.NewPostgres(pool, queries), nil)
	// Completed visits and stock sales become sale lines (TEC-916) in the
	// same transaction, whichever process runs them.
	salesSvc := salesusecase.New(salesrepository.NewPostgres(pool, queries), cashSvc)
	reservationsSvc.SetCompletionHook(salesSvc.RecordReservationTx)
	productsSvc.SetSaleRecorder(salesmodule.ProductSaleRecorder(salesSvc))
	// ... and every sale line with a staff member its share (TEC-917).
	staffSharesSvc := staffsharesusecase.New(staffsharesrepository.NewPostgres(pool, queries), cashSvc)
	salesSvc.SetLineHook(staffSharesSvc.RecordLineTx)
	productsSvc.SetSaleReverser(salesmodule.ProductSaleReverser(salesSvc))
	// Income and expense records (TEC-915): the export of the list.
	financeSvc := financeusecase.New(financerepository.NewPostgres(pool, queries), cashSvc, salesSvc)
	// Session packages (TEC-984): export of the sold packages and bulk
	// activate / deactivate of the definitions.
	sessionPackagesSvc := sessionpackagesusecase.New(sessionpackagesrepository.NewPostgres(pool, queries), salesSvc)
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
		productsadapters.NewIO(productsSvc),
		productsadapters.NewMovementsIO(productsSvc),
		sessionpackagesadapters.NewIO(sessionPackagesSvc),
		customersadapters.NewIO(customersSvc),
		reservationsadapters.NewIO(reservationsSvc),
		cashadapters.NewAccountsIO(cashSvc),
		cashadapters.NewMovementsIO(cashSvc),
		financeadapters.NewEntriesIO(financeSvc),
		staffsharesadapters.NewLinesIO(staffSharesSvc),
		staffsharesadapters.NewPayoutsIO(staffSharesSvc),
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
		productsadapters.NewBulk(productsSvc),
		sessionpackagesadapters.NewBulk(sessionPackagesSvc),
		customersadapters.NewBulk(customersSvc),
		reservationsadapters.NewBulk(reservationsSvc),
		classesadapters.NewSessionBulk(classesusecase.New(queries, reservationHolds), reservationsSvc),
		cashadapters.NewBulk(cashSvc),
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
		productsadapters.NewSearch(queries),
		sessionpackagesadapters.NewSearch(queries),
		customersadapters.NewSearch(queries),
		reservationsadapters.NewSearch(queries),
		cashadapters.NewSearch(queries),
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
	// Confirmation requests and deadlines move the confirmation list
	// (TEC-924).
	confirmEngine.SetResourceEvents(resourceEvents)
	// ... and so do the send results and delivery reports of the requests
	// (a failed or skipped send, TEC-981).
	notifyEngine.SetResourceEvents(resourceEvents, confirmationsusecase.Event)

	// Integration jobs (TEC-880): queue steps and the minute sweep of due
	// steps (lost tasks, delivery reports, backoff retries).
	fieldCipher, err := crypto.NewFieldCipher(cfg.Encryption.Key)
	if err != nil {
		log.Error("field_cipher_failed", "error", err)
		exit(1)
	}
	// SMS notices carry the phone's single-use panel link (TEC-910); the
	// link rows and their keyed hashes belong to the customer panel.
	panelCfg := customerpanelusecase.DefaultConfig
	panelCfg.PanelURL = strings.TrimRight(cfg.Auth.FrontendURL, "/") + "/panel"
	panelLinks := customerpanelusecase.New(queries, nil, fieldCipher, nil, nil, customerpanelusecase.PanelDeps{}, log)
	panelLinks.SetConfig(panelCfg)
	notifyEngine.SetLinkIssuer(customernotifyadapters.PanelLinks(panelLinks))
	// Confirmation requests carry a link to the one visit (confirm, cancel,
	// ask for a change), issued at send time.
	notifyEngine.SetLinkIssuerFor(customernotifycatalog.ConfirmationRequest, customernotifyadapters.VisitLinks(panelLinks))
	integrationAdapters := append(paymentsmodule.Adapters(cfg), whatsappmodule.Adapters(whatsappClient)...)
	integrationKit, err := integrationsmodule.New(queries, fieldCipher, cfg.App.Env, log,
		append(integrationAdapters, voicecallmodule.Adapters(voiceClient)...)...)
	if err != nil {
		log.Error("integrations_failed", "error", err)
		exit(1)
	}
	whatsappSvc.SetAccounts(integrationKit.Accounts, integrationKit.Runner)
	voiceSvc.SetAccounts(integrationKit.Accounts, integrationKit.Runner, fieldCipher)
	integrationKit.Runner.SetDispatcher(queue.IntegrationDispatcher{Client: queueClient})
	integrationKit.Runner.SetResourceEvents(resourceEvents)
	// Provider refunds whose result is unknown (TEC-978): this process asks
	// the provider's refund status query, posts what it confirms and tells
	// the customer; nothing is sent to the provider again.
	paymentsSvc := paymentsusecase.New(queries, reservationsSvc, salesSvc, integrationKit.Accounts, integrationKit.Runner,
		paymentsusecase.Config{FrontendURL: cfg.Auth.FrontendURL}, log)
	paymentsSvc.SetResourceEvents(resourceEvents)
	paymentsSvc.SetRefundNotifier(paymentsmodule.RefundNotices(notifyEngine, queries))

	// The minute sweep of capacity holds also cancels the requests whose
	// approval or payment hold lapsed (TEC-899).
	capacityHolds := availabilityusecase.NewHolds(pool, queries, nil)
	capacityHolds.SetExpiryHook(reservationsusecase.ExpireRequestsNotifying(reservationNotifier))
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
		WithCapacityExpiry(capacityHolds.ExpireDue).
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
	if err := queue.RegisterCapacityExpirySchedule(scheduler); err != nil {
		log.Error("capacity_expiry_scheduler_failed", "error", err)
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

	go notifyEngine.Run(ctx, customernotifyusecase.DefaultSendInterval, customernotifyusecase.DefaultReminderInterval)
	go confirmEngine.Run(ctx, confirmationsusecase.DefaultInterval)
	go paymentsSvc.RunRefundChecks(ctx, paymentsusecase.DefaultRefundCheckInterval)

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
