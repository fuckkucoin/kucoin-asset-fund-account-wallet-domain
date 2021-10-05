// @FileName: engine.go
// @Summary: 出箱泵投递 Mark 半截与毒丸重投双效果收口引擎。
// 重点：
// TickWithGen 每次 Pending→投递钉 deliver_effect_gen；Publish 成功后 Mark 失败只可续提。
// PoisonReplayAssert 必须 CAS 世代；已 applied 效果键第二次失败并落 OutboxDupProof。
// 风险点：
// 无世代、世代过期、指纹不匹配不得 Mark 为已投递成功业务效果。
// 不改写 outboxonce Halt↔Deliver，不改 Kafka 路由与毒丸主题命名。
// @Author: Jensen
// @Created: 2021.10.04

package pumpmarkfence

import (
	"context"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/persist"
)

// Engine 世代闸与半截续提核心。
type Engine struct {
	box  *persist.Outbox
	gens GenRepo
	halves HalfRepo
	proofs  ProofRepo
	applied AppliedRepo
	audits  AuditRepo
	clock   func() time.Time
	ttl     time.Duration
	mu sync.Mutex
}

  func NewEngine(box *persist.Outbox, gens GenRepo, halves HalfRepo, proofs ProofRepo, applied AppliedRepo, audits AuditRepo) *Engine {
	  if box == nil {
		   box = persist.NewOutbox(nil)
	  }
	   if gens == nil {
		  gens = NewMemGenRepo()
	   }
	   if halves == nil {
		   halves = NewMemHalfRepo()
	   }
	  if proofs == nil {
		   proofs = NewMemProofRepo()
	  }
	  if applied == nil {
		  applied = NewMemAppliedRepo()
	   }
	   if audits == nil {
		  audits = NewMemAuditRepo()
	  }
	   return &Engine{
		   box: box, gens: gens, halves: halves, proofs: proofs, applied: applied, audits: audits,
		   clock: func() time.Time { return time.Now().UTC() },
		   ttl: DefaultTTL,
	  }
  }

   func NewMemEngine() *Engine {
	   return NewEngine(persist.NewOutbox(nil), NewMemGenRepo(), NewMemHalfRepo(), NewMemProofRepo(), NewMemAppliedRepo(), NewMemAuditRepo())
  }

  func (e *Engine) WithClock(c func() time.Time) *Engine {
	  if e != nil && c != nil {
		  e.clock = c
	  }
	  return e
  }

   func (e *Engine) WithTTL(d time.Duration) *Engine {
	  if e != nil && d > 0 {
		  e.ttl = d
	   }
	  return e
  }

   func (e *Engine) Box() *persist.Outbox {
	  if e == nil || e.box == nil {
		   return persist.NewOutbox(nil)
	  }
	   return e.box
  }

   func (e *Engine) now() time.Time {
	  if e == nil || e.clock == nil {
		  return time.Now().UTC()
	   }
	   return e.clock().UTC()
  }

  func (e *Engine) audit(ctx context.Context, genID, outboxID, actor, action, outcome, detail string, gen int64) {
	  if e == nil || e.audits == nil {
		   return
	  }
	   _ = e.audits.Append(ctx, FenceAudit{
		  AuditID: uuid.NewString(), GenID: genID, OutboxID: outboxID,
		   Actor: actor, Action: action, Outcome: outcome, Detail: detail, Generation: gen, At: e.now(),
	   })
  }

  func (e *Engine) Enqueue(ctx context.Context, key, kind, payload string) (persist.OutboxEvent, error) {
	  if e == nil || e.box == nil {
		  return persist.OutboxEvent{}, ErrUnavailable
	  }
	  return e.box.Enqueue(ctx, key, kind, payload)
   }

  func (e *Engine) GetGen(ctx context.Context, genID string) (*DeliverEffectGen, error) {
	   if e == nil {
		  return nil, ErrUnavailable
	  }
	  return e.gens.Get(ctx,
		   strings.TrimSpace(genID))
  }

   func (e *Engine) GetGenByOutbox(ctx context.Context, outboxID string) (*DeliverEffectGen, error) {
	   if e == nil {
		   return nil, ErrUnavailable
	   }
	  return e.gens.GetByOutbox(ctx, strings.TrimSpace(outboxID))
   }

   func (e *Engine) ListProofs(ctx context.Context, outboxID string, limit int) ([]OutboxDupProof, error) {
	   if e == nil {
		  return nil, ErrUnavailable
	  }
	  if strings.TrimSpace(outboxID) != "" {
		  return e.proofs.ListByOutbox(ctx, outboxID, limit)
	   }
	  return e.proofs.ListUnexported(ctx, limit)
  }

   func (e *Engine) ListHalfOpen(ctx context.Context, limit int) ([]HalfMarkCase, error) {
	   if e == nil {
		  return nil, ErrUnavailable
	  }
	  return e.halves.ListOpen(ctx, limit)
  }

   func (e *Engine) ListAudit(ctx context.Context, outboxID string, limit int) ([]FenceAudit, error) {
	  if e == nil {
		   return nil, ErrUnavailable
	   }
	   return e.audits.ListByOutbox(ctx, outboxID, limit)
   }

  func (e *Engine) writeDup(ctx context.Context, gen *DeliverEffectGen, kind ProofKind, actor, detail string) (*OutboxDupProof, error) {
	   if gen == nil {
		  return nil, ErrGenMissing
	  }
	  p := &OutboxDupProof{
		   ProofID: uuid.NewString(), GenID: gen.GenID, OutboxID: gen.OutboxID,
		   EffectKey: gen.EffectKey, Kind: kind, Generation: gen.Generation,
		  PayloadHash: gen.PayloadHash, Actor: actor, Detail: detail, DetectedAt: e.now(),
	  }
	   if err := e.proofs.Save(ctx, p); err != nil {
		  return nil, err
	   }
	   recordDup(string(kind))
	   e.audit(ctx, gen.GenID, gen.OutboxID, actor, ActionDupProof, OutcomeDup, detail, gen.Generation)
	  return p, nil
   }

  func (e *Engine) currentOrIssue(ctx context.Context, ev persist.OutboxEvent, actor string) (*DeliverEffectGen, error) {
	   ph := PayloadHash(ev.Kind, ev.Key, ev.Payload)
	   ek := EffectKey(ev.ID, ph)
	  if prev, err := e.applied.Get(ctx, ek); err == nil && prev != nil {
		  g, gerr := e.gens.Get(ctx, prev.GenID)
		  if gerr == nil && g != nil {
			  return g, ErrAlreadyApplied
		   }
		   return &DeliverEffectGen{
			  GenID: prev.GenID, OutboxID: ev.ID, EffectKey: ek, PayloadHash: ph,
			  Generation: prev.Generation, State: GenMarked, Applied: true,
		   }, ErrAlreadyApplied
	  }
	  if cur, err := e.gens.GetByOutbox(ctx, ev.ID); err == nil && cur != nil {
		   if cur.State == GenExpired || (cur.Expired(e.now()) && cur.State.Live()) {
			  if cur.State.Live() {
				   _ = TransitionGen(cur, GenExpired)
				  cur.ExpiredAt = e.now()
				   _ = e.gens.Save(ctx, cur)
				   recordExpire()
			   }
			  e.audit(ctx, cur.GenID, ev.ID, actor, ActionExpire, OutcomeExpired, "ttl", cur.Generation)
			  return cur, ErrGenExpired
		   }
		   if cur.PayloadHash != "" && !FingerprintMatch(cur.PayloadHash, ph) {
			  return cur, ErrFingerprint
		   }
		  return cur, nil
	  }
	   now := e.now()
	   row := &DeliverEffectGen{
		   GenID: uuid.NewString(), OutboxID: ev.ID, BusinessKey: ev.Key, Kind: ev.Kind,
		  PayloadHash: ph, EffectKey: ek, Generation: 1, State: GenIssued,
		   TTL: e.ttl,
		  Deadline: now.Add(e.ttl), IssuedAt: now, Actor: actor,
	   }
	   if err := e.gens.Save(ctx, row); err != nil {
		   return nil, err
	  }
	   recordIssue()
	   e.audit(ctx, row.GenID, ev.ID, actor, ActionIssueGen, OutcomeOK, "issued", row.Generation)
	   return row, nil
   }

   func (e *Engine) putApplied(ctx context.Context, gen *DeliverEffectGen) error {
	   if gen == nil {
		  return ErrGenMissing
	  }
	   return e.applied.Put(ctx, &AppliedEffect{
		  EffectKey: gen.EffectKey, GenID: gen.GenID, OutboxID: gen.OutboxID,
		   Generation: gen.Generation, AppliedAt: e.now(),
	   })
   }

  func actorOr(s string) string {
	   s = strings.TrimSpace(s)
	  if s == "" {
		  return "pump"
	   }
	  return s
  }
