package destlimit

import (
	"context"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/riskpack"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/google/uuid"
)

// Engine 限额占用账本，背后挂风险图谱。

type Engine struct {
	mu    sync.Mutex
	graph *riskpack.Store
	policies map[string]*Policy
	holds map[string]*Hold
	byReq map[string]string
	overrides map[string]*Override
	audits  []Audit
	signals []Signal
	sql *persist.SQLDB
	clock   func() time.Time
	holdTTL time.Duration
	generation int64
	closed bool
}

   func New() *Engine {
	   return &Engine{
		  graph:    riskpack.NewStore(),
		   policies: map[string]*Policy{}, holds: map[string]*Hold{},
		  byReq: map[string]string{}, overrides: map[string]*Override{},
		  clock:   func() time.Time { return time.Now().UTC() },
		  holdTTL: 30 * time.Minute,
	   }
  }

  func (e *Engine) WithClock(c func() time.Time) *Engine {
	  if e == nil || c == nil {
		  return e
	  }
	   e.clock = c
	   if e.graph != nil {
		  e.graph.WithClock(c)
	   }
	   return e
   }

  func (e *Engine) SetHoldTTL(d time.Duration) *Engine {
	  if e != nil && d > 0 {
		   e.holdTTL = d
	  }
	   return e
  }

  func (e *Engine) Graph() *riskpack.Store {
	  if e == nil {
		  return nil
	   }
	  return e.graph
   }

   func (e *Engine) now() time.Time {
	   if e == nil || e.clock == nil {
		   return time.Now().UTC()
	   }
	   return e.clock().UTC()
  }

  func (e *Engine) Close() {
	  if e == nil {
		  return
	   }
	   e.mu.Lock()
	  e.closed = true
	  e.mu.Unlock()
  }

// Admit 占用一笔额度。超限返回 DenyError；引擎关闭或图谱缺失返回 ErrUnavailable。

  func (e *Engine) Admit(ctx context.Context, req AdmitReq) (Hold, error) {
	  _ = ctx
	  if e == nil || e.graph == nil {
		   return Hold{}, ErrUnavailable
	   }
	   uid,
		  err := validate.UserID(req.UserID)
	   if err != nil {
		   return Hold{}, err
	   }
	  as, err := validate.Asset(req.Asset)
	  if err != nil {
		  return Hold{}, err
	  }
	   amt, err := money.Parse(req.Amount)
	  if err != nil {
		  return Hold{}, err
	   }
	  ch, err := validate.Chain(req.Chain)
	  if err != nil {
		  return Hold{}, err
	  }
	  dest,
		  err := validate.Address(ch, req.Dest)
	  if err != nil {
		  dest = strings.TrimSpace(req.Dest)
		  if dest == "" || len(dest) < 8 {
			  return Hold{}, err
		   }
	   }
	   idem := strings.TrimSpace(req.Idem)
	  if idem == "" {
		   return Hold{}, ErrPolicy
	  }
	  hint := strings.ToLower(strings.TrimSpace(req.KYTHint))
	   e.mu.Lock()
	  defer e.mu.Unlock()
	   if e.closed {
		  return Hold{}, ErrUnavailable
	   }
	  if idem != "" {
		  if id, ok := e.byReq[idem]; ok {
			   if h := e.holds[id]; h != nil && (h.State == HoldReserved || h.State == HoldConsumed) {
				   if hint != "" {
					  if _, nerr := e.graph.NoteKYT(ch, dest, hint); nerr != nil {
						  return Hold{}, ErrUnavailable
					  }
				  }
				  cp := *h
				  cp.Reused = true
				  return cp, nil
			   }
		  }
	  }
	   var node riskpack.DestNode
	  if hint != "" {
		   node, err = e.graph.NoteKYT(ch, dest, hint)
	   } else {
		   node, err = e.graph.UpsertNode(ch, dest, "")
		  if err == nil {
			   if _, rerr := e.graph.Recompute(e.now()); rerr != nil {
				   err = rerr
			   } else if n, ok := e.graph.Node(ch, dest); ok {
				   node = n
			  }
		   }
	   }
	   if err != nil {
		  return Hold{}, ErrUnavailable
	   }
	   exp, err := e.graph.ExposureOf(ch, dest, as)
	  if err != nil {
		  return Hold{}, ErrUnavailable
	  }
	  tier := exp.Tier
	  if tier == "" {
		  tier = node.Tier
	  }
	  if tier == "" {
		  tier = "low"
	   }
	   score := exp.Score
	  if score == "" {
		   score = node.Score
	  }
	   cluster := exp.ClusterID
	   if cluster == "" {
		  cluster = node.ClusterID
	   }
	  now := e.now()
	   q, err := e.quote(uid, as, ch, dest, cluster, tier, score, now)
	  if err != nil {
		   if de, ok := err.(*DenyError); ok {
			  e.noteDeny(uid, as, cluster, amt.String(), de, now)
		  }
		   return Hold{}, err
	   }
	  if de := decide(q, amt.String()); de != nil {
		   e.noteDeny(uid, as, cluster, amt.String(), de, now)
		  observeDeny(de.Code)
		   return Hold{}, de
	   }
	  hold := Hold{
		  ID: uuid.NewString(), UserID: uid, Asset: as, Chain: ch, Dest: dest,
		  ClusterID: cluster,
		   Amount:    amt.String(), PolicyID: q.PolicyID, Tier: tier,
		   State: HoldReserved, RequestID: idem, Limit: q.ClusterLimit,
		  ReservedAt: now, Deadline: now.Add(e.holdTTL),
	   }
	   saved, err := e.putHold(hold)
	  if err != nil {
		  return Hold{}, ErrUnavailable
	  }
	  e.pushAudit(Audit{
		   Kind: AuditReserve, Subject: saved.ID, Before: "0", After: saved.Amount,
		  Detail: "cluster=" + cluster + " tier=" + tier + " limit=" + q.ClusterLimit, At: now,
	  })
	  e.syncOccupancy(cluster, as)
	  observeAdmit(tier)
	   return saved, nil
  }

   func (e *Engine) noteDeny(user, asset, cluster, amount string, de *DenyError, now time.Time) {
	   if de == nil {
		   return
	  }
	   e.pushAudit(Audit{
		   Kind: AuditDeny, Subject: user + "|" + asset, Before: de.Used, After: de.Limit,
		   Detail: de.Code + " " + de.Reason, At: now,
	   })
	   e.pushSignal(Signal{
		  Kind: SignalOverLimit, Code: de.Code, UserID: user, Asset: asset,
		   ClusterID: cluster, Amount: amount, Limit: de.Limit, At: now,
	   })
  }

  func (e *Engine) Audits(kind string, limit int) []Audit {
	   if e == nil {
		   return nil
	  }
	   if limit <= 0 || limit > 200 {
		  limit = 50
	   }
	   kind = strings.TrimSpace(kind)
	  e.mu.Lock()
	  defer e.mu.Unlock()
	   out := make([]Audit, 0, limit)
	  for i := len(e.audits) - 1; i >= 0 && len(out) < limit; i-- {
		  if kind != "" && e.audits[i].Kind != kind {
			  continue
		  }
		  out = append(out, e.audits[i])
	   }
	   return out
   }

  func (e *Engine) pushAudit(a Audit) {
	   if a.ID == "" {
		  a.ID = uuid.NewString()
	   }
	  if a.At.IsZero() {
		   a.At = e.now()
	   }
	  e.audits = append(e.audits, a)
	  if len(e.audits) > 512 {
		   e.audits = append([]Audit(nil), e.audits[len(e.audits)-512:]...)
	  }
	   _ = e.persistAudit(a)
   }

   func (e *Engine) Signals(limit int) []Signal {
	  if e == nil {
		   return nil
	   }
	   if limit <= 0 || limit > 200 {
		  limit = 50
	  }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	  out := make([]Signal, 0, limit)
	   for i := len(e.signals) - 1; i >= 0 && len(out) < limit; i-- {
		   out = append(out, e.signals[i])
	   }
	  return out
   }

  func (e *Engine) pushSignal(s Signal) {
	  if s.At.IsZero() {
		   s.At = e.now()
	   }
	  if s.Kind == "" {
		  s.Kind = SignalOverLimit
	   }
	  e.signals = append(e.signals, s)
	  if len(e.signals) > 256 {
		   e.signals = append([]Signal(nil), e.signals[len(e.signals)-256:]...)
	  }
  }
