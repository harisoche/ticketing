package service

import "context"

// TxManager runs `fn` inside a single database transaction. Repositories
// implemented against the same underlying connection must honour the
// transaction by extracting it from `ctx` (using the implementation's
// chosen key) before issuing queries.
//
// Defining this in the domain layer keeps the use case from importing
// GORM directly while still letting it compose atomic write flows.
type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}
