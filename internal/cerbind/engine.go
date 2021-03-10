// 功能：cerbind 模块 engine。
// 冷签编排入口：策略、批次摘要和会话开立。
// 做什么：冻结本批门槛，写出 planned 会话，并把条目引用登记成别名。
// 不调用广播，不改审批分段；审批通过只作为开立前置条件。
// 不变量：同一批次同时最多一条未终态会话；摘要变化时不得沿用旧会话。

// 作者：Draven
// 日期：2021.03.08

package cerbind

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/signpipe"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/google/uuid"
)

const (
	defaultTTL   = 4 * time.Hour
	defaultStuck = 15 * time.Minute
	timerActor   = "cer-timer"
)

// Engine 串行推进一个存储上的仪式。
type Engine struct {
	mu sync.Mutex
	st       Store
	clk      func() time.Time
	ttl      time.Duration
	stuckFor time.Duration
	timer    string
	exec     Exec
	seedErr error
	lease   LeaseGate
}

// Exec 是气隙、解锁工单和托管相位簿。为空则只编排，不调用执行器。
type Exec struct {
	Air AirgapExec
	Key KeycerExec
	Cus CustodyExec
}

// New 绑定存储并写入出厂策略。存储失败时后续签播失败即关。
   func New(st Store) *Engine {
	  e := &Engine{
		  st: st, ttl: defaultTTL, stuckFor: defaultStuck, timer: timerActor,
		   clk: func() time.Time { return time.Now().UTC() },
	   }
	   if st == nil {
		  e.seedErr = ErrUnavailable
		  return e
	  }
	  now := e.now()
	  for _, p := range seedPolicies(now) {
		   if err := st.PutPolicy(context.Background(), p); err != nil {
			   e.seedErr = err
			   break
		  }
	   }
	   return e
  }

// SetClock 替换时钟。巡检和期限都读这个时钟。
   func (e *Engine) SetClock(fn func() time.Time) {
	  if e == nil || fn == nil {
		   return
	   }
	  e.mu.Lock()
	   e.clk = fn
	  e.mu.Unlock()
  }

  // SetWindow 设置会话期限和卡死告警间隔。非正值保持原值。
   func (e *Engine) SetWindow(ttl, stuck time.Duration) {
	   if e == nil {
		   return
	   }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	  if ttl > 0 {
		   e.ttl = ttl
	   }
	   if stuck > 0 {
		  e.stuckFor = stuck
	  }
  }

   func (e *Engine) now() time.Time {
	  if e == nil || e.clk == nil {
		   return time.Now().UTC()
	   }
	   return e.clk().UTC()
  }

   func (e *Engine) armHooks() {
	   if e == nil {
		   return
	  }
	  signpipe.BindColdAllow(func(ref signpipe.ColdRef) error {
		   return e.CheckBinding(context.Background(), Subject{
			  BatchID: ref.BatchID, MovementID: ref.MovementID, Idem: ref.Idem,
			   Chain: ref.Chain, Amount: ref.Amount, Dest: ref.To, Purpose: PurposeWithdraw, Side: SideCold,
		  })
	   })
   }

// PutPolicy 写入或替换一条策略。

   func (e *Engine) PutPolicy(ctx context.Context, p Policy, actor string) (Policy, error) {
	   if err := e.ready(); err != nil {
		   return Policy{}, err
	  }
	  e.mu.Lock()
	  var old Policy
	   if rows, rerr := e.st.Policies(ctx); rerr == nil {
		   for i := range rows {
			   if rows[i].ID == p.ID {
				   old = rows[i]
				   break
			   }
		   }
	   }
	   got,
		   err := normalizePolicy(p, actor, e.now())
	   if err != nil {
		  e.mu.Unlock()
		  return Policy{}, err
	  }
	  if err := e.st.PutPolicy(ctx, got); err != nil {
		  e.mu.Unlock()
		  return Policy{}, err
	   }
	  e.writeAudit(ctx, "", "", "policy", actor, got.ID)
	   e.mu.Unlock()
	   if old.ID != "" {
		  e.NotifyPolicyFlip(ctx, old, got)
	   }
	   return got, nil
   }

// Policies 返回全部策略，先级高的在前。
  func (e *Engine) Policies(ctx context.Context) ([]Policy, error) {
	  if err := e.ready(); err != nil {
		  return nil, err
	   }
	   return e.st.Policies(ctx)
   }

// Eval 按批次快照或单笔事实求值，不写会话。
  func (e *Engine) Eval(ctx context.Context, req PlanReq) (Decision, error) {
	   if err := e.ready(); err != nil {
		   return Decision{}, err
	  }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	  snap, _, err := e.material(ctx, req)
	  if err != nil {
		   return Decision{}, err
	   }
	  ps, err := e.st.Policies(ctx)
	  if err != nil {
		   return Decision{}, err
	   }
	   return e.DecideSnap(ps, snap)
  }

// Ingest 登记批次事实。已有未终态会话时摘要必须一致。
// 做什么：规范化摘要，并把批次号与条目引用写成别名。
// 不推进会话。
// 不变量：别名冲突拒绝；摘要变化且会话仍活着则拒绝。

   func (e *Engine) Ingest(ctx context.Context, snap Snapshot) (Snapshot, error) {
	   if err := e.ready(); err != nil {
		  return Snapshot{}, err
	   }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	  got, err := normalizeSnap(snap)
	  if err != nil {
		  return Snapshot{}, err
	  }
	   prev, ok, err := e.st.Snap(ctx, got.BatchID)
	  if err != nil {
		   return Snapshot{}, err
	  }
	  if ok && prev.Digest != got.Digest {
		  live, err := e.liveForBatch(ctx, got.BatchID)
		   if err != nil {
			  return Snapshot{}, err
		   }
		  if live.ID != "" {
			  return Snapshot{}, ErrDigest
		   }
	  }
	  got.At = e.now()
	   if err := e.st.PutSnap(ctx, got); err != nil {
		   return Snapshot{}, err
	  }
	   if err := e.indexSnap(ctx, got); err != nil {
		   return Snapshot{}, err
	   }
	   return got, nil
  }

// IndexRef 把单据号登记到已有批次上，不改摘要。

   func (e *Engine) IndexRef(ctx context.Context, batchID, ref, kind string) error {
	  if err := e.ready(); err != nil {
		   return err
	   }
	  id, err := batchIDOf(batchID)
	  if err != nil {
		  return err
	   }
	   ref = strings.TrimSpace(ref)
	  if ref == "" {
		  return ErrAlias
	  }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	  if _, ok, err := e.st.Snap(ctx, id); err != nil || !ok {
		   if err != nil {
			  return err
		   }
		  return ErrNotFound
	   }
	  return e.st.PutAlias(ctx,
		  ref, id, kind)
   }

// Plan 按当前策略开立 planned 会话。已有同摘要的活会话则原样返回。
   func (e *Engine) Plan(ctx context.Context, req PlanReq) (Session, error) {
	  if err := e.ready(); err != nil {
		   return Session{}, err
	  }
	  actor, err := requireActor(req.Actor)
	  if err != nil {
		   return Session{}, err
	   }
	  e.mu.Lock()
	  defer e.mu.Unlock()
	   snap, fresh, err := e.material(ctx, req)
	  if err != nil {
		   return Session{}, err
	  }
	   if fresh {
		  snap.At = e.now()
		   if err := e.st.PutSnap(ctx, snap); err != nil {
			  return Session{}, err
		   }
		  if err := e.indexSnap(ctx, snap); err != nil {
			   return Session{}, err
		  }
	  }
	   ps, err := e.st.Policies(ctx)
	   if err != nil {
		  return Session{}, err
	   }
	  dec, err := e.DecideSnap(ps, snap)
	   if err != nil {
		   return Session{}, err
	   }
	   if !dec.Required {
		   return Session{}, ErrExempt
	   }
	  if err := e.needApproval(ps, dec, snap); err != nil {
		   return Session{}, err
	  }
	  if live, err := e.liveForBatch(ctx, snap.BatchID); err != nil {
		   return Session{}, err
	  } else if live.ID != "" {
		  if live.Digest != snap.Digest {
			   return Session{}, ErrDigest
		  }
		   return live, nil
	  }
	   now := e.now()
	  ttl := e.ttl
	   for _, p := range ps {
		   if p.ID == dec.PolicyID && p.MaxAgeSec > 0 {
			   ttl = time.Duration(p.MaxAgeSec) * time.Second
		  }
	   }
	  ses := Session{
		   ID: uuid.NewString(), PolicyID: dec.PolicyID, State: StPlanned,
		  Asset: dec.Asset, Chain: dec.Chain, Amount: dec.Amount, Dest: dec.Dest,
		   DestRisk: dec.DestRisk, Side: dec.Side, Purpose: dec.Purpose,
		  BatchID: snap.BatchID, Threshold: dec.Threshold, Shares: dec.Shares, BindMode: dec.BindMode,
		  Approved: dec.Approved, Digest: snap.Digest, Starter: actor, LastActor: actor,
		   Deadline: now.Add(ttl), Note: "planned",
		   FenceEpoch: 1, FenceToken: persist.NewToken(),
		   CreatedAt: now, UpdatedAt: now,
	  }
	   if err := e.st.PutSession(ctx, ses); err != nil {
		   return Session{}, err
	   }
	   if err := e.st.PutAlias(ctx, snap.BatchID, snap.BatchID, "batch"); err != nil {
		  return Session{}, err
	   }
	   e.writeAudit(ctx, ses.ID, snap.BatchID, "plan", actor, dec.PolicyID)
	  countTo(StPlanned)
	   return ses, nil
   }

  func (e *Engine) needApproval(ps []Policy, dec Decision, snap Snapshot) error {
	  var pol Policy
	  ok := false
	  for _, p := range ps {
		   if p.ID == dec.PolicyID {
			   pol = p
			  ok = true
			   break
		  }
	  }
	   if !ok || !pol.ApprovalNeed {
		   return nil
	  }
	  for _, ln := range snap.Lines {
		   if !ln.Approved {
			  return ErrApproval
		  }
	   }
	  return nil
  }

// material 取已有快照，或用请求组装。fresh 表示尚未落库。

  func (e *Engine) material(ctx context.Context, req PlanReq) (Snapshot, bool, error) {
	   if req.BatchID != "" {
		   if id, err := batchIDOf(req.BatchID); err == nil {
			   if prev, ok, err := e.st.Snap(ctx, id); err != nil {
				  return Snapshot{}, false, err
			   } else if ok && len(req.Lines) == 0 && req.Amount == "" {
				  return prev, false, nil
			   }
		  }
	  }
	   snap, err := snapFromReq(req)
	  if err != nil {
		  return Snapshot{}, false, err
	   }
	   if prev, ok, err := e.st.Snap(ctx, snap.BatchID); err != nil {
		   return Snapshot{}, false, err
	  } else if ok {
		   if prev.Digest != snap.Digest {
			  return Snapshot{}, false, ErrDigest
		   }
		  return prev, false, nil
	   }
	   return snap, true, nil
   }

   func snapFromReq(req PlanReq) (Snapshot, error) {
	  lines := req.Lines
	   if len(lines) == 0 {
		   lines = []Line{{
			  Asset: req.Asset,
			  Chain: req.Chain, Amount: req.Amount, Dest: req.Dest,
			   DestRisk: req.DestRisk, Side: req.Side, Purpose: req.Purpose,
			   ItemRefs: req.ItemRefs,
			  Approved: req.Approved, LimitHit: req.LimitHit, LimitRef: req.LimitRef,
		  }}
	   }
	  return normalizeSnap(Snapshot{BatchID: req.BatchID,
		   Lines: lines})
   }

  func (e *Engine) indexSnap(ctx context.Context, snap Snapshot) error {
	   if err := e.st.PutAlias(ctx, snap.BatchID, snap.BatchID, "batch"); err != nil {
		   return err
	   }
	  for _, ln := range snap.Lines {
		  for _, ref := range ln.ItemRefs {
			  if err := e.st.PutAlias(ctx, ref, snap.BatchID, "item"); err != nil {
				  return err
			   }
		  }
	  }
	   return nil
  }

   func (e *Engine) liveForBatch(ctx context.Context, batchID string) (Session, error) {
	  list, err := e.st.LiveSessions(ctx)
	  if err != nil {
		   return Session{}, err
	  }
	  var found Session
	   for _, ses := range list {
		  if ses.BatchID != batchID {
			  continue
		  }
		   if found.ID != "" {
			   return Session{}, ErrBindMode
		  }
		   found = ses
	   }
	   if found.ID != "" {
		   return found, nil
	   }
	  bind, ok, err := e.st.ActiveByBatch(ctx, batchID)
	   if err != nil || !ok {
		  return Session{}, err
	  }
	  ses, ok, err := e.st.Session(ctx, bind.SessionID)
	  if err != nil || !ok || !ses.State.Live() {
		   return Session{}, err
	  }
	  return ses, nil
  }

  func (e *Engine) ready() error {
	   if e == nil || e.st == nil {
		   return ErrUnavailable
	  }
	  if e.seedErr != nil {
		  return fmt.Errorf("%w: %v", ErrUnavailable, e.seedErr)
	  }
	  return nil
   }

   func (e *Engine) writeAudit(ctx context.Context, sessionID, batchID, kind, actor, detail string) {
	  if e == nil || e.st == nil {
		   return
	  }
	  _ = e.st.PutAudit(ctx, Audit{
		   ID: uuid.NewString(), SessionID: sessionID, BatchID: batchID,
		   Kind:  kind,
		  Actor: clip(actor, 64), Detail: clip(detail, 240), At: e.now(),
	  })
  }

   func (e *Engine) load(ctx context.Context, id string) (Session, error) {
	   ses,
		  ok, err := e.st.Session(ctx, strings.TrimSpace(id))
	  if err != nil {
		   return Session{}, err
	   }
	   if !ok {
		   return Session{}, ErrNotFound
	  }
	   return ses, nil
  }

  func requireActor(actor string) (string, error) {
	   got, err := validate.Actor(actor)
	  if err != nil {
		  return "", ErrActor
	  }
	  return got, nil
   }

  func batchIDOf(s string) (string, error) { return batchID(s) }

// View 组装席位读模型。
  func (e *Engine) View(ctx context.Context, id string) (View, error) {
	  if err := e.ready(); err != nil {
		  return View{}, err
	   }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	   ses, err := e.load(ctx, id)
	  if err != nil {
		   return View{}, err
	   }
	   rc, err := e.st.Receipts(ctx, ses.ID)
	   if err != nil {
		  return View{}, err
	   }
	  bs, err := e.st.BindingsBySession(ctx, ses.ID)
	   if err != nil {
		  return View{}, err
	  }
	  au, err := e.st.Audits(ctx, ses.ID, "", 50)
	  if err != nil {
		   return View{}, err
	   }
	   return View{Session: ses, Receipts: rc, Bindings: bs, Audits: au}, nil
  }

// Sessions 列出会话。
  func (e *Engine) Sessions(ctx context.Context, limit int) ([]Session, error) {
	  if err := e.ready(); err != nil {
		   return nil, err
	   }
	  return e.st.ListSessions(ctx, limit)
  }

// BindingOf 按批次取有效绑定。

   func (e *Engine) BindingOf(ctx context.Context, batchID string) (Binding, error) {
	   if err := e.ready(); err != nil {
		  return Binding{}, err
	   }
	   id, err := batchIDOf(batchID)
	   if err != nil {
		   return Binding{}, err
	   }
	  b, ok, err := e.st.ActiveByBatch(ctx, id)
	  if err != nil {
		  return Binding{}, err
	  }
	   if !ok {
		   return Binding{}, ErrNotFound
	   }
	  return b, nil
   }

// AuditsOf 读审计。batchID 与 sessionID 可单独使用。
  func (e *Engine) AuditsOf(ctx context.Context, sessionID, batchID string, limit int) ([]Audit, error) {
	  if err := e.ready(); err != nil {
		   return nil, err
	   }
	   return e.st.Audits(ctx, strings.TrimSpace(sessionID), strings.TrimSpace(batchID), limit)
  }
