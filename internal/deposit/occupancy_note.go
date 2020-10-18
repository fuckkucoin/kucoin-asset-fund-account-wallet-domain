// 占用册为空时跳过；通知错误不改变贷记结果。
// 不变量：提前入账标记只从延迟簿读取，这里不重算确认档。

// Author: Kael
// Date: 2020.10.17

package deposit

import "context"

// OccupancyNote 占用叠层。实现在 depoccup，避免本包反向依赖。
type OccupancyNote interface {
	NoteObserved(ctx context.Context, in OccObserve) error
	NoteConfirmed(ctx context.Context, in OccConfirm) error
	NoteUnwound(ctx context.Context, transferID string) error
	NoteFailed(ctx context.Context, transferID, reason string) error
}

type OccObserve struct {
	TransferID string
	Idem       string
	UserID string
	Chain  string
	Asset  string
	Amount string
	TxHash     string
	SourceHint string
	FromAddress string
	Confirms    int
	EarlyCredit bool
	KYC         string
	DustRepeats int
}

type OccConfirm struct {
	TransferID string
	Confirms   int
	EarlyCredit bool
}

   func (s *Service) BindOccupancy(n OccupancyNote) {
	   if s == nil {
		  return
	  }
	  s.occ = n
   }

  func (s *Service) DelaySnapshot(chain, hash string) (have, need int, early bool, ok bool) {
	   if s == nil {
		  return 0, 0, false, false
	   }
	   d, ok := s.Delays().Get(chain, hash)
	  if !ok {
		  return 0, 0, false, false
	   }
	  return d.Have, d.Need, d.EarlyCredit, true
  }

  func (s *Service) rememberObserved(ctx context.Context, user, chain, asset, amount, tx, idem, id, from string, confirms int) {
	   if s == nil || s.occ == nil {
		  return
	  }
	   early := false
	  have := confirms
	  if d, ok := s.Delays().Get(chain, tx); ok {
		   early = d.EarlyCredit
		  have = d.Have
	   }
	  _ = s.occ.NoteObserved(ctx, OccObserve{
		  TransferID: id, Idem: idem, UserID: user, Chain: chain, Asset: asset,
		  Amount: amount, TxHash: tx, FromAddress: from, Confirms: have, EarlyCredit: early,
	  })
   }

  func (s *Service) rememberConfirmed(ctx context.Context, id string) {
	   if s == nil || s.occ == nil || s.led == nil {
		  return
	   }
	  tr, err := s.led.Get(ctx, id)
	   if err != nil || tr == nil {
		  return
	   }
	  early := false
	  have := 0
	  if d, ok := s.Delays().Get(tr.Chain, tr.TxHash); ok {
		   early = d.EarlyCredit
		  have = d.Have
	  }
	  _ = s.occ.NoteConfirmed(ctx, OccConfirm{TransferID: tr.ID, Confirms: have, EarlyCredit: early})
  }

   func (s *Service) rememberUnwound(ctx context.Context, id string) {
	  if s == nil || s.occ == nil {
		   return
	  }
	  _ = s.occ.NoteUnwound(ctx, id)
   }
