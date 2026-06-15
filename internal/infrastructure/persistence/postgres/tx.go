package postgres

import (
	"context"

	"gorm.io/gorm"

	"ticketing-api/internal/domain/service"
)

// txCtxKey is the context key under which the active *gorm.DB lives during
// a TxManager.WithinTx call. It is unexported and unique so it cannot
// collide with other context values.
type txCtxKey struct{}

// TxManager implements service.TxManager backed by GORM.
type TxManager struct {
	db *gorm.DB
}

func NewTxManager(db *gorm.DB) service.TxManager {
	return &TxManager{db: db}
}

func (m *TxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txCtx := context.WithValue(ctx, txCtxKey{}, tx)
		return fn(txCtx)
	})
}

// dbFrom returns the active transaction handle from ctx if present, otherwise
// the supplied default DB scoped to ctx. Repositories call this so the same
// method works inside and outside a transaction.
func dbFrom(ctx context.Context, def *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(txCtxKey{}).(*gorm.DB); ok && tx != nil {
		return tx.WithContext(ctx)
	}
	return def.WithContext(ctx)
}
