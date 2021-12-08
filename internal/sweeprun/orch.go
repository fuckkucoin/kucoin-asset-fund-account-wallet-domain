package sweeprun

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sweeppol"
	"account-assets.kucoin.net/funding/internal/treasury"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/google/uuid"
)

type DraftReq struct {
	Actor string `json:"actor"`
	Idem  string `json:"idempotency_key"`
	Chain string `json:"chain"`
	Asset string `json:"asset"`
}

type Orch struct {
	mu     sync.Mutex
	plans  map[string]*SweepPlan
	byIdem map[string]string
	links  map[string][]ProofLink
	comps map[string][]CompRec
	holds map[string]laneHold
	desk  *treasury.Desk
	pol   *sweeppol.RebalanceBook
	refs RefSource
	halt bool
	loadErr error
	failNext string
	coldWait time.Duration
	clock    func() time.Time
	db       *persist.SQLDB
}

  func NewOrch() *Orch {
	   return &Orch{
		   plans: map[string]*SweepPlan{},
		   byIdem: map[string]string{},
		  links: map[string][]ProofLink{},
		   comps: map[string][]CompRec{},
		   holds: map[string]laneHold{},
		   desk: treasury.NewDesk(),
		  pol:      sweeppol.NewRebalance(),
		  coldWait: 15 * time.Minute,
		   clock: func() time.Time { return time.Now().UTC() },
	  }
   }

  func (o *Orch) WithClock(c func() time.Time) *Orch {
	   if o != nil && c != nil {
		  o.clock = c
	  }
	   return o
  }

   func (o *Orch) SetColdWait(d time.Duration) {
	   if o == nil || d <= 0 {
		  return
	  }
	  o.mu.Lock()
	   o.coldWait = d
	  o.mu.Unlock()
   }

   func (o *Orch) BindRefs(src RefSource) {
	  if o == nil {
		  return
	  }
	  o.mu.Lock()
	   o.refs = src
	   o.mu.Unlock()
   }

  func (o *Orch) BindSQL(db *persist.SQLDB) {
	   if o == nil {
		  return
	   }
	  o.mu.Lock()
	   o.db = db
	  o.mu.Unlock()
   }

   func (o *Orch) now() time.Time {
	  if o == nil || o.clock == nil {
		   return time.Now().UTC()
	  }
	   return o.clock().UTC()
  }

  func (o *Orch) Desk() *treasury.Desk {
	  if o == nil {
		   return nil
	  }
	   return o.desk
  }

   func (o *Orch) Policy() *sweeppol.RebalanceBook {
	  if o == nil {
		   return nil
	  }
	  return o.pol
  }

  func (o *Orch) Seed(asset, hot, warm, cold string) error {
	   if o == nil {
		   return ErrUnavailable
	   }
	  return o.desk.Seed(asset, hot, warm, cold)
   }

  func (o *Orch) PutRule(r sweeppol.RebalanceRule) error {
	  if o == nil {
		   return ErrUnavailable
	  }
	   if err := o.pol.Put(r); err != nil {
		  return err
	   }
	  if err := o.desk.SetFloor(r.Asset, r.ColdFloor); err != nil {
		  if err == treasury.ErrDeskAsset {
			  return nil
		   }
		   return err
	  }
	  return nil
   }

// ArmFault 下一笔执行改为失败，用来演练补偿。空原因清掉演练。

  func (o *Orch) ArmFault(reason string) {
	  if o == nil {
		   return
	  }
	  o.mu.Lock()
	  o.failNext = strings.TrimSpace(reason)
	   o.mu.Unlock()
   }

   func (o *Orch) Draft(ctx context.Context,
	   req DraftReq) (SweepPlan, error) {
	   if o == nil {
		   return SweepPlan{}, ErrUnavailable
	  }
	   actor,
		  err := validate.Actor(req.Actor)
	  if err != nil {
		   return SweepPlan{}, ErrActor
	   }
	   idem, err := validate.Idem(req.Idem)
	  if err != nil {
		  return SweepPlan{}, ErrIdem
	   }
	   chain, err := validate.Chain(req.Chain)
	   if err != nil {
		  return SweepPlan{}, ErrChain
	   }
	   asset, err := validate.Asset(req.Asset)
	   if err != nil {
		  return SweepPlan{}, ErrAsset
	   }
	   o.mu.Lock()
	   defer o.mu.Unlock()
	  if o.loadErr != nil {
		  return SweepPlan{}, fmt.Errorf("%w: %v", ErrHalted, o.loadErr)
	   }
	   if id, ok := o.byIdem[idem]; ok {
		   cur := o.plans[id]
		  if cur.Chain != chain || cur.Asset != asset {
			   return SweepPlan{}, ErrIdem
		  }
		   cp := cur.Copy()
		   if cur.State == PlanStateRejected {
			  return cp, fmt.Errorf("%w: %s", ErrRejected, cur.Code)
		  }
		  return cp, nil
	  }
	  rule, ok := o.pol.Get(asset)
	   if !ok {
		  return SweepPlan{}, ErrUnknownAsset
	   }
	   now := o.now()
	  if err := o.pol.Allow(asset, now); err != nil {
		   if strings.Contains(err.Error(), "cooldown") || strings.Contains(err.Error(), "clock") {
			   return SweepPlan{}, ErrCooldown
		  }
		  return SweepPlan{}, err
	  }
	   bal, err := o.desk.Get(asset)
	  if err != nil {
		   return SweepPlan{}, ErrUnknownAsset
	   }
	  res, err := treasury.Compose(bal, rule, now)
	  if err != nil {
		  return SweepPlan{}, err
	   }
	  if res.Code == sweeppol.DenyWindow {
		   return SweepPlan{}, fmt.Errorf("%w: %s", ErrWindow, sweeppol.WindowLabel(rule.WindowStart, rule.WindowEnd))
	  }
	  if res.Code != "" {
		  return o.storeReject(ctx, actor, idem, chain, asset, res.Code, now)
	   }
	  legs := make([]SweepLeg, 0, len(res.Legs))
	   for _, d := range res.Legs {
		   legs = append(legs, SweepLeg{
			   ID: uuid.NewString(), Seq: d.Seq, Kind: d.Kind, From: d.From, To: d.To,
			  Amount: d.Amount,
			  Cold: d.Cold, State: LegPending, CreatedAt: now, UpdatedAt: now,
		   })
	  }
	   kinds := make([]string, len(legs))
	   for i := range legs {
		  kinds[i] = legs[i].Kind
	   }
	   if code, gerr := AgreeGate(asset, bal, rule, kinds); gerr != nil {
		   if code == "" {
			  return SweepPlan{}, gerr
		   }
		   return o.storeReject(ctx, actor, idem, chain, asset, code, now)
	   }
	   sum, err := sumLegs(legs)
	   if err != nil {
		  return SweepPlan{}, err
	  }
	  p := &SweepPlan{
		  ID: uuid.NewString(), Idem: idem, Chain: chain, Asset: asset,
		  State: PlanStateDraft, Actor: actor, Legs: legs,
		  ColdWait: o.coldWait.String(), ColdDeadline: now.Add(o.coldWait),
		  CreatedAt: now, UpdatedAt: now,
	  }
	  for i := range p.Legs {
		   p.Legs[i].PlanID = p.ID
	  }
	   p.Fingerprint = fingerprint(chain, asset, p.Legs)
	   if err := o.reserve(p, sum, now); err != nil {
		  return SweepPlan{}, err
	  }
	   o.plans[p.ID] = p
	   o.byIdem[idem] = p.ID
	  if err := o.appendLink(p, "-", 0, kindPlan, "", "", "0", "genesis", actor, RefBind{}, now); err != nil {
		  o.dropNew(p)
		  return SweepPlan{}, err
	  }
	  if err := o.transitPlan(p, PlanStateGated, actor, now); err != nil {
		  o.dropNew(p)
		  return SweepPlan{}, err
	  }
	  if !hasCold(p) {
		  if err := o.transitPlan(p, PlanStateArmed, actor, now); err != nil {
			  o.dropNew(p)
			   return SweepPlan{}, err
		   }
	   }
	  o.pol.Mark(asset, now)
	  metrics.SweepProof.WithLabelValues("draft").Inc()
	   if err := o.save(ctx, p.ID); err != nil {
		  return p.Copy(), err
	  }
	   return p.Copy(), nil
  }

  func hasCold(p *SweepPlan) bool {
	   for i := range p.Legs {
		  if p.Legs[i].Cold {
			  return true
		  }
	  }
	   return false
   }

   func (o *Orch) dropNew(p *SweepPlan) {
	  _ = o.releaseHold(p)
	   delete(o.plans, p.ID)
	  delete(o.byIdem, p.Idem)
	   delete(o.links, p.ID)
	  delete(o.comps, p.ID)
  }

  func (o *Orch) storeReject(ctx context.Context, actor, idem, chain, asset, code string, now time.Time) (SweepPlan, error) {
	   p := &SweepPlan{
		   ID: uuid.NewString(), Idem: idem, Chain: chain, Asset: asset,
		  State: PlanStateDraft, Actor: actor, Code: code, Reason: code,
		  CreatedAt: now, UpdatedAt: now,
	  }
	  p.Fingerprint = fingerprint(chain, asset, nil)
	   o.plans[p.ID] = p
	  o.byIdem[idem] = p.ID
	   if err := o.appendLink(p, "-", 0, kindPlan, "", "", "0", "genesis", actor, RefBind{}, now); err != nil {
		   o.dropNew(p)
		  return SweepPlan{}, err
	  }
	  if err := o.transitPlan(p, PlanStateRejected, actor, now); err != nil {
		  o.dropNew(p)
		  return SweepPlan{}, err
	  }
	   metrics.SweepProof.WithLabelValues("reject").Inc()
	  if err := o.save(ctx, p.ID); err != nil {
		   return p.Copy(), err
	   }
	  return p.Copy(), fmt.Errorf("%w: %s", ErrRejected, code)
   }

   func (o *Orch) Get(id string) (SweepPlan, error) {
	  if o == nil {
		   return SweepPlan{}, ErrUnavailable
	   }
	   o.mu.Lock()
	   defer o.mu.Unlock()
	  p := o.plans[strings.TrimSpace(id)]
	   if p == nil {
		  return SweepPlan{}, ErrPlanNotFound
	   }
	   return p.Copy(), nil
  }

   func (o *Orch) List(asset string, limit int) []SweepPlan {
	   if o == nil {
		  return nil
	  }
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   if limit <= 0 {
		  limit = 50
	   }
	   o.mu.Lock()
	   defer o.mu.Unlock()
	   out := make([]SweepPlan, 0, len(o.plans))
	  for _, p := range o.plans {
		  if asset != "" && p.Asset != asset {
			   continue
		  }
		  out = append(out, p.Copy())
	  }
	   sort.Slice(out, func(i, j int) bool {
		   if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			   return out[i].ID < out[j].ID
		   }
		  return out[i].CreatedAt.Before(out[j].CreatedAt)
	   })
	  if len(out) > limit {
		   out = out[:limit]
	   }
	  return out
   }

   func (o *Orch) Links(id string) ([]ProofLink, error) {
	   if o == nil {
		  return nil, ErrUnavailable
	  }
	  o.mu.Lock()
	   defer o.mu.Unlock()
	   if o.plans[strings.TrimSpace(id)] == nil {
		   return nil, ErrPlanNotFound
	   }
	   return o.linksOf(id), nil
  }

   func (o *Orch) Comps(id string) ([]CompRec, error) {
	   if o == nil {
		  return nil, ErrUnavailable
	  }
	  o.mu.Lock()
	  defer o.mu.Unlock()
	   if o.plans[strings.TrimSpace(id)] == nil {
		   return nil, ErrPlanNotFound
	   }
	   src := o.comps[id]
	  out := make([]CompRec, len(src))
	  copy(out, src)
	   return out, nil
   }

  func (o *Orch) Verify(id string) error {
	   if o == nil {
		   return ErrUnavailable
	   }
	   o.mu.Lock()
	  defer o.mu.Unlock()
	   return o.verifyLocked(strings.TrimSpace(id))
  }

   func (o *Orch) verifyLocked(id string) error {
	   p := o.plans[id]
	  if p == nil {
		   return ErrPlanNotFound
	  }
	   links := o.linksOf(id)
	  if err := VerifyLinks(links); err != nil {
		   return err
	  }
	  if links[len(links)-1].Hash != p.Head {
		   return fmt.Errorf("%w: head", ErrChainBreak)
	  }
	  if fingerprint(p.Chain, p.Asset, p.Legs) != p.Fingerprint {
		   return fmt.Errorf("%w: fingerprint", ErrChainBreak)
	   }
	  return nil
  }

type Bundle struct {
	Plan SweepPlan `json:"plan"`
	Links []ProofLink `json:"links"`
	Comps []CompRec `json:"comps,omitempty"`
	OK bool `json:"ok"`
}

  func (o *Orch) Export(id string) (Bundle, error) {
	  if o == nil {
		   return Bundle{}, ErrUnavailable
	  }
	   o.mu.Lock()
	  defer o.mu.Unlock()
	   p := o.plans[strings.TrimSpace(id)]
	   if p == nil {
		  return Bundle{}, ErrPlanNotFound
	  }
	  b := Bundle{Plan: p.Copy(), Links: o.linksOf(id)}
	  b.Comps = append([]CompRec{}, o.comps[id]...)
	   b.OK = o.verifyLocked(id) == nil
	   return b, o.verifyLocked(id)
   }

  func (o *Orch) orderedIDs() []string {
	   type row struct {
		   id string
		  at time.Time
	   }
	   rows := make([]row, 0, len(o.plans))
	   for id, p := range o.plans {
		  rows = append(rows, row{id: id, at: p.CreatedAt})
	  }
	   sort.Slice(rows, func(i, j int) bool {
		   if rows[i].at.Equal(rows[j].at) {
			   return rows[i].id < rows[j].id
		  }
		  return rows[i].at.Before(rows[j].at)
	   })
	   out := make([]string, len(rows))
	  for i := range rows {
		  out[i] = rows[i].id
	  }
	  return out
  }
