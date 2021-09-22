// occseggate.Engine：BeforeFreezeWithGen / CommitAssertGen / AbortAssertGen / ExpireRevokeGens。
// 职责：
// 1. BeforeFreezeWithGen：驱动释放台占用并签发 occ_seg_gen。
// 2. CommitAssertGen / AbortAssertGen：同一 Grant 互斥；必须匹配世代。
// 3. Commit 成功后 Abort 不得静默成功；夹缝落 GhostOccupancyProof。
// 4. ExpireRevokeGens：吊销旧世代；Expire→Pending/再 Admit 后旧 Grant 失败。
// 5. 无 gen、gen 落后、RefuseLiveUnfreeze 一律 fail-closed。
// 只读挂钩：ReleaseView、DeskPort。
// 不改写补偿编排、holdrel、liq_gen、Decision。

// Author: Cason
// Date: 2021.09.04

package occseggate

import (
	"context"
	"errors"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/wdrelease"
)

// Config 引擎配置。
type Config struct {
	// AllowCommitWithoutGen 必须 false。
	AllowCommitWithoutGen bool
	// AllowAbortWithoutGen 必须 false。
	AllowAbortWithoutGen bool
	RevokeBatchSize      int
	ProofBatchSize   int
	InspectBatchSize int
}

  func (c Config) withDefaults() Config {
	  if c.RevokeBatchSize <= 0 {
		  c.RevokeBatchSize = 200
	   }
	   if c.ProofBatchSize <= 0 {
		  c.ProofBatchSize = 200
	  }
	  if c.InspectBatchSize <= 0 {
		   c.InspectBatchSize = 200
	   }
	   return c
   }

// Engine 占用段世代引擎。

type Engine struct {
	store Store
	desk  DeskPort
	view  ReleaseView
	cfg   Config
	clock func() time.Time
}

// NewEngine 初始化。desk 可为空（仅测存储）。
  func NewEngine(store Store, desk DeskPort, cfg Config) *Engine {
	  e := &Engine{store: store, desk: desk, cfg: cfg.withDefaults()}
	   if v, ok := desk.(ReleaseView); ok {
		   e.view = v
	   }
	  return e
   }

// WithClock 时钟。
  func (e *Engine) WithClock(fn func() time.Time) *Engine {
	   e.clock = fn
	   return e
   }

   func (e *Engine) now() time.Time {
	  if e.clock != nil {
		   return e.clock().UTC()
	  }
	   return time.Now().UTC()
  }

  func (e *Engine) audit(ctx context.Context, ticketID, grantIdem, segID, from, to string, gen uint64, actor, note string) {
	   _ = e.store.InsertAudit(ctx, &OccAudit{
		   ID: uuid.NewString(), TicketID: ticketID, GrantIdem: grantIdem, SegID: segID,
		  FromSeg: from,
		   ToSeg:   to, Gen: gen, Actor: actor, Note: note, CreatedAt: e.now(),
	   })
  }

  func (e *Engine) nextGen(ctx context.Context, ticketID, userID string, epoch uint64) (uint64, error) {
	   now := e.now()
	  cur, err := e.store.GetCursor(ctx, ticketID)
	   if err != nil && !errors.Is(err, ErrNotFound) {
		   return 0, err
	  }
	  var next uint64 = 1
	  if err == nil && cur != nil {
		  next = cur.NextGen
		  if cur.Epoch != epoch {
			   next = 1
		   }
	   }
	   if err := e.store.UpsertCursor(ctx, &TicketCursor{
		   TicketID: ticketID, UserID: userID, NextGen: next + 1, Epoch: epoch, UpdatedAt: now,
	  }); err != nil {
		   return 0, err
	   }
	   return next, nil
  }

// OpenAndAdmit 开票并准入（运维/单测入口）。

   func (e *Engine) OpenAndAdmit(ctx context.Context, userID, asset, amount string) (*wdrelease.OccTicket, error) {
	   if e == nil || e.desk == nil {
		  return nil, ErrUnavailable
	   }
	   t,
		   err := e.desk.OpenTicket(ctx, userID, asset, amount)
	   if err != nil {
		   return nil, err
	   }
	   return e.desk.Admit(ctx, t.ID)
   }

// OccOccupyRequest 占用签发请求。
type OccOccupyRequest struct {
	TicketID  string
	GrantIdem string
	Amount string
	Asset string
}

// OccOccupyResult 占用签发结果。
type OccOccupyResult struct {
	Ticket *wdrelease.OccTicket
	Grant *wdrelease.OccGrant
	Seg   *OccSegGen
}

// BeforeFreezeWithGen 占用并签发 occ_seg_gen。
// 不变量：每次成功占用必须有 ticket_id、grant_idem、amount、asset、世代。

   func (e *Engine) BeforeFreezeWithGen(ctx context.Context, req OccOccupyRequest) (*OccOccupyResult, error) {
	   if e == nil || e.store == nil || e.desk == nil {
		  return nil, ErrUnavailable
	  }
	  idem := trim(req.GrantIdem)
	   if idem == "" {
		   return nil, ErrIdemConflict
	  }
	  if existing, err := e.store.GetSegByGrantIdem(ctx, idem); err == nil {
		   t, _ := e.desk.GetTicket(ctx, existing.TicketID)
		  g,
			  _ := e.desk.GetGrantByIdem(ctx, idem)
		  return &OccOccupyResult{Ticket: t, Grant: g, Seg: existing}, nil
	   } else if !errors.Is(err, ErrNotFound) {
		   return nil, err
	   }
	  occ, err := e.desk.BeforeFreeze(ctx, wdrelease.OccOccupyRequest{
		  TicketID:  req.TicketID,
		  GrantIdem: idem, Amount: req.Amount, Asset: req.Asset,
	   })
	  if err != nil {
		   return nil, err
	  }
	   now := e.now()
	  gen,
		   err := e.nextGen(ctx, occ.Ticket.ID, occ.Ticket.UserID, occ.Ticket.Epoch)
	  if err != nil {
		  return nil, err
	  }
	  seg := &OccSegGen{
		  ID: uuid.NewString(), TicketID: occ.Ticket.ID, UserID: occ.Ticket.UserID,
		   GrantID: occ.Grant.ID, GrantIdem: idem, Asset: occ.Grant.Asset, Amount: occ.Grant.Amount,
		   Gen: gen, Epoch: occ.Ticket.Epoch, State: SegReserved,
		   IdemKey: segIdemKey(occ.Ticket.ID, idem, gen, occ.Ticket.Epoch),
		  CreatedAt: now, UpdatedAt: now,
	  }
	   if err := e.store.InsertSeg(ctx, seg); err != nil {
		  return nil, err
	   }
	  e.audit(ctx, seg.TicketID, idem, seg.ID, "", string(SegReserved), gen, "engine", "before_freeze_with_gen")
	  incIssued()
	   return &OccOccupyResult{Ticket: occ.Ticket, Grant: occ.Grant, Seg: seg}, nil
  }

// GenRequest Commit / Abort 世代断言请求。
type GenRequest struct {
	GrantIdem string
	OccSegGen *uint64
	Actor     string
}

// GateResult 闸门结果。
type GateResult struct {
	Allowed bool
	Seg          *OccSegGen
	Grant        *wdrelease.OccGrant
	Proof        *GhostOccupancyProof
	RejectReason string
}

// CommitAssertGen 提交冻结，必须匹配 occ_seg_gen。
// 不变量：
// - 无世代或世代落后 fail-closed。
// - 与 Abort 互斥；占不到锁则落 race 证明。
// - 段已 aborted / expired_revoked 不得成功。
  func (e *Engine) CommitAssertGen(ctx context.Context, req GenRequest) (*GateResult, error) {
	   return e.assertLeg(ctx, req, MutexKindCommit)
  }

// AbortAssertGen 中止占用，必须匹配 occ_seg_gen。
// 不变量：
// - 段已 frozen：必须失败并落 AbortAfterCommit 证明，不得留下可再解冻幽灵段。
// - 与 Commit 互斥。

  func (e *Engine) AbortAssertGen(ctx context.Context, req GenRequest) (*GateResult, error) {
	   return e.assertLeg(ctx,
		   req, MutexKindAbort)
  }

  func (e *Engine) assertLeg(ctx context.Context, req GenRequest, kind string) (*GateResult, error) {
	   if e == nil || e.store == nil || e.desk == nil {
		   return nil, ErrUnavailable
	  }
	  idem := trim(req.GrantIdem)
	  if idem == "" {
		   return nil, ErrIdemConflict
	   }
	   actor := firstNonEmpty(req.Actor,
		   kind)
	  allowMissing := false
	  if kind == MutexKindCommit {
		   allowMissing = e.cfg.AllowCommitWithoutGen
	   } else {
		  allowMissing = e.cfg.AllowAbortWithoutGen
	  }
	  if req.OccSegGen == nil {
		   p, _ := e.recordGhost(ctx, "", "", idem, "", 0, 0, ReasonMissingGen, ProofKindMissing, false, false, "", actor, "missing_gen")
		  incMissing()
		  res := &GateResult{Allowed: false, Proof: p, RejectReason: string(ReasonMissingGen)}
		  if allowMissing && AllowedWithoutGen(kind) {
			   return res, ErrMissingGen
		  }
		   return res, wrapProof(ErrMissingGen, proofID(p))
	   }
	   seg, err := e.store.GetSegByGrantIdem(ctx, idem)
	  if err != nil {
		  if errors.Is(err, ErrNotFound) {
			  p, _ := e.recordGhost(ctx, "", "", idem, "", *req.OccSegGen, 0, ReasonMissingGen, ProofKindMissing, false, false, "", actor, "no_seg")
			  return &GateResult{Allowed: false, Proof: p, RejectReason: string(ReasonMissingGen)}, wrapProof(ErrMissingGen, proofID(p))
		   }
		  return nil, err
	  }
	  if *req.OccSegGen != seg.Gen {
		   p, _ := e.recordGhost(ctx, seg.TicketID, seg.UserID, idem, seg.ID, *req.OccSegGen, seg.Gen, ReasonStaleGen, ProofKindStale, seg.State == SegFrozen, false, string(seg.State), actor, "stale_gen")
		   incStale()
		   return &GateResult{Allowed: false, Seg: seg, Proof: p, RejectReason: string(ReasonStaleGen)}, wrapProof(ErrStaleGen, proofID(p))
	  }
	   if seg.State == SegExpiredRevoked {
		   p, _ := e.recordGhost(ctx, seg.TicketID, seg.UserID, idem, seg.ID, seg.Gen, seg.Gen, ReasonExpireReopen, ProofKindExpire, false, false, string(seg.State), actor, "revoked")
		   return &GateResult{Allowed: false,
			   Seg: seg, Proof: p, RejectReason: string(ReasonExpireReopen)}, wrapProof(ErrRevoked, proofID(p))
	  }
	  t,
		  err := e.desk.GetTicket(ctx, seg.TicketID)
	  if err != nil {
		  return nil, err
	  }
	   if t.Epoch != seg.Epoch {
		  p, _ := e.recordGhost(ctx, seg.TicketID, seg.UserID, idem, seg.ID, seg.Gen, seg.Gen, ReasonEpochMismatch, ProofKindExpire, false, false, string(seg.State), actor, "epoch")
		   return &GateResult{Allowed: false, Seg: seg, Proof: p, RejectReason: string(ReasonEpochMismatch)}, wrapProof(ErrEpochStale, proofID(p))
	  }
	  if kind == MutexKindAbort && seg.State == SegFrozen {
		  p, _ := e.recordGhost(ctx, seg.TicketID, seg.UserID, idem, seg.ID, seg.Gen, seg.Gen, ReasonAbortAfterCommit, ProofKindGhost, true, true, string(seg.State), actor, "abort_after_commit")
		  incGhost()
		  return &GateResult{Allowed: false, Seg: seg, Proof: p, RejectReason: string(ReasonAbortAfterCommit)}, wrapProof(ErrAlreadyFrozen, proofID(p))
	  }
	  if kind == MutexKindCommit && seg.State == SegAborted {
		  p, _ := e.recordGhost(ctx, seg.TicketID, seg.UserID, idem, seg.ID, seg.Gen, seg.Gen, ReasonCommitAfterAbort, ProofKindRace, false, false, string(seg.State), actor, "commit_after_abort")
		  incRace()
		   return &GateResult{Allowed: false, Seg: seg, Proof: p, RejectReason: string(ReasonCommitAfterAbort)}, wrapProof(ErrAlreadyAborted, proofID(p))
	  }
	  owner := kind + ":" + uuid.NewString()
	  if err := e.store.TryAcquireMutex(ctx, &MutexRecord{
		   GrantIdem: idem, Owner: owner, Kind: kind, Gen: seg.Gen, AcquiredAt: e.now(),
	   }); err != nil {
		  p, _ := e.recordGhost(ctx, seg.TicketID, seg.UserID, idem, seg.ID, seg.Gen, seg.Gen, ReasonMutexHeld, ProofKindRace, seg.State == SegFrozen, kind == MutexKindAbort, string(seg.State), actor, "mutex")
		   incRace()
		   return &GateResult{Allowed: false, Seg: seg, Proof: p, RejectReason: string(ReasonMutexHeld)}, wrapProof(ErrMutexHeld, proofID(p))
	   }
	   defer func() { _ = e.store.ReleaseMutex(ctx, idem, owner) }()
	   // 占锁后再读，消化夹缝。
	   seg, err = e.store.GetSegByGrantIdem(ctx, idem)
	   if err != nil {
		   return nil, err
	   }
	  if *req.OccSegGen != seg.Gen {
		   p, _ := e.recordGhost(ctx, seg.TicketID, seg.UserID, idem, seg.ID, *req.OccSegGen, seg.Gen, ReasonStaleGen, ProofKindStale, false, false, string(seg.State), actor, "stale_after_lock")
		  return &GateResult{Allowed: false,
			  Seg: seg, Proof: p, RejectReason: string(ReasonStaleGen)}, wrapProof(ErrStaleGen, proofID(p))
	  }
	  if seg.State == SegExpiredRevoked {
		   p, _ := e.recordGhost(ctx, seg.TicketID, seg.UserID, idem, seg.ID, seg.Gen, seg.Gen, ReasonExpireReopen, ProofKindExpire, false, false, string(seg.State), actor, "revoked_locked")
		   return &GateResult{Allowed: false, Seg: seg, Proof: p, RejectReason: string(ReasonExpireReopen)}, wrapProof(ErrRevoked, proofID(p))
	  }
	  if kind == MutexKindAbort && seg.State == SegFrozen {
		   p, _ := e.recordGhost(ctx, seg.TicketID, seg.UserID, idem, seg.ID, seg.Gen, seg.Gen, ReasonAbortAfterCommit, ProofKindGhost, true, true, string(seg.State), actor, "ghost_locked")
		  incGhost()
		  return &GateResult{Allowed: false, Seg: seg, Proof: p, RejectReason: string(ReasonAbortAfterCommit)}, wrapProof(ErrAlreadyFrozen, proofID(p))
	  }
	  if kind == MutexKindCommit && seg.State == SegAborted {
		   p, _ := e.recordGhost(ctx, seg.TicketID, seg.UserID, idem, seg.ID, seg.Gen, seg.Gen, ReasonCommitAfterAbort, ProofKindRace, false, false, string(seg.State), actor, "race_locked")
		  incRace()
		  return &GateResult{Allowed: false,
			   Seg: seg, Proof: p, RejectReason: string(ReasonCommitAfterAbort)}, wrapProof(ErrAlreadyAborted, proofID(p))
	  }
	   target, ok := SegAfterAction(kind)
	  if !ok {
		   return nil, ErrInvalidTransit
	  }
	   if err := AssertSegTransit(seg.State, target); err != nil {
		   p, _ := e.recordGhost(ctx, seg.TicketID, seg.UserID, idem, seg.ID, seg.Gen, seg.Gen, ReasonUnexpectedSeg, ProofKindRace, seg.State == SegFrozen, kind == MutexKindAbort, string(seg.State), actor, "unexpected")
		  return &GateResult{Allowed: false, Seg: seg, Proof: p, RejectReason: string(ReasonUnexpectedSeg)}, wrapProof(ErrUnexpectedSeg, proofID(p))
	   }
	   var grant *wdrelease.OccGrant
	  if kind == MutexKindCommit {
		   grant, err = e.desk.CommitFreeze(ctx, idem)
	  } else {
		   grant, err = e.desk.AbortFreeze(ctx, idem)
	  }
	   if err != nil {
		  reason := ReasonUnexpectedSeg
		   if errors.Is(err, wdrelease.ErrAlreadyFrozen) {
			   reason = ReasonAbortAfterCommit
			  p, _ := e.recordGhost(ctx, seg.TicketID, seg.UserID, idem, seg.ID, seg.Gen, seg.Gen, reason, ProofKindGhost, true, true, string(seg.State), actor, "desk_frozen")
			  incGhost()
			  return &GateResult{Allowed: false, Seg: seg, Proof: p, RejectReason: string(reason)}, wrapProof(ErrAlreadyFrozen, proofID(p))
		   }
		  return nil, err
	  }
	  from := seg.State
	   now := e.now()
	   seg.State = target
	   seg.UpdatedAt = now
	   if err := e.store.UpdateSeg(ctx, seg); err != nil {
		   return nil, err
	   }
	   e.audit(ctx, seg.TicketID, idem, seg.ID, string(from), string(target), seg.Gen, actor, kind+"_ok")
	   if kind == MutexKindCommit {
		  incCommit()
	  } else {
		  incAbort()
	  }
	   return &GateResult{Allowed: true, Seg: seg, Grant: grant}, nil
  }

// ExpireRevokeGens 过期并吊销本 Epoch 全部 occ_seg_gen。
  func (e *Engine) ExpireRevokeGens(ctx context.Context, ticketID, note string) (*ExpireRevocation, error) {
	   if e == nil || e.store == nil || e.desk == nil {
		  return nil, ErrUnavailable
	  }
	   tid := trim(ticketID)
	  t,
		   err := e.desk.GetTicket(ctx, tid)
	  if err != nil {
		  return nil, err
	   }
	  idem := revokeIdemKey(tid, t.Epoch)
	   if existing, err := e.store.GetRevocationByIdem(ctx, idem); err == nil {
		   return existing, nil
	   } else if !errors.Is(err, ErrNotFound) {
		  return nil, err
	   }
	   fromEpoch := t.Epoch
	  if _, err := e.desk.Expire(ctx, tid, note); err != nil {
		   return nil, err
	   }
	   now := e.now()
	   segs, err := e.store.ListSegsByTicket(ctx, tid, 1024)
	   if err != nil {
		  return nil, err
	   }
	   n := 0
	  for _, seg := range segs {
		   if seg.Epoch != fromEpoch {
			   continue
		  }
		  if seg.State == SegExpiredRevoked {
			  continue
		  }
		   if err := AssertSegTransit(seg.State, SegExpiredRevoked); err != nil {
			   continue
		   }
		  from := seg.State
		  seg.State = SegExpiredRevoked
		   seg.UpdatedAt = now
		   if err := e.store.UpdateSeg(ctx, seg); err != nil {
			  return nil, err
		   }
		   e.audit(ctx, tid, seg.GrantIdem, seg.ID, string(from), string(SegExpiredRevoked), seg.Gen, "engine", "expire_revoke")
		  n++
	   }
	   rev := &ExpireRevocation{
		   ID: uuid.NewString(), TicketID: tid, UserID: t.UserID,
		   FromEpoch: fromEpoch, ToEpoch: fromEpoch, RevokedN: n,
		   State: "applied", IdemKey: idem, Note: trim(note),
		  CreatedAt: now, UpdatedAt: now,
	   }
	  if err := e.store.InsertRevocation(ctx, rev); err != nil {
		  if errors.Is(err, ErrDuplicate) {
			   return e.store.GetRevocationByIdem(ctx, idem)
		  }
		  return nil, err
	   }
	  incRevoke(n)
	   return rev, nil
   }

// ReopenAdmit 过期重开并再准入。旧 occ_seg_gen 必须已吊销。
  func (e *Engine) ReopenAdmit(ctx context.Context,
	   ticketID string) (*wdrelease.OccTicket, error) {
	  if e == nil || e.desk == nil {
		  return nil, ErrUnavailable
	   }
	   t,
		   err := e.desk.GetTicket(ctx, ticketID)
	  if err != nil {
		   return nil, err
	  }
	  if t.State != wdrelease.OccTicketExpired {
		  if _, err := e.ExpireRevokeGens(ctx, ticketID, "reopen_prep"); err != nil {
			   return nil, err
		  }
	   }
	  t, err = e.desk.Reopen(ctx, ticketID)
	  if err != nil {
		  return nil, err
	  }
	   return e.desk.Admit(ctx, t.ID)
   }

// RefuseLiveUnfreeze 活链解冻：恒拒绝并落证明。

   func (e *Engine) RefuseLiveUnfreeze(ctx context.Context, grantIdem string) (*GhostOccupancyProof, error) {
	   if e == nil || e.desk == nil {
		  return nil, ErrUnavailable
	   }
	   err := e.desk.TryLiveUnfreeze(ctx,
		  grantIdem)
	   seg, _ := e.store.GetSegByGrantIdem(ctx, trim(grantIdem))
	   ticketID, userID, segID, gen, phase := "", "", "", uint64(0), ""
	   if seg != nil {
		   ticketID, userID, segID, gen, phase = seg.TicketID, seg.UserID, seg.ID, seg.Gen, string(seg.State)
	  }
	  p, perr := e.recordGhost(ctx, ticketID, userID, trim(grantIdem), segID, gen, gen, ReasonLiveUnfreeze, ProofKindLive, false, false, phase, "refuse", "live_unfreeze")
	   incLive()
	  if err == nil {
		   return p, ErrLiveUnfreeze
	   }
	   if perr != nil {
		  return p, perr
	  }
	   return p, wrapProof(ErrLiveUnfreeze, proofID(p))
   }

   func proofID(p *GhostOccupancyProof) string {
	  if p == nil {
		   return ""
	  }
	   return p.ID
   }

// GetSeg 查询占用段。

  func (e *Engine) GetSeg(ctx context.Context, id string) (*OccSegGen, error) {
	   return e.store.GetSegByID(ctx,
		  id)
   }

// GetSegByGrant 按 Grant 查询。
   func (e *Engine) GetSegByGrant(ctx context.Context, grantIdem string) (*OccSegGen, error) {
	  return e.store.GetSegByGrantIdem(ctx, grantIdem)
  }

// ListSegs 按票据列出。
   func (e *Engine) ListSegs(ctx context.Context, ticketID string, limit int) ([]*OccSegGen, error) {
	   return e.store.ListSegsByTicket(ctx, ticketID, limit)
   }

// GetProof 查询证明。
  func (e *Engine) GetProof(ctx context.Context, id string) (*GhostOccupancyProof, error) {
	  return e.store.GetProofByID(ctx, id)
  }

// ListProofs 按状态列出。
  func (e *Engine) ListProofs(ctx context.Context, st ProofState, limit int) ([]*GhostOccupancyProof, error) {
	  return e.store.ListProofsByState(ctx, st, limit)
  }

// GetRevocation 查询吊销。
  func (e *Engine) GetRevocation(ctx context.Context, id string) (*ExpireRevocation, error) {
	   return e.store.GetRevocation(ctx, id)
  }

// ListAudit 审计。

  func (e *Engine) ListAudit(ctx context.Context, ticketID string, limit int) ([]*OccAudit, error) {
	   return e.store.ListAuditByTicket(ctx, ticketID, limit)
  }

// InspectGhost 幽灵段巡检：frozen 段若释放台仍允许 Abort 则证明化。

   func (e *Engine) InspectGhost(ctx context.Context, limit int) (int, error) {
	   if e == nil || e.store == nil {
		   return 0, ErrUnavailable
	   }
	   if limit <= 0 {
		  limit = e.cfg.InspectBatchSize
	   }
	   segs, err := e.store.ListSegsByState(ctx, SegFrozen, limit)
	  if err != nil {
		  return 0, err
	  }
	   n := 0
	  for _, seg := range segs {
		  g, ok := e.lookupGrant(ctx, seg.GrantIdem)
		   abortWould := false
		  if ok && g.State != string(wdrelease.OccGrantFrozen) && g.LedgerTag != "frozen" {
			  abortWould = true
		   }
		  if GhostIfAbortAfterFrozen(seg.State, abortWould) || (seg.State == SegFrozen && abortWould) {
			  _, _ = e.recordGhost(ctx, seg.TicketID, seg.UserID, seg.GrantIdem, seg.ID, seg.Gen, seg.Gen, ReasonInspectedGhost, ProofKindGhost, true, abortWould, string(seg.State), "inspect", "ghost_scan")
			   n++
		  }
	  }
	  addInspect(n)
	   return n, nil
   }

   func (e *Engine) lookupGrant(ctx context.Context, idem string) (wdrelease.OccLiveGrant, bool) {
	   if e.view != nil {
		   return e.view.LookupGrantByIdem(ctx, idem)
	   }
	   if e.desk != nil {
		   g, err := e.desk.GetGrantByIdem(ctx, idem)
		   if err != nil || g == nil {
			   return wdrelease.OccLiveGrant{}, false
		   }
		  return wdrelease.OccLiveGrant{
			  ID:       g.ID,
			   TicketID: g.TicketID, GrantIdem: g.GrantIdem,
			  Asset: g.Asset, Amount: g.Amount, State: string(g.State),
			  Epoch: g.Epoch, LedgerTag: g.LedgerTag,
		  }, true
	  }
	  return wdrelease.OccLiveGrant{}, false
  }

// PumpExpireRevoke 扫描已过期票据补吊销。
  func (e *Engine) PumpExpireRevoke(ctx context.Context, limit int) (int, error) {
	   if e == nil || e.desk == nil {
		   return 0, ErrUnavailable
	   }
	  _ = limit
	   // Desk 无按状态枚举时，巡检已有吊销缺口：reserved 段对应 expired 票据。
	  segs, err := e.store.ListActiveSegs(ctx, e.cfg.RevokeBatchSize)
	   if err != nil {
		   return 0, err
	   }
	   n := 0
	   seen := map[string]struct{}{}
	  for _, seg := range segs {
		   if _, ok := seen[seg.TicketID]; ok {
			  continue
		  }
		   seen[seg.TicketID] = struct{}{}
		   t, err := e.desk.GetTicket(ctx, seg.TicketID)
		   if err != nil {
			   continue
		   }
		   if t.State == wdrelease.OccTicketExpired || t.Epoch != seg.Epoch {
			  if _, err := e.ExpireRevokeGens(ctx, seg.TicketID, "pump"); err == nil {
				   n++
			  }
		   }
	  }
	  addRevokePump(n)
	  return n, nil
   }

// ExportProofs 导出 open 证明为 JSON 友好切片。
  func (e *Engine) ExportProofs(ctx context.Context, limit int) ([]*GhostOccupancyProof, error) {
	  return e.store.ListProofsByState(ctx, ProofOpen, limit)
   }

// AckProof 证明 open → acked。
   func (e *Engine) AckProof(ctx context.Context, id, note string) (*GhostOccupancyProof, error) {
	   p, err := e.store.GetProofByID(ctx, id)
	   if err != nil {
		   return nil, err
	  }
	  if p.State == ProofAcked {
		  return p, nil
	  }
	  if err := AssertProofTransit(p.State, ProofAcked); err != nil {
		   return nil, ErrInvalidTransit
	  }
	  now := e.now()
	  p.State = ProofAcked
	   p.AckedAt = &now
	   p.Note = firstNonEmpty(note, p.Note)
	  p.UpdatedAt = now
	   if err := e.store.UpdateProof(ctx, p); err != nil {
		  return nil, err
	   }
	   incAck()
	  return p, nil
   }

  func (e *Engine) recordGhost(ctx context.Context, ticketID, userID, grantIdem, segID string, expected, actual uint64, reason GhostReason, kind ProofKind, ledgerFrozen, abortWould bool, phase, actor, note string) (*GhostOccupancyProof, error) {
	  idem := ghostIdemKey(ticketID, grantIdem, expected, actual, reason)
	  if existing, err := e.store.GetProofByIdem(ctx, idem); err == nil {
		  return existing, nil
	   } else if err != nil && !errors.Is(err, ErrNotFound) {
		   return nil, err
	  }
	   now := e.now()
	  p := &GhostOccupancyProof{
		  ID: uuid.NewString(), Kind: kind, TicketID: ticketID, UserID: userID,
		   GrantIdem: grantIdem, SegID: segID, ExpectedGen: expected, ActualGen: actual,
		  Reason: reason, State: ProofOpen, IdemKey: idem,
		   Digest:       ghostDigest(ticketID, grantIdem, segID, expected, actual, reason, phase),
		   LedgerFrozen: ledgerFrozen, AbortWouldOK: abortWould, PhaseAtIssue: phase,
		  Actor: actor, Note: note, CreatedAt: now, UpdatedAt: now,
	  }
	   if err := e.store.InsertProof(ctx, p); err != nil {
		  if errors.Is(err, ErrDuplicate) {
			  return e.store.GetProofByIdem(ctx, idem)
		   }
		   return nil, err
	   }
	   return p, nil
  }

// ProveOpen 将 open 证明固化为已导出（状态仍 open，计数导出）。
  func (e *Engine) ProveOpen(ctx context.Context, limit int) (int, error) {
	  list, err := e.store.ListProofsByState(ctx, ProofOpen, limit)
	   if err != nil {
		   return 0, err
	   }
	  addExport(len(list))
	  return len(list), nil
   }
