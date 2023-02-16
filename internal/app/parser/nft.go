package parser

import (
	"context"

	"github.com/pkg/errors"
	"github.com/xssnick/tonutils-go/address"
	"github.com/xssnick/tonutils-go/tlb"
	"github.com/xssnick/tonutils-go/ton/nft"

	"github.com/iam047801/tonidx/internal/core"
)

func getContentDataNFT(ret *core.AccountData, c nft.ContentAny) {
	switch content := c.(type) {
	case *nft.ContentSemichain: // TODO: remove this (?)
		ret.ContentURI = content.URI
		ret.ContentName = content.Name
		ret.ContentDescription = content.Description
		ret.ContentImage = content.Image
		ret.ContentImageData = content.ImageData

	case *nft.ContentOnchain:
		ret.ContentName = content.Name
		ret.ContentDescription = content.Description
		ret.ContentImage = content.Image
		ret.ContentImageData = content.ImageData

	case *nft.ContentOffchain:
		ret.ContentURI = content.URI
	}
}

func getCollectionDataNFT(ret *core.AccountData, data *nft.CollectionData) {
	ret.Types = append(ret.Types, core.NFTCollection)
	ret.NextItemIndex = data.NextItemIndex.Uint64()
	ret.OwnerAddress = data.OwnerAddress.String()
	getContentDataNFT(ret, data.Content)
}

func getRoyaltyDataNFT(ret *core.AccountData, params *nft.CollectionRoyaltyParams) {
	ret.Types = append(ret.Types, core.NFTRoyalty)
	ret.RoyaltyAddress = params.Address.String()
	ret.RoyaltyBase = params.Base
	ret.RoyaltyFactor = params.Factor
}

func getItemDataNFT(ret *core.AccountData, data *nft.ItemData) {
	ret.Types = append(ret.Types, core.NFTItem)
	ret.Initialized = data.Initialized
	ret.ItemIndex = data.Index.Uint64()
	ret.CollectionAddress = data.CollectionAddress.String()
	ret.OwnerAddress = data.OwnerAddress.String()
}

func getEditorDataNFT(ret *core.AccountData, editor *address.Address) {
	ret.Types = append(ret.Types, core.NFTEditable)
	ret.EditorAddress = editor.String()
}

//nolint // TODO: simplify account data parsing logic
func (s *Service) getAccountDataNFT(ctx context.Context, b *tlb.BlockInfo, acc *tlb.Account, types []core.ContractType, ret *core.AccountData) error {
	var collection, item, editable, royalty bool

	addr := acc.State.Address

	for _, t := range types {
		switch t {
		case core.NFTCollection:
			collection = true
		case core.NFTItem, core.NFTItemSBT:
			item = true
		case core.NFTEditable:
			editable = true
		case core.NFTRoyalty:
			royalty = true
		}
	}

	switch {
	case collection:
		c := nft.NewCollectionClient(s.api, addr)

		data, err := c.GetCollectionDataAtBlock(ctx, b)
		if err != nil {
			return errors.Wrap(err, "get collection data")
		}
		getCollectionDataNFT(ret, data)

	case collection && royalty:
		c := nft.NewCollectionClient(s.api, addr)

		params, err := c.RoyaltyParamsAtBlock(ctx, b)
		if err != nil {
			return errors.Wrap(err, "get royalty params")
		}
		getRoyaltyDataNFT(ret, params)

	case item:
		c := nft.NewItemClient(s.api, addr)

		data, err := c.GetNFTDataAtBlock(ctx, b)
		if err != nil {
			return errors.Wrap(err, "get nft item data")
		}
		getItemDataNFT(ret, data)

		if data.Content != nil {
			collect := nft.NewCollectionClient(s.api, data.CollectionAddress)
			con, err := collect.GetNFTContentAtBlock(ctx, data.Index, data.Content, b)
			if err != nil {
				return errors.Wrap(err, "get nft content")
			}
			getContentDataNFT(ret, con)
		}

	case editable:
		c := nft.NewItemEditableClient(s.api, addr)

		editor, err := c.GetEditorAtBlock(ctx, b)
		if err != nil {
			return errors.Wrap(err, "get editor")
		}
		getEditorDataNFT(ret, editor)

	default:
		return errors.Wrap(core.ErrNotAvailable, "get account nft data")
	}

	return nil
}
