package feebumpgate

import (
	"context"
	"strings"
	"time"
	"github.com/google/uuid"
)

   func (e *Engine) guardOpenOrphan(ctx context.Context, transferID string) error {
	   proofs, err := e.store.ListProofsByTransfer(ctx, transferID)
	   if err != nil {
		  return err
	   }
	  for _, p := range proofs {
		  if p.BlocksNewSign() {
			   return ErrOrphanOpen
		  }
	   }
	  return nil
  }

  func (e *Engine) openOrphan(ctx context.Context,
	  transferID, genID, prevGenID, occID string, kind ProofKind,
	  quoted, live string, quotedGen, liveGen int64, held int, detail, actor string) RbfOrphanProof {
	  now := e.now()
	   p := RbfOrphanProof{
		  ProofID: uuid.NewString(), TransferID: transferID, GenID: genID, PrevGenID: prevGenID,
		  OccID: occID, Kind: kind, State: OrphanOpen, QuotedHash: quoted, LiveHash: live,
		  QuotedGen: quotedGen, LiveGen: liveGen, HeldOccCount: held, Detail: detail,
		   Actor: actor, At: now, CreatedAt: now,
	  }
	   _ = e.store.SaveProof(ctx, p)
	  e.metric.IncOrphan()
	   return p
   }

   func (e *Engine) recordProof(ctx context.Context, transferID, genID, prevGenID, occID string, kind ProofKind,
	  quoted, live string, quotedGen, liveGen int64, detail, actor string) RbfOrphanProof {
	  now := e.now()
	  st := OrphanClosed
	   if kind == ProofDualOccupancy || kind == ProofRetireFail || kind == ProofOrphanOpen ||
		  kind == ProofSignBlocked || kind == ProofOldBroadcast {
		   st = OrphanOpen
	  }
	   p := RbfOrphanProof{
		  ProofID: uuid.NewString(), TransferID: transferID, GenID: genID, PrevGenID: prevGenID,
		  OccID: occID, Kind: kind, State: st, QuotedHash: quoted, LiveHash: live,
		  QuotedGen: quotedGen, LiveGen: liveGen, Detail: detail, Actor: actor,
		   At: now, CreatedAt: now,
	  }
	  if st == OrphanClosed {
		  t := now
		   p.ClosedAt = &t
	   }
	   _ = e.store.SaveProof(ctx, p)
	  return p
   }

  func (e *Engine) closeOpenOrphans(ctx context.Context, transferID, actor string) int {
	  proofs, err := e.store.ListProofsByTransfer(ctx, transferID)
	  if err != nil {
		  return 0
	   }
	  n := 0
	  now := e.now()
	   for _, p := range proofs {
		  if !p.State.Open() {
			   continue
		   }
		   p.State = OrphanClosed
		  t := now
		   p.ClosedAt = &t
		  p.Actor = actor
		   if err := e.store.CASProofState(ctx, p.ProofID, OrphanOpen, OrphanClosed, p); err == nil {
			  n++
		   }
	   }
	  return n
  }

// DetectOrphans 扫描活跃世代：仅签章未退役或双 held。
  func (e *Engine) DetectOrphans(ctx context.Context, limit int) (int, error) {
	   if e == nil || e.store == nil {
		  return 0, ErrUnavailable
	  }
	  if limit <= 0 {
		  limit = 64
	   }
	  gens, err := e.store.ListActiveGens(ctx, limit)
	  if err != nil {
		  return 0, err
	   }
	   seen := map[string]bool{}
	  n := 0
	   for _, g := range gens {
		  if seen[g.TransferID] {
			  continue
		  }
		   seen[g.TransferID] = true
		   got,
			  err := e.inspectTransfer(ctx, g.TransferID)
		   if err != nil {
			   continue
		  }
		  n += got
	  }
	   return n, nil
   }

   func (e *Engine) inspectTransfer(ctx context.Context, transferID string) (int, error) {
	  held, err := e.store.ListHeldByTransfer(ctx, transferID)
	   if err != nil {
		  return 0, err
	  }
	   gens, err := e.store.ListGens(ctx, transferID)
	  if err != nil {
		  return 0, err
	   }
	  n := 0
	   if len(held) > 1 {
		  latest,
			  _ := e.store.LatestGen(ctx, transferID)
		   _ = e.openOrphan(ctx, transferID, latest.GenID, "", "", ProofDualOccupancy,
			  "", latest.NewBroadcastHash, 0, latest.Generation, len(held),
			  "scanner found dual held occupancy", "scanner")
		  n++
		  return n, nil
	  }
	  for _, g := range gens {
		   if g.State != GenSigned {
			   continue
		  }
		  occ,
			   err := e.store.GetOccByGen(ctx, g.GenID)
		   if err != nil {
			   _ = e.openOrphan(ctx, transferID, g.GenID, "", "", ProofOrphanOpen,
				   "", g.NewBroadcastHash, g.Generation, g.Generation, len(held),
				  "signed gen missing occupancy", "scanner")
			   n++
			   continue
		   }
		  latest, _ := e.store.LatestGen(ctx, transferID)
		  if latest.GenID != g.GenID && occ.State.Held() {
			   _ = e.openOrphan(ctx, transferID, g.GenID, latest.GenID, occ.OccID, ProofOrphanOpen,
				   g.NewBroadcastHash, latest.NewBroadcastHash, g.Generation, latest.Generation, len(held),
				   "signed gen not retired after successor", "scanner")
			  n++
		   }
	  }
	   return n, nil
   }

// RetireExpired 占用超时退役泵。
   func (e *Engine) RetireExpired(ctx context.Context, now time.Time, limit int) (int, error) {
	   if e == nil || e.store == nil {
		  return 0, ErrUnavailable
	  }
	   rows, err := e.store.ListHeldExpired(ctx, now, limit)
	  if err != nil {
		   return 0, err
	   }
	  n := 0
	  for _, occ := range rows {
		   occ.UpdatedAt = now
		   t := now
		   occ.RetiredAt = &t
		   if err := e.store.CASOcc(ctx, occ.OccID, OccHeld, OccRetired, occ); err != nil {
			   continue
		  }
		   if gen, err := e.store.GetGen(ctx, occ.GenID); err == nil && gen.State.CanSupersede() {
			   from := gen.State
			   gen.State = GenSuperseded
			  gen.UpdatedAt = now
			   gen.SupersededAt = &t
			   _ = e.store.CASGen(ctx,
				  gen.GenID, from, GenSuperseded, gen)
		   }
		   _ = e.recordProof(ctx, occ.TransferID, occ.GenID, "", occ.OccID, ProofOccTimeout,
			  occ.BroadcastHash, "", occ.Generation, occ.Generation, "occupancy ttl expired", "retire-pump")
		  e.metric.IncRetire()
		  n++
	   }
	  return n, nil
  }

// ExportProofs 证明导出。

   func (e *Engine) ExportProofs(ctx context.Context, transferID string) ([]RbfOrphanProof, error) {
	  if e == nil || e.store == nil {
		   return nil, ErrUnavailable
	  }
	   if strings.TrimSpace(transferID) == "" {
		  return e.store.ListOpenOrphans(ctx, 200)
	   }
	  return e.store.ListProofsByTransfer(ctx, transferID)
   }

// ListOpenOrphans 运维查询。
   func (e *Engine) ListOpenOrphans(ctx context.Context, limit int) ([]RbfOrphanProof, error) {
	   if e == nil || e.store == nil {
		  return nil, ErrUnavailable
	  }
	  return e.store.ListOpenOrphans(ctx, limit)
   }

// GetGen 查询世代。
   func (e *Engine) GetGen(ctx context.Context, genID string) (FeeBumpGen, error) {
	   if e == nil || e.store == nil {
		   return FeeBumpGen{},
			   ErrUnavailable
	   }
	   return e.store.GetGen(ctx, genID)
   }

// ListGens 按 transfer 列出。

   func (e *Engine) ListGens(ctx context.Context, transferID string) ([]FeeBumpGen, error) {
	   if e == nil || e.store == nil {
		  return nil, ErrUnavailable
	  }
	  return e.store.ListGens(ctx, transferID)
   }
