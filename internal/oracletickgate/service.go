package oracletickgate

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/oracle"
)

// Service 估值 Tick 资金写租约闸。
type Service struct {
	feed *oracle.Feed
	store Store
	policy Policy
	now func() time.Time
}

// New 默认内存仓并挂钩 Feed.OnPut。

   func New(feed *oracle.Feed) *Service {
	   return NewWith(feed, NewMemStore(), DefaultPolicy())
   }

// NewWith 注入仓与策略。
  func NewWith(feed *oracle.Feed, store Store, policy Policy) *Service {
	   if feed == nil {
		   feed = oracle.NewFeed(0)
	   }
	   if store == nil {
		  store = NewMemStore()
	  }
	   s := &Service{feed: feed, store: store, policy: policy, now: func() time.Time { return time.Now().UTC() }}
	   feed.OnPut(s.onTickPut)
	   return s
   }

// WithClock 时钟。

   func (s *Service) WithClock(c func() time.Time) *Service {
	  if s != nil && c != nil {
		  s.now = c
	   }
	  return s
  }

  // Feed 只读价源。
   func (s *Service) Feed() *oracle.Feed {
	  if s == nil || s.feed == nil {
		  return oracle.NewFeed(0)
	   }
	   return s.feed
   }

// Store 仓。
  func (s *Service) Store() Store { return s.store }

   func (s *Service) clock() time.Time {
	   if s == nil || s.now == nil {
		   return time.Now().UTC()
	   }
	  return s.now().UTC()
   }

// IssueTickLease 资金副作用前签发 tick_lease_id。
// 缺价 Convert 不得签发 Enforce 租约。Shadow/Probe 可探活但不可写。
  func (s *Service) IssueTickLease(ctx context.Context, req IssueTickLeaseRequest) (*TickLease, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	   }
	   actor, err := requireActor(req.Actor)
	  if err != nil {
		  return nil, err
	  }
	   kind, err := normalizeKind(req.ValuationKind)
	   if err != nil {
		  return nil, err
	  }
	  mode, err := normalizeMode(req.Mode)
	  if err != nil {
		  return nil, err
	  }
	  if mode == ModeShadow && !s.policy.AllowShadow {
		  return nil, ErrMode
	  }
	  if mode == ModeProbe && !s.policy.AllowProbe {
		  return nil, ErrMode
	   }
	  ttl, err := s.policy.TTL(req.TTLSeconds)
	  if err != nil {
		  return nil, err
	  }
	   if key := strings.TrimSpace(req.IdemKey); key != "" {
		  if id, ok, _ := s.store.GetIdem(ctx, key); ok {
			  return s.store.GetLease(ctx, id)
		  }
	  }
	  now := s.clock()
	   lease := TickLease{
		  LeaseID:       uuid.NewString(),
		  ValuationKind: kind,
		  Mode:          mode,
		  TTLSeconds:    int64(ttl / time.Second),
		   IssuedAt:      now,
		  ExpiresAt:     now.Add(ttl),
		  State:         LeaseIssued,
		  Actor:         actor,
		  IdemKey:       strings.TrimSpace(req.IdemKey),
	   }
	   switch kind {
	   case ValConvert:
		   if err := s.fillConvert(ctx, &lease, req, mode); err != nil {
			   return nil, err
		   }
	   case ValMTM:
		   if err := s.fillMTM(ctx, &lease, req, mode); err != nil {
			   return nil, err
		  }
	   case ValLTV:
		  if err := s.fillLTV(ctx, &lease, req, mode); err != nil {
			   return nil, err
		   }
	  }
	   if err := s.store.InsertLease(ctx, lease); err != nil {
		  return nil, err
	  }
	   _ = s.store.UpsertGeneration(ctx, TickGeneration{
		   Pair: lease.Pair, Generation: lease.Generation, Price: lease.Price, TickAt: lease.TickAt, UpdatedAt: now,
	   })
	  s.audit(ctx, lease.LeaseID, lease.Pair, "issue", actor, "", string(LeaseIssued), mode.stringNote(kind))
	  if key := strings.TrimSpace(req.IdemKey); key != "" {
		  _ = s.store.PutIdem(ctx, key, lease.LeaseID)
	  }
	  cp := lease
	   return &cp, nil
  }

   func (m LeaseMode) stringNote(k ValuationKind) string {
	   return string(m) + "/" + string(k)
   }

   func (s *Service) fillConvert(ctx context.Context, lease *TickLease, req IssueTickLeaseRequest, mode LeaseMode) error {
	   as, q, err := requirePair(req.Asset, req.Quote)
	   if err != nil {
		  return err
	   }
	   t, err := liveTick(s.feed, as, q)
	  if err != nil {
		   if mode == ModeEnforce {
			  s.openProof(ctx, TickDriftProof{
				  ProofID: uuid.NewString(), Pair: oracle.PairKey(as, q), Kind: ProofMissingPriceIssue,
				  FailClosed: true, Summary: "missing price; enforce lease not issued", OpenedAt: s.clock(),
				   FundWriteHeld: true,
			   })
			  return fmt.Errorf("%w: %v", ErrMissingPriceIssue, err)
		   }
		  return fmt.Errorf("%w: %v", ErrTickMissing, err)
	   }
	  if _, _, cerr := s.feed.Convert(as, nzAmt(req.Amount), q); cerr != nil && mode == ModeEnforce {
		  return fmt.Errorf("%w: %v", ErrMissingPriceIssue, cerr)
	  }
	   lease.Asset,
		   lease.Quote, lease.QuoteCcy = as, q, q
	   lease.Pair = oracle.PairKey(as, q)
	  lease.Price = t.Price
	   lease.TickAt = t.At
	   lease.Fingerprint = FingerprintOf(t)
	  lease.Generation = s.feed.Generation(as, q)
	  lease.Amount = nzAmt(req.Amount)
	   return nil
  }

  func (s *Service) fillMTM(ctx context.Context, lease *TickLease, req IssueTickLeaseRequest, mode LeaseMode) error {
	   q := strings.ToUpper(strings.TrimSpace(req.Quote))
	   if q == "" {
		   q = "USDT"
	   }
	   amounts := req.Amounts
	  if len(amounts) == 0 && req.Asset != "" {
		   amounts = map[string]string{req.Asset: nzAmt(req.Amount)}
	   }
	   if len(amounts) == 0 {
		  return ErrPair
	   }
	  mark, err := s.feed.MarkToMarket(q, amounts)
	   if err != nil {
		  if mode == ModeEnforce {
			  s.openProof(ctx, TickDriftProof{
				   ProofID: uuid.NewString(), Pair: q, Kind: ProofMissingPriceIssue,
				   FailClosed: true, Summary: "mtm missing price; enforce lease not issued", OpenedAt: s.clock(),
				   FundWriteHeld: true,
			  })
			  return fmt.Errorf("%w: %v", ErrMissingPriceIssue, err)
		   }
		  return err
	  }
	  // 钉第一行 Tick 作为主指纹；多资产写路径仍复验 Feed 世代。
	   var t oracle.Tick
	  if len(mark.Lines) > 0 {
		  t = mark.Lines[0].Tick
	   }
	  if t.Asset == "" {
		  return ErrTickMissing
	   }
	  lease.Asset, lease.Quote, lease.QuoteCcy = t.Asset, t.Quote, q
	   lease.Pair = oracle.PairKey(t.Asset, t.Quote)
	   lease.Price = t.Price
	   lease.TickAt = t.At
	  lease.Fingerprint = FingerprintOf(t)
	   lease.Generation = s.feed.Generation(t.Asset,
		  t.Quote)
	  lease.MTMTotal = mark.Total
	  lease.Amount = nzAmt(req.Amount)
	  return nil
   }

   func (s *Service) fillLTV(ctx context.Context, lease *TickLease, req IssueTickLeaseRequest, mode LeaseMode) error {
	  q := strings.ToUpper(strings.TrimSpace(req.Quote))
	   if q == "" {
		  q = "USDT"
	   }
	   colA, colQ := strings.TrimSpace(req.ColAsset), q
	   if req.ColAsset == "" {
		  return ErrPair
	  }
	   bps, err := s.feed.MarginLTV(req.BorrowAsset, nzAmt(req.BorrowAmt), req.ColAsset, nzAmt(req.ColAmt), q)
	   if err != nil {
		   if mode == ModeEnforce {
			  s.openProof(ctx, TickDriftProof{
				  ProofID: uuid.NewString(), Pair: oracle.PairKey(colA, colQ), Kind: ProofMissingPriceIssue,
				  FailClosed: true, Summary: "margin ltv missing price; enforce lease not issued", OpenedAt: s.clock(),
				   FundWriteHeld: true,
			   })
			   return fmt.Errorf("%w: %v", ErrMissingPriceIssue, err)
		   }
		   return err
	  }
	  t, terr := liveTick(s.feed, req.ColAsset, q)
	   if terr != nil {
		  if mode == ModeEnforce {
			   return fmt.Errorf("%w: %v", ErrMissingPriceIssue, terr)
		  }
		  return terr
	   }
	   lease.Asset, lease.Quote, lease.QuoteCcy = t.Asset, t.Quote, q
	   lease.Pair = oracle.PairKey(t.Asset, t.Quote)
	   lease.Price = t.Price
	   lease.TickAt = t.At
	  lease.Fingerprint = FingerprintOf(t)
	   lease.Generation = s.feed.Generation(t.Asset, t.Quote)
	  lease.BorrowAsset, lease.BorrowAmt = req.BorrowAsset, nzAmt(req.BorrowAmt)
	  lease.ColAsset, lease.ColAmt = req.ColAsset, nzAmt(req.ColAmt)
	  lease.LTVBPS = bps
	   return nil
   }

  func nzAmt(s string) string {
	  if strings.TrimSpace(s) == "" {
		  return "1"
	   }
	   return s
  }

// GetLease 只读。

   func (s *Service) GetLease(ctx context.Context, id string) (*TickLease, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	   }
	  return s.store.GetLease(ctx, id)
  }

// View 组合视图。
   func (s *Service) View(ctx context.Context, leaseID string) (*LeaseView, error) {
	   l, err := s.GetLease(ctx, leaseID)
	  if err != nil {
		   return nil, err
	   }
	   v := &LeaseView{Lease: l}
	   if g, err := s.store.GetGeneration(ctx, l.Pair); err == nil {
		  v.Gen = g
	  }
	  v.Proofs, _ = s.store.ListProofsByLease(ctx, leaseID)
	   v.Audits, _ = s.store.ListAuditsByLease(ctx, leaseID, 50)
	  return v, nil
  }

// ListProofs 导出证明。
  func (s *Service) ListProofs(ctx context.Context, limit int) ([]TickDriftProof, error) {
	   return s.store.ListProofs(ctx, limit)
   }

// ExportProof 标记导出。
  func (s *Service) ExportProof(ctx context.Context, id string) (*TickDriftProof, error) {
	  if err := s.store.MarkProofExported(ctx, id, s.clock()); err != nil {
		   return nil, err
	   }
	   return s.store.GetProof(ctx, id)
  }

// RevokeLease 运维吊销。

  func (s *Service) RevokeLease(ctx context.Context, req RevokeLeaseRequest) (*TickLease, error) {
	   actor,
		   err := requireActor(req.Actor)
	  if err != nil {
		   return nil, err
	   }
	   if strings.TrimSpace(req.Reason) == "" {
		  return nil, ErrActor
	   }
	   l, err := s.store.GetLease(ctx, req.LeaseID)
	  if err != nil {
		   return nil, err
	  }
	   if l.State.Terminal() {
		   return l, nil
	   }
	   now := s.clock()
	  next := *l
	   if err := CanTransit(l.State, LeaseRevoked); err != nil {
		  return nil, err
	  }
	  next.State = LeaseRevoked
	  next.RevokedAt = &now
	  next.Note = req.Reason
	   if err := s.store.CASLease(ctx, l.LeaseID, l.Generation, l.State, next); err != nil {
		  return nil, err
	  }
	  s.audit(ctx, l.LeaseID, l.Pair, "revoke", actor, string(l.State), string(LeaseRevoked), req.Reason)
	  return s.store.GetLease(ctx, l.LeaseID)
  }

   func (s *Service) onTickPut(prev *oracle.Tick, next oracle.Tick, gen int64) {
	  ctx := context.Background()
	  now := s.clock()
	   _ = s.store.UpsertGeneration(ctx, TickGeneration{
		   Pair: oracle.PairKey(next.Asset, next.Quote), Generation: gen, Price: next.Price, TickAt: next.At, UpdatedAt: now,
	  })
	   opens, err := s.store.ListOpenLeasesByPair(ctx, oracle.PairKey(next.Asset, next.Quote))
	   if err != nil {
		  return
	   }
	  for _, l := range opens {
		   if l.Fingerprint == FingerprintOf(next) && l.Generation == gen {
			   continue
		   }
		   reason := ErrFingerprint
		   kind := ProofTickFlip
		  if prev != nil && DriftExceeded(l.Price, next.Price, s.policy.DriftBPS) {
			   reason = ErrPriceDrift
			   kind = ProofPriceDrift
		   }
		   _ = s.revokeInternal(ctx, l, LeaseRevoked, reason.Error(), kind, next, gen)
	  }
  }

  func (s *Service) revokeInternal(ctx context.Context, l TickLease, to LeaseState, note string, kind ProofKind, live oracle.Tick, liveGen int64) error {
	   if l.State.Terminal() {
		  return nil
	  }
	   if err := CanTransit(l.State, to); err != nil {
		  return err
	   }
	   now := s.clock()
	   next := l
	   next.State = to
	   next.Note = note
	  if to == LeaseRevoked || to == LeaseExpired {
		  next.RevokedAt = &now
	  }
	  if err := s.store.CASLease(ctx, l.LeaseID, l.Generation, l.State, next); err != nil {
		  return err
	  }
	  s.openProof(ctx, TickDriftProof{
		   ProofID: uuid.NewString(), LeaseID: l.LeaseID, Pair: l.Pair, Kind: kind, FailClosed: true,
		   IssuedPrice: l.Price, LivePrice: live.Price, IssuedGen: l.Generation, LiveGen: liveGen,
		   IssuedFP: l.Fingerprint, LiveFP: FingerprintOf(live),
		  Summary:  note,
		   OpenedAt: now, FundWriteHeld: true,
	   })
	  s.audit(ctx, l.LeaseID, l.Pair, "auto-revoke", "tick-flip", string(l.State), string(to), note)
	  return nil
  }

   func (s *Service) openProof(ctx context.Context, p TickDriftProof) {
	  if p.ProofID == "" {
		   p.ProofID = uuid.NewString()
	  }
	   if p.OpenedAt.IsZero() {
		   p.OpenedAt = s.clock()
	   }
	   p.FailClosed = true
	  _ = s.store.InsertProof(ctx, p)
  }

   func (s *Service) audit(ctx context.Context, leaseID, pair, action, actor, from, to, detail string) {
	   _ = s.store.InsertAudit(ctx, AuditEvent{
		  AuditID: uuid.NewString(),
		  LeaseID: leaseID, Pair: pair, Action: action, Actor: actor,
		   FromState: from, ToState: to, Detail: detail, CreatedAt: s.clock(),
	  })
  }

// ExpireOpen 过期吊销一批。

   func (s *Service) ExpireOpen(ctx context.Context, limit int) (int, error) {
	   rows, err := s.store.ListExpiredOpen(ctx, s.clock(), limit)
	   if err != nil {
		   return 0, err
	   }
	  n := 0
	  for _, l := range rows {
		  if err := s.revokeInternal(ctx, l, LeaseExpired, "lease ttl or tick at expired", ProofLeaseExpired, oracle.Tick{Asset: l.Asset, Quote: l.Quote, Price: l.Price, At: l.TickAt}, l.Generation); err == nil {
			  n++
		   }
	  }
	  return n, nil
   }

// RecheckStale 活 Tick At 过期则吊销。
   func (s *Service) RecheckStale(ctx context.Context, limit int) (int, error) {
	  rows,
		  err := s.store.ListOpenLeases(ctx, limit)
	   if err != nil {
		   return 0, err
	   }
	   n := 0
	   now := s.clock()
	  for _, l := range rows {
		   t, err := s.feed.Get(l.Asset, l.Quote)
		   rc := RecheckLive(l, t, err, s.feed.Generation(l.Asset, l.Quote), now, s.feed.MaxAge(), s.policy.DriftBPS)
		   if rc.OK {
			  continue
		   }
		   to := LeaseRevoked
		   if rc.ProofKind == ProofLeaseExpired || rc.ProofKind == ProofTickStale {
			  to = LeaseExpired
		  }
		  if e := s.revokeInternal(ctx, l, to, rc.Reason.Error(), rc.ProofKind, rc.Live, rc.LiveGen); e == nil {
			  n++
		  }
	   }
	   return n, nil
   }
