package assetworker

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

// SQLAA 自 aa_deposits 扫入金认领并标未终态。
type SQLAA struct {
	Repo  *sqlrepo.Deposit
	Books ledger.Store
	Ctx context.Context
}

  func (s SQLAA) ListClaims(limit int) ([]AAClaim, error) {
	   if s.Repo == nil {
		  return nil, ErrRestore
	  }
	   ctx := s.Ctx
	  if ctx == nil {
		  ctx = context.Background()
	  }
	  rows,
		  err := s.Repo.List(ctx, limit)
	   if err != nil {
		   return nil, err
	  }
	   out := make([]AAClaim, 0, len(rows))
	  for _, r := range rows {
		   c := AAClaim{
			   Chain: r.Chain, TxHash: r.TxHash, OutputIndex: r.OutputIndex,
			   UserID: r.UserID, Asset: r.Asset, TransferID: r.TransferID,
			   Phase: PhaseObserving,
		  }
		  if strings.TrimSpace(r.TransferID) != "" && s.Books != nil {
			  tr, err := s.Books.GetTransfer(ctx, r.TransferID)
			   if err == nil && tr != nil && !tr.State.Open() {
				  c.Phase = PhaseCredited
			   } else if err == nil && tr != nil {
				   c.Phase = PhaseCrediting
			  }
		   }
		   out = append(out, c)
	   }
	   return out, nil
  }
