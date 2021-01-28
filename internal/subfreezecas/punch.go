package subfreezecas

import (
	"context"
	"errors"
	"strings"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/subacct"
)

   func (e *Engine) recordPunchLocked(ctx context.Context, cas *FreezeCAS, owner, parentID, childID, asset, amount, idem string) (*FrozenSpendProof, error) {
	   if idem != "" {
		   if prev, err := e.proofs.GetByDebitIdem(ctx, idem); err == nil && prev != nil {
			   return prev, nil
		   }
	   }
	  casID := ""
	   if cas != nil {
		   casID = cas.FreezeCASID
	   }
	   p := &FrozenSpendProof{
		   ProofID: uuid.NewString(), FreezeCASID: casID, ParentID: parentID, ChildID: childID,
		   OwnerID: owner,
		  Asset:   asset, Amount: amount, DebitIdem: idem,
		   DetectedAt: e.now(), State: ProofOpen, Note: "frozen spend punch",
	   }
	  if err := e.proofs.Save(ctx, p); err != nil {
		  return nil, err
	   }
	   return p, nil
  }

  func (e *Engine) compensateLocked(ctx context.Context, p *FrozenSpendProof) error {
	   if p == nil {
		   return ErrProofNotFound
	  }
	  if p.Compensated {
		  return nil
	  }
	   key := p.CompensateCreditKey
	  if key == "" {
		   key = "cas-compensate:" + p.ProofID
		   p.CompensateCreditKey = key
	   }
	  e.books.ForgetPost("debit", subacct.PostReq{
		  OwnerID: p.OwnerID, Asset: p.Asset, Amount: p.Amount, IdempotencyKey: p.DebitIdem,
	  })
	  bal, err := e.books.Credit(subacct.PostReq{
		   OwnerID: p.OwnerID, Asset: p.Asset, Amount: p.Amount,
		   Kind: "credit", IdempotencyKey: key,
	  })
	  if err != nil {
		  _ = e.proofs.Save(ctx, p)
		   return ErrCompensate
	  }
	   p.Compensated = true
	  p.RolledBackAvail = bal.Available
	  if err := e.proofs.Save(ctx, p); err != nil {
		  return err
	  }
	  e.audit(ctx, p.FreezeCASID, p.ParentID, p.ChildID, "engine", ActionCompensate, OutcomeCompensated, p.ProofID, 0)
	  recordCompensate()
	   return nil
  }

// InspectPunch 并发击穿巡检：冻后成功借记无证明则补证并回滚。

  func (e *Engine) InspectPunch(ctx context.Context, batch int) (int, error) {
	  if e == nil {
		   return 0, ErrUnavailable
	   }
			   continue
		   }
		   owners := append([]string{cas.ParentID}, cas.ChildIDs...)
		   for _, owner := range owners {
			  rows,
				  err := e.journals.ListByOwner(ctx, owner, cas.FrozenAt, batch)
			   if err != nil {
				   return fixed, err
			   }
			   for _, j := range rows {
				  if !j.OK {
					  continue
				   }
				  if !j.AfterFrozen && !j.At.Before(cas.FrozenAt) {
					   // 冻点之后的成功借记视为击穿
				  } else if j.At.Before(cas.FrozenAt) {
					   continue
				  }
				  if j.Idem != "" {
					   if _, err := e.proofs.GetByDebitIdem(ctx, j.Idem); err == nil {
						  continue
					   }
				   }
				   proof, err := e.recordPunchLocked(ctx, &cas, j.OwnerID, cas.ParentID, j.ChildID, j.Asset, j.Amount, j.Idem)
				  if err != nil {
					  return fixed, err
				  }
				   if err := e.compensateLocked(ctx, proof); err != nil {
					  return fixed, err
				  }
				   fixed++
				   recordPunch()
			  }
		   }
	   }
	  return fixed, nil
   }

// GetProof 查询冻后借记证明。

  func (e *Engine) GetProof(ctx context.Context, proofID string) (*FrozenSpendProof, error) {
	   if e == nil {
		  return nil, ErrUnavailable
	   }
	  if ctx == nil {
		  ctx = context.Background()
	  }
	  return e.proofs.Get(ctx, strings.TrimSpace(proofID))
   }

// ListOpenProofs 未 ack 证明。
   func (e *Engine) ListOpenProofs(ctx context.Context, limit int) ([]FrozenSpendProof, error) {
	  if e == nil {
		  return nil, ErrUnavailable
	   }
	  return e.proofs.ListOpen(ctx, limit)
  }

// AckProof 运维确认证明。

  func (e *Engine) AckProof(ctx context.Context, proofID, actor, note string) (*FrozenSpendProof, error) {
	  if e == nil {
		  return nil, ErrUnavailable
	   }
	  if ctx == nil {
		   ctx = context.Background()
	   }
	  actor = strings.TrimSpace(actor)
	  if actor == "" {
		  return nil, ErrActor
	  }
	  e.mu.Lock()
	  defer e.mu.Unlock()
	  p, err := e.proofs.Get(ctx, strings.TrimSpace(proofID))
	  if err != nil {
		  return nil, err
	   }
	   if p.State == ProofAcked {
		   return p, ErrProofAcked
	   }
	   if !p.Compensated {
		  if err := e.compensateLocked(ctx, p); err != nil {
			  return nil, err
		  }
	  }
	   if err := TransitionProof(p, ProofAcked); err != nil {
		  return nil, err
	  }
	   p.AckedBy = actor
	  p.AckedAt = e.now()
	   if n := strings.TrimSpace(note); n != "" {
		   p.Note = n
	   }
	  if err := e.proofs.Save(ctx, p); err != nil {
		  return nil, err
	   }
	  e.audit(ctx, p.FreezeCASID, p.ParentID, p.ChildID, actor, ActionProofAck, OutcomeOK, p.ProofID, 0)
	  recordAck()
	   return p, nil
  }

// ListProofsByCAS 按 freeze_cas_id 列证明。
   func (e *Engine) ListProofsByCAS(ctx context.Context, casID string, limit int) ([]FrozenSpendProof, error) {
	  if e == nil {
		  return nil, ErrUnavailable
	   }
	   return e.proofs.ListByCAS(ctx, strings.TrimSpace(casID), limit)
   }

   func proofIdemConflict(err error) bool {
	   return errors.Is(err, ErrIdemConflict)
  }

// SeedJournal 接入外部借记轨迹，供巡检冻点。路径由 DebitAssertCAS 写入。
   func (e *Engine) SeedJournal(ctx context.Context, row DebitJournal) error {
	  if e == nil {
		   return ErrUnavailable
	   }
	   if ctx == nil {
		  ctx = context.Background()
	   }
	   if row.JournalID == "" {
		  row.JournalID = uuid.NewString()
	  }
	  if row.At.IsZero() {
		  row.At = e.now()
	  }
	   return e.journals.Save(ctx, &row)
  }
