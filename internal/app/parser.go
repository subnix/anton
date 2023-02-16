package app

import (
	"context"

	"github.com/xssnick/tonutils-go/tlb"
	"github.com/xssnick/tonutils-go/ton"

	"github.com/iam047801/tonidx/internal/core"
	"github.com/iam047801/tonidx/internal/core/repository"
)

type ServerAddr struct {
	IPPort    string
	PubKeyB64 string
}

type ParserConfig struct {
	DB      *repository.DB
	Servers []*ServerAddr
}

type ParserService interface {
	API() *ton.APIClient

	ParseAccountData(ctx context.Context, b *tlb.BlockInfo, acc *tlb.Account) (*core.AccountData, error)
	ParseMessagePayload(ctx context.Context, src, dst *core.AccountState, message *core.Message) (*core.MessagePayload, error)
}
