package account

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/go-clickhouse/ch"

	"github.com/iam047801/tonidx/internal/core"
)

var _ core.AccountRepository = (*Repository)(nil)

type Repository struct {
	ch *ch.DB
	pg *bun.DB
}

func NewRepository(_ch *ch.DB, _pg *bun.DB) *Repository {
	return &Repository{ch: _ch, pg: _pg}
}

func createIndexes(ctx context.Context, pgDB *bun.DB) error {
	// account data

	_, err := pgDB.NewCreateIndex().
		Model(&core.AccountData{}).
		Using("HASH").
		Column("owner_address").
		Where("length(owner_address) > 0").
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "address state pg create unique index")
	}

	_, err = pgDB.NewCreateIndex().
		Model(&core.AccountData{}).
		Using("HASH").
		Column("minter_address").
		Where("length(minter_address) > 0").
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "address state pg create unique index")
	}

	// account state

	_, err = pgDB.NewCreateIndex().
		Model(&core.AccountState{}).
		Using("HASH").
		Column("address").
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "address state pg create unique index")
	}

	_, err = pgDB.NewCreateIndex().
		Model(&core.AccountData{}).
		Using("GIN").
		Column("types").
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "account state contract types pg create index")
	}

	_, err = pgDB.NewCreateIndex().
		Model(&core.AccountState{}).
		Using("BTREE").
		Column("last_tx_lt").
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "account state last_tx_lt pg create index")
	}

	// latest account state

	_, err = pgDB.NewCreateIndex().
		Model(&core.LatestAccountState{}).
		Using("BTREE").
		Column("last_tx_lt").
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "account state last_tx_lt pg create index")
	}

	return nil
}

func CreateTables(ctx context.Context, chDB *ch.DB, pgDB *bun.DB) error {
	_, err := pgDB.ExecContext(ctx, "CREATE TYPE account_status AS ENUM (?, ?, ?, ?)",
		core.Uninit, core.Active, core.Frozen, core.NonExist)
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return errors.Wrap(err, "account status pg create enum")
	}

	_, err = chDB.NewCreateTable().
		IfNotExists().
		Engine("ReplacingMergeTree").
		Model(&core.AccountData{}).
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "account data ch create table")
	}

	_, err = pgDB.NewCreateTable().
		Model(&core.AccountData{}).
		IfNotExists().
		WithForeignKeys().
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "account data pg create table")
	}

	_, err = chDB.NewCreateTable().
		IfNotExists().
		Engine("ReplacingMergeTree").
		Model(&core.AccountState{}).
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "account state ch create table")
	}

	_, err = pgDB.NewCreateTable().
		Model(&core.AccountState{}).
		IfNotExists().
		// WithForeignKeys().
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "account state pg create table")
	}

	_, err = pgDB.NewCreateTable().
		Model(&core.LatestAccountState{}).
		IfNotExists().
		WithForeignKeys().
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "account state pg create table")
	}

	return createIndexes(ctx, pgDB)
}

func (r *Repository) AddAccountStates(ctx context.Context, tx bun.Tx, accounts []*core.AccountState) error {
	if len(accounts) == 0 {
		return nil
	}

	_, err := r.ch.NewInsert().Model(&accounts).Exec(ctx)
	if err != nil {
		return err
	}

	_, err = tx.NewInsert().Model(&accounts).Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "cannot insert new states")
	}

	for _, a := range accounts {
		_, err = tx.NewInsert().
			Model(&core.LatestAccountState{
				Address:  a.Address,
				LastTxLT: a.LastTxLT,
			}).
			On("CONFLICT (address) DO UPDATE").
			Where("latest_account_state.last_tx_lt < ?", a.LastTxLT).
			Set("last_tx_lt = EXCLUDED.last_tx_lt").
			Exec(ctx)
		if err != nil {
			return errors.Wrapf(err, "cannot set latest state for %s", &a.Address)
		}
	}

	return nil
}

func (r *Repository) AddAccountData(ctx context.Context, tx bun.Tx, data []*core.AccountData) error {
	if len(data) == 0 {
		return nil
	}
	_, err := r.ch.NewInsert().Model(&data).Exec(ctx)
	if err != nil {
		return err
	}
	_, err = tx.NewInsert().Model(&data).Exec(ctx)
	if err != nil {
		return err
	}
	return nil
}

func addAccountDataFilter(q *bun.SelectQuery, f *core.AccountStateFilter) *bun.SelectQuery {
	if !f.WithData {
		return q
	}

	prefix := ""
	if f.LatestState {
		prefix = "account_state__"
	}

	if len(f.ContractTypes) > 0 {
		q = q.Where(prefix+"state_data.types && ?", pgdialect.Array(f.ContractTypes))
	}
	if f.OwnerAddress != nil {
		q = q.Where(prefix+"state_data.owner_address = ?", f.OwnerAddress)
	}
	if f.MinterAddress != nil {
		q = q.Where(prefix+"state_data.minter_address = ?", f.MinterAddress)
	}

	return q
}

func (r *Repository) GetAccountStates(ctx context.Context, f *core.AccountStateFilter) (ret []*core.AccountState, err error) {
	var (
		q           *bun.SelectQuery
		statesTable string
		latest      []*core.LatestAccountState
	)

	// choose table to filter states by
	// and optionally join account data
	if f.LatestState {
		q = r.pg.NewSelect().Model(&latest).Relation("AccountState", func(q *bun.SelectQuery) *bun.SelectQuery {
			return q.ExcludeColumn("code", "data") // TODO: optional
		})
		if f.WithData {
			q = q.Relation("AccountState.StateData")
		}
		statesTable = "latest_account_state."
	} else {
		q = r.pg.NewSelect().Model(&ret).
			ExcludeColumn("code", "data") // TODO: optional
		if f.WithData {
			q = q.Relation("StateData")
		}
		statesTable = "account_state."
	}

	if len(f.Addresses) > 0 {
		q = q.Where(statesTable+"address in (?)", bun.In(f.Addresses))
	}

	q = addAccountDataFilter(q, f)

	if f.AfterTxLT != nil {
		if f.Order == "ASC" {
			q = q.Where(statesTable+"last_tx_lt > ?", f.AfterTxLT)
		} else {
			q = q.Where(statesTable+"last_tx_lt < ?", f.AfterTxLT)
		}
	}
	if f.Order != "" {
		q = q.Order(statesTable + "last_tx_lt " + strings.ToUpper(f.Order))
	}

	if f.Limit == 0 {
		f.Limit = 3
	}
	q = q.Limit(f.Limit)

	err = q.Scan(ctx)

	if f.LatestState {
		for _, a := range latest {
			ret = append(ret, a.AccountState)
		}
	}

	return ret, err
}
