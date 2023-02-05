package tx

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"github.com/uptrace/bun"
	"github.com/uptrace/go-clickhouse/ch"

	"github.com/iam047801/tonidx/internal/core"
)

var _ core.TxRepository = (*Repository)(nil)

type Repository struct {
	ch *ch.DB
	pg *bun.DB
}

func NewRepository(_ch *ch.DB, _pg *bun.DB) *Repository {
	return &Repository{ch: _ch, pg: _pg}
}

func createIndexes(ctx context.Context, pgDB *bun.DB) error {
	var err error

	// transactions

	_, err = pgDB.NewCreateIndex().
		Model(&core.Transaction{}).
		Using("HASH").
		Column("address").
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "transaction address pg create index")
	}

	_, err = pgDB.NewCreateIndex().
		Model(&core.Transaction{}).
		Using("HASH").
		Column("block_workchain", "block_shard", "block_seq_no").
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "tx block id pg create unique index")
	}

	_, err = pgDB.NewCreateIndex().
		Model(&core.Transaction{}).
		Using("HASH").
		Column("block_file_hash").
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "tx block id pg create unique index")
	}

	// messages

	_, err = pgDB.NewCreateIndex().
		Model(&core.Message{}).
		Using("HASH").
		Column("src_address").
		Where("length(src_address) > 0").
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "message src addr pg create index")
	}

	_, err = pgDB.NewCreateIndex().
		Model(&core.Message{}).
		Using("HASH").
		Column("dst_address").
		Where("length(dst_address) > 0").
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "message dst addr pg create index")
	}

	_, err = pgDB.NewCreateIndex().
		Model(&core.Message{}).
		Using("BTREE").
		Column("created_lt").
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "message created_lt pg create index")
	}

	// message payloads

	_, err = pgDB.NewCreateIndex().
		Model(&core.MessagePayload{}).
		Using("HASH").
		Column("src_contract").
		Where("length(src_contract) > 0").
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "message payload pg create src_contract index")
	}

	_, err = pgDB.NewCreateIndex().
		Model(&core.MessagePayload{}).
		Using("HASH").
		Column("dst_contract").
		Where("length(dst_contract) > 0").
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "message payload pg create dst_contract index")
	}

	return nil
}

func CreateTables(ctx context.Context, chDB *ch.DB, pgDB *bun.DB) error {
	_, err := pgDB.ExecContext(ctx, "CREATE TYPE message_type AS ENUM (?, ?, ?)",
		core.ExternalIn, core.ExternalOut, core.Internal)
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		return errors.Wrap(err, "account status pg create enum")
	}

	_, err = chDB.NewCreateTable().
		IfNotExists().
		Engine("ReplacingMergeTree").
		Model(&core.MessagePayload{}).
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "message payload ch create table")
	}

	_, err = pgDB.NewCreateTable().
		Model(&core.MessagePayload{}).
		IfNotExists().
		WithForeignKeys().
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "message payload pg create table")
	}

	_, err = chDB.NewCreateTable().
		IfNotExists().
		Engine("ReplacingMergeTree").
		Model(&core.Message{}).
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "message ch create table")
	}

	_, err = pgDB.NewCreateTable().
		Model(&core.Message{}).
		IfNotExists().
		// WithForeignKeys().
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "message pg create table")
	}

	_, err = chDB.NewCreateTable().
		IfNotExists().
		Engine("ReplacingMergeTree").
		Model(&core.Transaction{}).
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "transaction ch create table")
	}

	_, err = pgDB.NewCreateTable().
		Model(&core.Transaction{}).
		IfNotExists().
		WithForeignKeys().
		Exec(ctx)
	if err != nil {
		return errors.Wrap(err, "transaction pg create table")
	}

	return nil
}

func (r *Repository) AddTransactions(ctx context.Context, transactions []*core.Transaction) error {
	_, err := r.ch.NewInsert().Model(&transactions).Exec(ctx)
	if err != nil {
		return err
	}
	_, err = r.pg.NewInsert().Model(&transactions).Exec(ctx)
	if err != nil {
		return err
	}
	return nil
}

func (r *Repository) AddMessages(ctx context.Context, messages []*core.Message) error {
	_, err := r.ch.NewInsert().Model(&messages).Exec(ctx)
	if err != nil {
		return err
	}
	_, err = r.pg.NewInsert().Model(&messages).Exec(ctx)
	if err != nil {
		return err
	}
	return nil
}

func (r *Repository) AddMessagePayloads(ctx context.Context, payloads []*core.MessagePayload) error {
	_, err := r.ch.NewInsert().Model(&payloads).Exec(ctx)
	if err != nil {
		return err
	}
	_, err = r.pg.NewInsert().Model(&payloads).Exec(ctx)
	if err != nil {
		return err
	}
	return nil
}

func (r *Repository) GetSourceMessageHash(ctx context.Context, from, to string, lt uint64) (ret []byte, err error) {
	err = r.pg.NewSelect().Model(&core.Message{}).
		Column("hash").
		Where("src_address = ?", from).
		Where("dst_address = ?", to).
		Where("created_lt = ?", lt).
		Scan(ctx)
	if err != nil {
		return nil, err
	}

	return ret, nil
}

func selectTxFilter(q *bun.SelectQuery, f *core.TransactionFilter) *bun.SelectQuery {
	if len(f.Hash) > 0 {
		q = q.Where("hash = ?", f.Hash)
	}
	if len(f.Address) > 0 {
		q = q.Where("address = ?", f.Address)
	}
	if f.BlockID != nil {
		q = q.Where("block_workchain = ?", f.BlockID.Workchain).
			Where("block_shard = ?", f.BlockID.Shard).
			Where("block_seq_no = ?", f.BlockID.SeqNo)
	}
	if f.BlockFileHash != nil {
		q = q.Where("block_file_hash", f.BlockFileHash)
	}
	return q
}

func (r *Repository) GetTransactions(ctx context.Context, filter *core.TransactionFilter, offset, limit int) (ret []*core.Transaction, err error) {
	err = selectTxFilter(r.pg.NewSelect().Model(&ret), filter).
		Order("created_lt DESC").
		Offset(offset).Limit(limit).Scan(ctx)
	return ret, err
}

func selectMsgFilter(q *bun.SelectQuery, f *core.MessageFilter) *bun.SelectQuery {
	if len(f.Hash) > 0 {
		q = q.Where("hash = ?", f.Hash)
	}
	if len(f.SrcAddress) > 0 {
		q = q.Where("src_address = ?", f.SrcAddress)
	}
	if len(f.DstAddress) > 0 {
		q = q.Where("dst_address = ?", f.DstAddress)
	}
	if f.WithPayload {
		if f.SrcContract != "" {
			q = q.Where("payload.src_contract = ?", f.SrcContract)
		}
		if f.DstContract != "" {
			q = q.Where("payload.dst_contract = ?", f.DstContract)
		}
		if f.OperationName != "" {
			q = q.Where("payload.operation_name = ?", f.OperationName)
		}
	}
	return q
}

func (r *Repository) GetMessages(ctx context.Context, filter *core.MessageFilter, offset, limit int) ([]*core.MessageFilter, error) {
	panic(fmt.Errorf("not implemented"))
}
