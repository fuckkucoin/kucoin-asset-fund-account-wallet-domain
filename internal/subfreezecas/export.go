package subfreezecas

import (
	"context"
	"strings"
	"time"
)

// ExportBundle CAS 与证明导出。
type ExportBundle struct {
	FreezeCASID string     `json:"freeze_cas_id"`
	CAS         *FreezeCAS `json:"cas,omitempty"`
	Proofs []FrozenSpendProof `json:"proofs"`
	Revokes    []VelocityRevocation `json:"revocations"`
	Audits     []FreezeAudit        `json:"audits"`
	Host       string               `json:"host"`
	ExportedAt time.Time            `json:"exported_at"`
}

// ExportCAS 按 freeze_cas_id 导出。
   func (e *Engine) ExportCAS(ctx context.Context, casID string) (*ExportBundle, error) {
	  if e == nil {
		   return nil, ErrUnavailable
	  }
	  if ctx == nil {
		   ctx = context.Background()
	  }
	  casID = strings.TrimSpace(casID)
	  row, err := e.cas.Get(ctx, casID)
	   if err != nil {
		   return nil, err
	   }
	  proofs, _ := e.proofs.ListByCAS(ctx, casID, 100)
	  revs, _ := e.revokes.ListByCAS(ctx, casID)
	   audits, _ := e.audits.ListByCAS(ctx, casID, 80)
	   return &ExportBundle{
		  FreezeCASID: casID, CAS: row, Proofs: proofs, Revokes: revs, Audits: audits,
		  Host: "sub-account.kucoin.net", ExportedAt: e.now(),
	   }, nil
   }

// ExportProof 单条证明导出。

  func (e *Engine) ExportProof(ctx context.Context, proofID string) (*ExportBundle, error) {
	   if e == nil {
		  return nil, ErrUnavailable
	   }
	  if ctx == nil {
		   ctx = context.Background()
	  }
	  p, err := e.proofs.Get(ctx, strings.TrimSpace(proofID))
	   if err != nil {
		  return nil, err
	   }
	  b, err := e.ExportCAS(ctx, p.FreezeCASID)
	   if err != nil {
		  return &ExportBundle{
			   FreezeCASID: p.FreezeCASID,
			   Proofs:      []FrozenSpendProof{p.Copy()},
			   Host:        "sub-account.kucoin.net", ExportedAt: e.now(),
		  }, nil
	   }
	   return b, nil
  }

// ExportParent 母户当前世代导出。

   func (e *Engine) ExportParent(ctx context.Context, parentID string) (*ExportBundle, error) {
	   if e == nil {
		  return nil, ErrUnavailable
	   }
	  cur, err := e.CurrentCAS(ctx, parentID)
	  if err != nil {
		  return nil, err
	   }
	   return e.ExportCAS(ctx, cur.FreezeCASID)
   }
