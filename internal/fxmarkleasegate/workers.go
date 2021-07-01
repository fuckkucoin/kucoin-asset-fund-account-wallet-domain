package fxmarkleasegate

import (
	"context"
	"strings"
	"time"
	"github.com/rs/zerolog"
	"account-assets.kucoin.net/funding/internal/workerloop"
)

type WorkerConfig struct {
	Interval  time.Duration
	BatchSize int
}

   func NewExpireScanJob(eng *Engine, cfg WorkerConfig, log zerolog.Logger) workerloop.Job {
	  if cfg.BatchSize <= 0 {
		   cfg.BatchSize = DefaultExpiryBatch
	   }
	   return func(ctx context.Context) error {
		   if eng == nil {
			  return ErrNilEngine
		  }
		   n, err := eng.ExpireScan(ctx, cfg.BatchSize)
		   if err != nil {
			  return err
		   }
		  if n > 0 {
			   log.Info().Int("expired", n).Msg("fxmarkleasegate 过期租约已封堵")
		  }
		   return nil
	  }
   }

   func NewPutRevokePumpJob(eng *Engine, cfg WorkerConfig, log zerolog.Logger) workerloop.Job {
	   if cfg.BatchSize <= 0 {
		  cfg.BatchSize = DefaultPatrolBatch
	   }
	   return func(ctx context.Context) error {
		  if eng == nil {
			  return ErrNilEngine
		   }
		  n,
			  err := eng.PumpPutRevoke(ctx, cfg.BatchSize)
		  if err != nil {
			   return err
		   }
		   if n > 0 {
			  log.Info().Int("revoked", n).Msg("fxmarkleasegate Put 改价撤销泵")
		  }
		   return nil
	  }
   }

   func NewLiveDriftPatrolJob(eng *Engine, cfg WorkerConfig, log zerolog.Logger) workerloop.Job {
	  if cfg.BatchSize <= 0 {
		  cfg.BatchSize = DefaultPatrolBatch
	  }
	   return func(ctx context.Context) error {
		   if eng == nil {
			   return ErrNilEngine
		  }
		  n, err := eng.PatrolLiveDrift(ctx, cfg.BatchSize)
		  if err != nil {
			   return err
		   }
		   if n > 0 {
			   log.Info().Int("revoked", n).Msg("fxmarkleasegate live 漂移巡检撤销")
		   }
		  return nil
	  }
   }

   func (e *Engine) ExpireScan(ctx context.Context, batch int) (int, error) {
	  if err := e.require(); err != nil {
		  return 0, err
	   }
	   if batch <= 0 {
		  batch = DefaultExpiryBatch
	  }
	   rows,
		   err := e.store.ListExpiredLive(ctx, e.now(), batch)
	  if err != nil {
		   return 0, err
	  }
	   n := 0
	   for i := range rows {
		  l := rows[i]
		  unlock := e.keys.lock(PairKey(l.From, l.To))
		   if _, err := e.expireLease(ctx, &l, DriftExpired, "expire scan"); err == nil {
			  n++
		   }
		   unlock()
	  }
	   return n, nil
  }

   func (e *Engine) OnPutChange(ctx context.Context, asset, usd string) (int, error) {
	   if err := e.require(); err != nil {
		  return 0, err
	   }
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	  now := e.now()
	   prev, _ := e.store.GetGeneration(ctx, asset)
	  seq := prev.Seq + 1
	   if e.reader != nil {
		  if g := e.reader.GenerationOf(asset); g > seq {
			  seq = g
		   }
	   }
	   _,
		   _ = e.store.SaveGeneration(ctx, MarkGeneration{Asset: asset, Seq: seq, LastUSD: strings.TrimSpace(usd), UpdatedAt: now})
	  return e.revokeByAsset(ctx, asset, DriftPutChange, "put price change "+asset+"="+usd, DefaultPatrolBatch)
  }

  func (e *Engine) PumpPutRevoke(ctx context.Context, batch int) (int, error) {
	   if err := e.require(); err != nil {
		   return 0, err
	   }
	   gens, err := e.store.ListGenerations(ctx)
	  if err != nil {
		   return 0, err
	  }
	   n := 0
	   for _, g := range gens {
		  k, err := e.revokeByAsset(ctx, g.Asset, DriftPutChange, "put revoke pump", batch)
		   if err != nil {
			   return n, err
		  }
		  n += k
	   }
	  return n, nil
  }

  func (e *Engine) revokeByAsset(ctx context.Context, asset string, kind DriftKind, detail string, limit int) (int, error) {
	   rows, err := e.store.ListLeasesByAsset(ctx, asset, limit)
	  if err != nil {
		  return 0, err
	   }
	  n := 0
	   for i := range rows {
		  l := rows[i]
		   if !IsLive(l.State) {
			   continue
		  }
		  unlock := e.keys.lock(PairKey(l.From,
			   l.To))
		  live := LiveSnap{From: l.From, To: l.To, Rate: l.Rate, Source: l.Source, Generation: l.Generation}
		   if snap, err := e.readLive(ctx, l.From, l.To, l.Source == SourceLive || l.Source == SourceHTTP); err == nil {
			   live = snap
			  if snap.FromGen == l.FromGen && snap.ToGen == l.ToGen && !OverThreshold(l.Rate, snap.Rate, l.ThresholdBPS) {
				  unlock()
				  continue
			  }
		   }
		  if _, err := e.sealAndMaybeRevoke(ctx, &l, kind, live, detail, true); err == nil {
			  n++
		   }
		   unlock()
	  }
	  return n, nil
   }

   func (e *Engine) PatrolLiveDrift(ctx context.Context, batch int) (int, error) {
	  if err := e.require(); err != nil {
		  return 0, err
	  }
	   if batch <= 0 {
		   batch = DefaultPatrolBatch
	   }
	  rows, err := e.store.ListLive(ctx, batch)
	  if err != nil {
		  return 0, err
	  }
	   n := 0
	   for i := range rows {
		  l := rows[i]
		   unlock := e.keys.lock(PairKey(l.From,
			   l.To))
		  if proof, err := e.recheck(ctx, &l, "", "", "", true); err != nil || proof != nil {
			   n++
		   }
		   unlock()
	  }
	   return n, nil
   }

  func (e *Engine) OpsRevoke(ctx context.Context, req OpsRevokeReq) (OpsRevokeResp, error) {
	   if err := e.require(); err != nil {
		   return OpsRevokeResp{}, err
	  }
	   id, err := requireLeaseID(req.LeaseID)
	  if err != nil {
		   return OpsRevokeResp{}, err
	  }
	  actor, err := requireActor(req.Actor)
	   if err != nil {
		   return OpsRevokeResp{}, err
	   }
	  l, err := e.store.GetLease(ctx, id)
	   if err != nil {
		  return OpsRevokeResp{}, err
	   }
	   if l.State == LeaseConsumed {
		  return OpsRevokeResp{}, ErrOpsRevokeDenied
	  }
	   if l.State == LeaseRevoked {
		  return OpsRevokeResp{LeaseID: l.ID, State: l.State, Revoked: true, Reason: l.RevokeReason}, nil
	   }
	  unlock := e.keys.lock(PairKey(l.From, l.To))
	  defer unlock()
	   reason := strings.TrimSpace(req.Reason)
	   if reason == "" {
		   reason = "ops revoke"
	  }
	  from := l.State
	   if CanRevoke(l.State) {
		  next, err := MustApply(l.State, LeaseEvRevoke)
		   if err != nil {
			   return OpsRevokeResp{}, err
		  }
		  l.State = next
		  l.RevokedAt = stamp(e.now())
		  l.RevokeReason = reason
		  l.UpdatedAt = e.now()
		   if _, err := e.store.SaveLease(ctx, l); err != nil {
			  return OpsRevokeResp{}, err
		  }
		   e.writeAudit(ctx, l.ID, LeaseEvRevoke, from, next, actor, reason)
	  }
	  _, _ = e.sealProof(ctx, &l, DriftRevoked, LiveSnap{From: l.From, To: l.To, Rate: l.Rate, Source: l.Source, Generation: l.Generation}, reason)
	   return OpsRevokeResp{LeaseID: l.ID, State: l.State, Revoked: true, Reason: reason}, nil
   }

  func (e *Engine) ExportProofs(ctx context.Context, req ProofExportReq) (ProofExportResp, error) {
	   if err := e.require(); err != nil {
		   return ProofExportResp{}, err
	   }
	  limit := req.Limit
	   if limit <= 0 {
		   limit = 50
	  }
	   var (
		  proofs []FxMarkDriftProof
		   err error
	   )
	   if strings.TrimSpace(req.LeaseID) != "" {
		  proofs, err = e.store.ListProofsByLease(ctx, req.LeaseID, limit)
	  } else {
		  proofs, err = e.store.ListProofs(ctx, DriftKind(req.Kind), limit)
	   }
	  if err != nil {
		  return ProofExportResp{}, err
	   }
	   return ProofExportResp{Proofs: proofs, Count: len(proofs)}, nil
   }
