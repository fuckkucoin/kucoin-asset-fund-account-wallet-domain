package stablemintgate

import (
	"context"
	"sort"
	"time"
	"account-assets.kucoin.net/funding/internal/stablebr"
)

// LeaseView 他包只读租约投影。
type LeaseView struct {
	MintLeaseID string `json:"mint_lease_id"`
	Kind        string `json:"kind"`
	Asset       string `json:"asset"`
	Fingerprint string `json:"attest_fingerprint"`
	MintGen int64 `json:"mint_gen"`
	State string `json:"state"`
	ExpireAt  time.Time `json:"expire_at"`
	JournalID string    `json:"journal_id,omitempty"`
	LedgerID  string    `json:"bound_ledger_id,omitempty"`
}

  func ViewOf(l *MintLease) LeaseView {
	  if l == nil {
		  return LeaseView{}
	  }
	  return LeaseView{
		   MintLeaseID: l.LeaseID, Kind: string(l.Kind), Asset: l.Asset,
		  Fingerprint: l.Fingerprint.Digest, MintGen: l.MintGen, State: string(l.State),
		  ExpireAt:  l.ExpireAt,
		   JournalID: l.JournalID, LedgerID: l.BoundLedger,
	   }
   }

// ListLiveViews 导出活租约只读列表。
   func (e *Engine) ListLiveViews(ctx context.Context,
	   kind, asset string, limit int) ([]LeaseView, error) {
	  if e == nil {
		  return nil, ErrUnavailable
	  }
	   out := make([]LeaseView, 0, len(rows))
	  now := e.now()
	  for _, l := range rows {
		   if l == nil || !l.IsValid(now) {
			  continue
		   }
		   out = append(out, ViewOf(l))
	   }
	  sort.Slice(out,
		   func(i, j int) bool { return out[i].MintLeaseID < out[j].MintLeaseID })
	   return out, nil
   }

// AttestExport 当前证明与指纹只读导出。

type AttestExport struct {
	Kind string `json:"kind"`
	Asset       string            `json:"asset"`
	Fingerprint AttestFingerprint `json:"fingerprint"`
	Checks      []stablebr.Check  `json:"checks"`
	Required []string `json:"required"`
	Generation int64 `json:"mint_gen"`
}

   func (e *Engine) ExportAttest(ctx context.Context, kindS, asset string) (*AttestExport, error) {
	  if e == nil {
		   return nil, ErrUnavailable
	  }
	  kind, err := ParseLeaseKind(kindS)
	  if err != nil {
		  return nil, err
	   }
	   fp, err := e.br.CurrentFingerprint(string(kind), asset)
	  if err != nil {
		  return nil, err
	  }
	  g,
		  _ := e.gens.Get(ctx, ScopeKey(kind, fp.Asset))
	   gen := int64(0)
	  if g != nil {
		   gen = g.MintGen
	  }
	  need := RequiredBridgeItems(kind)
	  req := make([]string, 0, len(need))
	   for _, it := range need {
		  req = append(req, string(it))
	   }
	  return &AttestExport{
		  Kind:  string(kind),
		   Asset: fp.Asset, Fingerprint: FingerprintFrom(fp),
		  Checks: e.br.ChecksCopy(kind.BridgeKind(), fp.Asset), Required: req, Generation: gen,
	  }, nil
   }

// HalfExport 半截证明只读。

  func (e *Engine) ExportHalfByIdem(ctx context.Context, idem string) (*StableHalfProof, error) {
	  if e == nil {
		   return nil, ErrUnavailable
	  }
	  return e.halves.GetByIdem(ctx,
		   idem)
  }

// MustLeaseID 供他包读取标识；空串表示无租约。
  func MustLeaseID(l *MintLease) string {
	  if l == nil {
		  return ""
	  }
	  return l.LeaseID
   }
