// Command server is the composition root: wires concrete adapters into
// domain use-cases and starts the HTTP server. Mirrors
// backend/app/core/dependencies.py + backend/app/main.py.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"syscall"
	"time"

	"bailleul.dev/budget-server/internal/adapters/banksync"
	"bailleul.dev/budget-server/internal/adapters/db"
	"bailleul.dev/budget-server/internal/adapters/mail"
	"bailleul.dev/budget-server/internal/adapters/otaupdates"
	"bailleul.dev/budget-server/internal/adapters/security"
	"bailleul.dev/budget-server/internal/adapters/web"
	"bailleul.dev/budget-server/internal/core"
	"bailleul.dev/budget-server/internal/domain/ports"
	"bailleul.dev/budget-server/internal/domain/services"
)

func main() {
	settings := core.LoadSettings()
	if err := settings.Validate(); err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, settings); err != nil {
		log.Fatal(err)
	}
}

// run wires every adapter into the domain's use-cases and serves HTTP until
// ctx is cancelled, then shuts down gracefully - split out from main so the
// composition root itself can be exercised in a test (see main_test.go),
// which is otherwise impossible for a bare func main().
func run(ctx context.Context, settings core.Settings) error {
	log.Println("applying database migrations...")
	if err := core.RunMigrations(settings.DatabaseURL); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	pool, err := db.NewPool(ctx, settings.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	// --- Repositories ------------------------------------------------------
	users := db.NewUserRepository(pool)
	accounts := db.NewAccountRepository(pool)
	categories := db.NewCategoryRepository(pool)
	checkpoints := db.NewBalanceCheckpointRepository(pool)
	operations := db.NewOperationRepository(pool)
	operationSplits := db.NewOperationCategorySplitRepository(pool)
	comments := db.NewCommentRepository(pool)
	plannedItems := db.NewPlannedItemRepository(pool)
	occurrences := db.NewPlannedOccurrenceRepository(pool)
	bankProfiles := db.NewBankProfileRepository(pool)
	importBatches := db.NewImportBatchRepository(pool)
	collaborators := db.NewCollaboratorRepository(pool)
	bankConnections := db.NewBankConnectionRepository(pool)
	oauthClients := db.NewOAuthClientRepository(pool)
	oauthCodes := db.NewOAuthAuthorizationCodeRepository(pool)
	oauthGrants := db.NewOAuthGrantRepository(pool)

	// --- Domain use-cases ----------------------------------------------------
	clock := core.SystemClock{}
	passwordHasher := security.BcryptPasswordHasher{}
	authTokens := db.NewAuthTokenRepository(pool)
	authUseCase := services.NewAuthUseCase(users, passwordHasher, clock).WithSignupPolicy(settings.SignupAllowed).WithEmailFlows(services.EmailFlows{
		Tokens:    authTokens,
		Mailer:    newMailer(settings),
		PublicURL: settings.PublicURL,
	})
	switch {
	case slices.Contains(settings.SignupAllowedEmails, "*"):
		log.Println("registration: OPEN to everybody (SIGNUP_ALLOWED_EMAILS=*)")
	case len(settings.SignupAllowedEmails) == 0:
		log.Println("registration: CLOSED (SIGNUP_ALLOWED_EMAILS is empty) — only existing accounts can sign in")
	default:
		log.Printf("registration: private beta, %d address(es) allowed", len(settings.SignupAllowedEmails))
	}
	if settings.PublicURL == "" {
		log.Println("warning: PUBLIC_URL is not set — verification and password-reset emails cannot be sent")
	}
	generateOccurrences := services.NewGenerateOccurrencesUseCase(plannedItems, occurrences)
	reconcileUseCase := services.NewReconcileUseCase(operations, occurrences, plannedItems)
	detectRecurringUseCase := services.NewRecurringDetectionUseCase(operations, plannedItems)
	autoCategorizeUseCase := services.NewAutoCategorizeUseCase(operations, categories)
	previsionsUseCase := services.NewPrevisionsUseCase(operations, occurrences, checkpoints)
	importCsvUseCase := services.NewImportCsvUseCase(operations, importBatches, clock)
	sharingUseCase := services.NewSharingUseCase(collaborators, clock)
	computeInitialBalanceUseCase := services.NewComputeInitialBalanceUseCase(operations, checkpoints)
	oauthUseCase := services.NewOAuthUseCase(oauthClients, oauthCodes, oauthGrants, clock)

	bankSyncEncryptor, err := security.NewSecretBox(settings.BankSyncEncryptionKey)
	if err != nil {
		return fmt.Errorf("invalid BANK_SYNC_ENCRYPTION_KEY: %w", err)
	}
	bankSyncConnector := banksync.NewCLIConnector(settings.BankSyncPythonBin, settings.BankSyncProjectDir)
	exportUserDataUseCase := services.NewExportUserDataUseCase(accounts, checkpoints, operations, operationSplits, categories, plannedItems, occurrences, bankProfiles, comments, collaborators, importBatches, bankConnections)
	syncBankUseCase := services.NewSyncBankAccountsUseCase(bankConnections, accounts, checkpoints, operations, bankSyncConnector, bankSyncEncryptor, clock, autoCategorizeUseCase)
	connectBankUseCase := services.NewConnectBankUseCase(bankConnections, bankSyncEncryptor, syncBankUseCase)
	bankConnectionStatusUseCase := services.NewBankConnectionStatusUseCase(bankConnections)
	syncAllBankConnectionsUseCase := services.NewSyncAllBankConnectionsUseCase(bankConnections, syncBankUseCase)

	otaUpdatesHandler, err := newOTAUpdatesHandler(settings)
	if err != nil {
		return err
	}

	core.SeedGlobalBankProfiles(ctx, bankProfiles, clock)
	core.SeedDevData(ctx, settings, authUseCase, users, clock)

	// --- HTTP handlers -------------------------------------------------------
	accountHandler := web.NewAccountHandler(accounts, users, collaborators)
	categoryHandler := web.NewCategoryHandler(categories, accountHandler)
	operationHandler := web.NewOperationHandler(operations, categories, operationSplits, comments, accountHandler, autoCategorizeUseCase)
	bankSyncHandler := web.NewBankSyncHandler(connectBankUseCase, syncBankUseCase, bankConnectionStatusUseCase).
		WithAllowList(users, settings.BankSyncAllowedEmails)
	handlers := web.Handlers{
		Auth:               web.NewAuthHandler(authUseCase, settings),
		Accounts:           accountHandler,
		Categories:         categoryHandler,
		BalanceCheckpoints: web.NewBalanceCheckpointHandler(checkpoints, accountHandler, computeInitialBalanceUseCase),
		Operations:         operationHandler,
		PlannedItems:       web.NewPlannedItemHandler(plannedItems, occurrences, categories, generateOccurrences, reconcileUseCase, detectRecurringUseCase, accountHandler),
		Reconciliation:     web.NewReconciliationHandler(operations, occurrences, plannedItems, accountHandler, reconcileUseCase),
		Previsions:         web.NewPrevisionsHandler(previsionsUseCase, accountHandler),
		BankProfiles:       web.NewBankProfileHandler(bankProfiles),
		Imports:            web.NewImportHandler(bankProfiles, importCsvUseCase, reconcileUseCase, autoCategorizeUseCase, accountHandler),
		Collaborators:      web.NewCollaboratorHandler(collaborators, users, sharingUseCase),
		Export:             web.NewExportHandler(exportUserDataUseCase),
		BankSync:           bankSyncHandler,
		MCP:                web.NewMCPHandler(accountHandler, categoryHandler, operationHandler, bankSyncHandler),
		OAuth:              web.NewOAuthHandler(oauthUseCase, authUseCase, users, settings),
		OTAUpdates:         otaUpdatesHandler,
	}

	router := web.NewRouter(handlers, users, settings)

	addr := ":" + settings.Port
	server := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		// Bounds a slow upload. No WriteTimeout on purpose: a bank sync
		// waiting on a 2FA validation legitimately keeps a response open.
		ReadTimeout: 2 * time.Minute,
		IdleTimeout: 2 * time.Minute,
	}

	go func() {
		log.Printf("budget-server listening on %s", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	go core.RunDailyBankSync(ctx, syncAllBankConnectionsUseCase)

	<-ctx.Done()
	log.Println("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown error: %w", err)
	}
	return nil
}

// newMailer returns the SMTP mailer when SMTP_HOST is set, otherwise a stand-in
// that only logs (the message body — a live recovery link — only in
// development).
func newMailer(settings core.Settings) ports.Mailer {
	if settings.SMTPHost == "" {
		if settings.IsProduction() {
			log.Println("warning: SMTP_HOST is not set — emails are NOT sent (verification and password reset won't work)")
		}
		return mail.LogMailer{LogBody: !settings.IsProduction()}
	}
	return &mail.SMTPMailer{
		Host: settings.SMTPHost, Port: settings.SMTPPort,
		Username: settings.SMTPUsername, Password: settings.SMTPPassword, From: settings.SMTPFrom,
	}
}

// newOTAUpdatesHandler wires over-the-air updates for the mobile app, or
// returns nil (routes answer 404) when neither OTA_UPDATES_DIR nor
// OTA_S3_BUCKET is set. A configured-but-unreadable signing key fails
// startup instead of silently serving unsigned updates that signed builds
// would reject.
func newOTAUpdatesHandler(settings core.Settings) (*web.OTAUpdatesHandler, error) {
	var store *otaupdates.Store
	switch {
	case settings.OTAUpdatesDir != "" && settings.OTAS3Bucket != "":
		return nil, errors.New("set either OTA_UPDATES_DIR or OTA_S3_BUCKET, not both")
	case settings.OTAUpdatesDir != "":
		store = otaupdates.NewStore(settings.OTAUpdatesDir)
		log.Printf("serving mobile OTA updates from %s", settings.OTAUpdatesDir)
	case settings.OTAS3Bucket != "":
		s3Store, err := otaupdates.NewS3Store(otaupdates.S3Config{
			Endpoint:        settings.OTAS3Endpoint,
			Region:          settings.OTAS3Region,
			Bucket:          settings.OTAS3Bucket,
			AccessKeyID:     settings.OTAS3AccessKeyID,
			SecretAccessKey: settings.OTAS3SecretAccessKey,
			Prefix:          settings.OTAS3Prefix,
		})
		if err != nil {
			return nil, fmt.Errorf("invalid OTA_S3_* settings: %w", err)
		}
		store = s3Store
		log.Printf("serving mobile OTA updates from s3 bucket %s at %s", settings.OTAS3Bucket, settings.OTAS3Endpoint)
	default:
		return nil, nil
	}

	var signer ports.ManifestSigner
	if settings.OTACodeSigningKeyPath != "" {
		rsaSigner, err := security.LoadRSASigner(settings.OTACodeSigningKeyPath, "main")
		if err != nil {
			return nil, fmt.Errorf("invalid OTA_CODE_SIGNING_KEY_PATH: %w", err)
		}
		signer = rsaSigner
	} else {
		log.Println("warning: OTA updates are served unsigned (OTA_CODE_SIGNING_KEY_PATH not set)")
	}
	return web.NewOTAUpdatesHandler(services.NewResolveUpdateUseCase(store), store, signer, settings.OTAPublicURL), nil
}
