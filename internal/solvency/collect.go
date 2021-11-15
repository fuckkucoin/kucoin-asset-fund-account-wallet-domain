package solvency

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/google/uuid"
)

  func (p *Pipeline) readLeaves(ctx context.Context, att Attestation, at time.Time) ([]LiabilityLeaf, int64, error) {
	  if p == nil || p.books == nil {
		  return nil, 0, ErrUnavailable
	   }
	  epoch,
		   err := MaxFenceEpoch(ctx, p.books)
	   if err != nil {
		  return nil, 0, err
	   }
	  accs,
		   err := p.books.ListAccounts(ctx, accountScanLimit)
	   if err != nil {
		   return nil, 0, err
	  }
	   if len(accs) >= accountScanLimit {
		   return nil, 0, ErrTruncated
	   }
	   leaves := make([]LiabilityLeaf, 0, len(accs))
	   seen := map[string]struct{}{}
	  for _, a := range accs {
		   if !takeAccount(a) {
			   continue
		   }
		  if !scopeAllows(att.Scope, a.Asset) {
			  continue
		  }
		  lf, ok, err := leafFromAccount(att, a, epoch, at)
		  if err != nil {
			  return nil, 0, err
		   }
		   if !ok {
			   continue
		  }
		   if _, dup := seen[lf.Key()]; dup {
			   return nil, 0, ErrDuplicate
		   }
		   seen[lf.Key()] = struct{}{}
		   leaves = append(leaves, lf)
	  }
	   sortLeaves(leaves)
	  return leaves, epoch, nil
   }

  func takeAccount(a ledger.Account) bool {
	   if a.UserID == "" || a.UserID == "system" {
		  return false
	  }
	   return a.AccountType.UserFacing()
  }

   func leafFromAccount(att Attestation, a ledger.Account, epoch int64, at time.Time) (LiabilityLeaf, bool, error) {
	   uid, err := validate.UserID(a.UserID)
	  if err != nil {
		  return LiabilityLeaf{}, false, ErrLeaf
	   }
	   book,
		  err := NormBook(string(a.AccountType))
	  if err != nil {
		  return LiabilityLeaf{}, false, err
	   }
	  asset, err := validate.Asset(a.Asset)
	  if err != nil {
		  return LiabilityLeaf{}, false, ErrLeaf
	   }
	   avail, err := normAmt(a.Available.String())
	  if err != nil {
		  return LiabilityLeaf{}, false, err
	   }
	   frozen, err := normAmt(a.Frozen.String())
	   if err != nil {
		  return LiabilityLeaf{}, false, err
	   }
	  total, err := addAmt(avail, frozen)
	   if err != nil {
		   return LiabilityLeaf{}, false, err
	  }
	   if total == "0" {
		   return LiabilityLeaf{}, false, nil
	  }
	  lf := LiabilityLeaf{
		  ID: uuid.NewString(), AttestationID: att.ID,
		   UserID: uid, Book: book, Asset: asset,
		  Available: avail, Frozen: frozen, Total: total,
		   FenceEpoch: epoch,
		  FenceToken:  att.FenceToken,
		  CollectedAt: utc(at),
	   }
	  lf.LeafHash = lf.Hex()
	  return lf, true, nil
   }

// Collect 在 building 上落下负债叶与草稿根。
// 根写入后状态仍是 building：没有封存摘要就杀进程，发布必须失败。
// 同一证明单只采集一次；重采会把两次围栏混进同一根，直接拒绝。

   func (p *Pipeline) Collect(ctx context.Context, id string) (Attestation, error) {
	  if p == nil || p.store == nil {
		  return Attestation{}, ErrUnavailable
	   }
	   att, err := p.store.GetAttestation(ctx, id)
	  if err != nil {
		  return Attestation{}, err
	  }
	  if att.Status != StatusBuilding {
		  return att, ErrImmutable
	   }
	  if att.Root != "" {
		  return att, ErrAlreadyCollected
	   }
	  now := p.now()
	  leaves,
		  epoch, err := p.readLeaves(ctx, att, now)
	   if err != nil {
		  return att, err
	  }
	  tr, err := BuildTree(leaves)
	  if err != nil {
		  return att, err
	  }
	   att.Root = tr.Root
	  att.LeafCount = len(leaves)
	   att.BookDigest = BookDigest(leaves)
	  att.FenceEpoch = epoch
	   att.UpdatedAt = now
	  att.Solvency = true
	   att.Deprecated = false
	   if err := p.store.SaveCollection(ctx, att, leaves); err != nil {
		   return att, err
	  }
	   noteCollect(att.Status)
	   return p.store.GetAttestation(ctx, id)
   }
