// Package integrations wires the external work skeleton (TEC-880) the same
// way in the API and the worker: the credential vault, the adapter and
// operation registries, the integration accounts service and the job
// runner. Modules that call a provider (messages, payments) register their
// adapters and operations here and enqueue work on the runner.
package integrations

import (
	"fmt"
	"log/slog"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/database/db"
	accountsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/accounts/usecase"
	jobsusecase "github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/modules/integrations/jobs/usecase"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/crypto"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations"
	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/integrations/logprovider"
)

// Kit is the wired skeleton.
type Kit struct {
	Vault    *integrations.Vault
	Ops      *integrations.OperationRegistry
	Accounts *accountsusecase.Service
	Runner   *jobsusecase.Runner
}

// New builds the kit. Development and test environments also get the log
// providers (no network call), so the pipeline runs before a real provider
// is connected.
func New(q *db.Queries, cipher *crypto.FieldCipher, appEnv string, log *slog.Logger, adapters ...integrations.Adapter) (*Kit, error) {
	vault, err := integrations.NewVault(cipher)
	if err != nil {
		return nil, fmt.Errorf("integrations: vault: %w", err)
	}
	if appEnv == "development" || appEnv == "test" {
		adapters = append(adapters, logprovider.Adapters(log)...)
	}
	accounts := accountsusecase.New(q, vault, integrations.NewRegistry(adapters...))
	ops := integrations.NewOperationRegistry()
	runner := jobsusecase.NewRunner(q, accounts, ops)
	if log != nil {
		runner.SetLogger(log)
	}
	return &Kit{Vault: vault, Ops: ops, Accounts: accounts, Runner: runner}, nil
}
