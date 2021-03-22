package closelatecredit

import (
	"context"
	"errors"
	"time"
	"github.com/google/uuid"
)

// Config 引擎配置。

type Config struct {
	// PreferRejectOnClosed 关户后迟贷直接拒绝（默认 false：进争议仓）。
	PreferRejectOnClosed bool
	// AllowCreditWithoutEpoch 必须 false。
	AllowCreditWithoutEpoch bool
	// AllowForceLive 必须 false。
	AllowForceLive bool
	ScanBatchSize int
	HoldBatchSize  int
	ProofBatchSize int
}

  func (c Config) withDefaults() Config {
	   if c.ScanBatchSize <= 0 {
		  c.ScanBatchSize = 200
	   }
	  if c.HoldBatchSize <= 0 {
		   c.HoldBatchSize = 200
	   }
	   if c.ProofBatchSize <= 0 {
		   c.ProofBatchSize = 200
	  }
	  return c
  }

// Engine 关户迟贷引擎。
type Engine struct {
	store   Store
	view    AccountView
	observe ObservePort
	outbox OutboxPort
	fiat FiatPort
	cfg  Config
	clock func() time.Time
}

// NewEngine 初始化。view / 端口可为空。
  func NewEngine(store Store, cfg Config) *Engine {
	  return &Engine{
		   store: store,
		   cfg: cfg.withDefaults(),
		  observe: NopObserve{},
		  outbox: NopOutbox{},
		   fiat:   NopFiat{},
	  }
  }

// WithView 注入科目只读视图。
   func (e *Engine) WithView(v AccountView) *Engine {
	  e.view = v
	  return e
   }

// WithPorts 注入迟贷只读端口。

  func (e *Engine) WithPorts(obs ObservePort, out OutboxPort, fiat FiatPort) *Engine {
	  if obs != nil {
		   e.observe = obs
	  }
	   if out != nil {
		   e.outbox = out
	   }
	   if fiat != nil {
		   e.fiat = fiat
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

  func (e *Engine) ready() error {
	   if e == nil || e.store == nil {
		  return ErrUnavailable
	   }
	   return nil
  }

   func (e *Engine) audit(ctx context.Context, userID, book, from, to string, fromN, toN uint64, actor, note string) {
	  _ = e.store.InsertAudit(ctx, &EpochAudit{
		   ID: uuid.NewString(), UserID: userID, Book: book,
		   FromState: from, ToState: to, FromN: fromN, ToN: toN,
		  Actor: actor, Note: note, CreatedAt: e.now(),
	  })
  }

// EnsureOpen 保证用户科目存在 open 世代（N=1）。幂等。
  func (e *Engine) EnsureOpen(ctx context.Context, userID, book, actor string) (*CloseEpoch, error) {
	  if err := e.ready(); err != nil {
		   return nil, err
	  }
	  userID = trim(userID)
	  if userID == "" {
		  return nil, ErrUserRequired
	  }
	   book = BookDefault(book)
	  if existing, err := e.store.GetEpochByUserBook(ctx, userID, book); err == nil {
		  return existing, nil
	  } else if !errors.Is(err, ErrNotFound) {
		  return nil, err
	  }
	  now := e.now()
	  ep := &CloseEpoch{
		  ID: uuid.NewString(), UserID: userID, Book: book,
		   N: 1, State: EpochOpen, LiveEpoch: 1, Actor: actor,
		   IdemKey:   epochIdemKey(userID, book, 1, "ensure"),
		   CreatedAt: now, UpdatedAt: now,
	  }
	  if err := e.store.InsertEpoch(ctx, ep); err != nil {
		  if errors.Is(err, ErrDuplicate) {
			  return e.store.GetEpochByUserBook(ctx,
				  userID, book)
		  }
		   return nil, err
	   }
	   _ = e.store.UpsertCursor(ctx, &EpochCursor{UserID: userID, Book: book, NextN: 2, UpdatedAt: now})
	   e.audit(ctx, userID, book, "", string(EpochOpen), 0, 1, actor, "ensure_open")
	   return ep, nil
   }

// CurrentEpoch 只读当前世代。不存在则 ErrNotFound。
   func (e *Engine) CurrentEpoch(ctx context.Context, userID, book string) (*CloseEpoch, error) {
	  if err := e.ready(); err != nil {
		  return nil, err
	   }
	  return e.store.GetEpochByUserBook(ctx, trim(userID), BookDefault(book))
   }

// CloseWithEpoch 关户成功升 close_epoch。open/reopened → closing → closed，N++。
// 前置：科目允许关户（funding 由调用方闸，本包仍记录世代）。
// 失败：无用户、存储不可用。已关户再次关闭返回当前行（幂等，不升世代）。

   func (e *Engine) CloseWithEpoch(ctx context.Context, req CloseRequest) (*CloseEpoch, error) {
	   if err := e.ready(); err != nil {
		   return nil, err
	   }
	  req.Normalize()
	  if req.UserID == "" {
		  return nil, ErrUserRequired
	   }
	   req.Book = BookDefault(req.Book)
	  ep, err := e.EnsureOpen(ctx, req.UserID, req.Book, req.Actor)
	  if err != nil {
		  return nil, err
	  }
	  if ep.State == EpochClosed {
		   return ep, nil
	   }
	  now := e.now()
	  from := ep.State
	   fromN := ep.N
	   if err := AssertEpochTransit(ep.State, EpochClosing); err != nil {
		   return nil, err
	   }
	   ep.State = EpochClosing
	   ep.UpdatedAt = now
	   if err := e.store.UpdateEpoch(ctx, ep); err != nil {
		  return nil, err
	   }
	  e.audit(ctx, ep.UserID, ep.Book, string(from), string(EpochClosing), fromN, ep.N, req.Actor, "close_begin")
	  if err := AssertEpochTransit(EpochClosing, EpochClosed); err != nil {
		   return nil, err
	  }
	   next := ep.N + 1
	  ep.N = next
	   ep.State = EpochClosed
	   ep.ClosedEpoch = next
	   ep.CloseReason = req.Reason
	   ep.Actor = firstNonEmpty(req.Actor, ep.Actor)
	   ep.UpdatedAt = e.now()
	   if err := e.store.UpdateEpoch(ctx, ep); err != nil {
		   return nil, err
	   }
	  _ = e.store.UpsertCursor(ctx, &EpochCursor{UserID: ep.UserID, Book: ep.Book, NextN: next + 1, UpdatedAt: ep.UpdatedAt})
	  e.audit(ctx, ep.UserID, ep.Book, string(EpochClosing), string(EpochClosed), fromN, next, req.Actor, "close_commit")
	   incClose()
	   return ep, nil
  }

// ReopenWithEpoch 重新开户升世代。closed → reopened，N++。旧世代贷记不得入新活户。

  func (e *Engine) ReopenWithEpoch(ctx context.Context, req ReopenRequest) (*CloseEpoch, error) {
	  if err := e.ready(); err != nil {
		   return nil, err
	  }
	   req.Normalize()
	  if req.UserID == "" {
		   return nil, ErrUserRequired
	  }
	  req.Book = BookDefault(req.Book)
	  ep, err := e.store.GetEpochByUserBook(ctx, req.UserID, req.Book)
	  if err != nil {
		   return nil, err
	   }
	  if ep.State.AllowsLiveCredit() {
		  return ep, nil
	   }
	   if err := AssertEpochTransit(ep.State, EpochReopened); err != nil {
		  return nil, err
	   }
	  fromN := ep.N
	  next := ep.N + 1
	  ep.N = next
	   ep.State = EpochReopened
	   ep.LiveEpoch = next
	  ep.ReopenReason = req.Reason
	   ep.Actor = firstNonEmpty(req.Actor, ep.Actor)
	  ep.UpdatedAt = e.now()
	   if err := e.store.UpdateEpoch(ctx, ep); err != nil {
		   return nil, err
	  }
	  _ = e.store.UpsertCursor(ctx, &EpochCursor{UserID: ep.UserID, Book: ep.Book, NextN: next + 1, UpdatedAt: ep.UpdatedAt})
	  e.audit(ctx, ep.UserID, ep.Book, string(EpochClosed), string(EpochReopened), fromN, next, req.Actor, "reopen")
	   incReopen()
	   return ep, nil
  }

// ConfirmOpen 将 reopened 确认回 open（不升世代）。
  func (e *Engine) ConfirmOpen(ctx context.Context, userID, book, actor string) (*CloseEpoch, error) {
	   if err := e.ready(); err != nil {
		  return nil, err
	   }
	   ep, err := e.store.GetEpochByUserBook(ctx, trim(userID), BookDefault(book))
	   if err != nil {
		   return nil, err
	   }
	  if ep.State == EpochOpen {
		  return ep, nil
	  }
	  if err := AssertEpochTransit(ep.State, EpochOpen); err != nil {
		  return nil, err
	  }
	  from := ep.State
	   ep.State = EpochOpen
	   ep.Actor = firstNonEmpty(actor, ep.Actor)
	  ep.UpdatedAt = e.now()
	   if err := e.store.UpdateEpoch(ctx, ep); err != nil {
		   return nil, err
	  }
	  e.audit(ctx, ep.UserID, ep.Book, string(from), string(EpochOpen), ep.N, ep.N, actor, "confirm_open")
	   return ep, nil
  }

// CreditAssert 贷记断言。无 epoch 或失配不得成功。
// 不写 ledger。ForceLive 在配置下恒拒绝。
  func (e *Engine) CreditAssert(ctx context.Context, req CreditAssertRequest) (*GateResult, error) {
	  if err := e.ready(); err != nil {
		   return nil, err
	   }
	  req.Normalize()
	   res := &GateResult{}
	   if req.UserID == "" {
		  return res.FailClosed("user_id"), ErrUserRequired
	   }
	   if !req.Source.Valid() {
		   return res.FailClosed("source"), ErrSourceRequired
	  }
	  if !AssetOK(req.Asset) {
		  return res.FailClosed("asset"), ErrAsset
	   }
	  if !AmountOK(req.Amount) {
		  return res.FailClosed("amount"), ErrAmount
	  }
	  if req.ForceLive && (ForceLiveDenied(req.ForceLive) && !e.cfg.AllowForceLive) {
		  incForceDeny()
		  return e.sealReject(ctx, req, nil, 0, ReasonForceDenied, DecisionReject, ErrForceDenied)
	  }
	   if !CreditRequiresEpoch(req.ClaimedEpoch) && !e.cfg.AllowCreditWithoutEpoch {
		   incMissing()
		   ep, _ := e.store.GetEpochByUserBook(ctx, req.UserID, BookDefault(req.Book))
		   actual := uint64(0)
		   if ep != nil {
			   actual = ep.N
			  res.Epoch = ep
		   }
		  return e.sealReject(ctx, req, ep, actual, ReasonMissingEpoch, DecisionForMissing(), ErrMissingEpoch)
	   }
	  if AllowedWithoutEpoch(req.Source) {
		   return res.FailClosed("policy"), ErrMissingEpoch
	   }
	  ep, err := e.store.GetEpochByUserBook(ctx, req.UserID, BookDefault(req.Book))
	  if err != nil && !errors.Is(err, ErrNotFound) {
		   return nil, err
	   }
	   if errors.Is(err, ErrNotFound) || ep == nil {
		  incMissing()
		   return e.sealReject(ctx, req, nil, 0, ReasonMissingEpoch, DecisionReject, ErrMissingEpoch)
	  }
	  res.Epoch = ep
	   claimed := claimedOrZero(req.ClaimedEpoch)
	  if req.ClaimedUser != "" && req.ClaimedUser != req.UserID {
		  incMismatch()
		  return e.sealReject(ctx, req, ep, ep.N, ReasonWrongUser, DecisionReject, ErrEpochMismatch)
	   }
	  if ep.IsClosed() {
		  dec := DecisionForClosed(req.Source, e.cfg.PreferRejectOnClosed)
		  reason := ReasonForClosed(dec)
		   if claimed != ep.N {
			   incMismatch()
			   reason = ReasonEpochMismatch
			   dec = DecisionReject
		   }
		   if dec == DecisionDispute {
			   return e.sealDispute(ctx, req, ep, reason)
		  }
		   return e.sealReject(ctx,
			  req, ep, ep.N, reason, dec, ErrClosedRefuse)
	  }
	  if claimed != ep.N {
		   if ep.IsLive() && ep.StaleAgainst(claimed) {
			   incStale()
			   return e.sealReject(ctx, req, ep, ep.N, ReasonStaleAfterOpen, DecisionForStale(), ErrEpochStale)
		   }
		  incMismatch()
		   return e.sealReject(ctx, req, ep, ep.N, ReasonEpochMismatch, DecisionReject, ErrEpochMismatch)
	   }
	   if !ep.IsLive() {
		  dec := DecisionForClosed(req.Source, e.cfg.PreferRejectOnClosed)
		  if dec == DecisionDispute {
			   return e.sealDispute(ctx,
				   req, ep, ReasonForClosed(dec))
		  }
		   return e.sealReject(ctx, req, ep, ep.N, ReasonForClosed(dec), dec, ErrClosedRefuse)
	  }
	  incAdmit()
	  res.Allowed = true
	  res.Decision = DecisionAdmit
	  return res, nil
  }

   func (e *Engine) sealReject(ctx context.Context, req CreditAssertRequest, ep *CloseEpoch, actual uint64, reason LateReason, dec CreditDecision, base error) (*GateResult, error) {
	  p, err := e.upsertProof(ctx, req, ep, actual, reason, dec, LateStateForDecision(dec), "")
	  if err != nil {
		   return nil, err
	   }
	  incReject()
	  incProof()
	  res := &GateResult{Allowed: false, Decision: dec, Epoch: ep, Proof: p, RejectReason: string(reason)}
	  return res, wrapProof(base, p.ID)
  }

  func (e *Engine) sealDispute(ctx context.Context, req CreditAssertRequest, ep *CloseEpoch, reason LateReason) (*GateResult, error) {
	  p, err := e.upsertProof(ctx, req, ep, ep.N, reason, DecisionDispute, LateDisputed, "")
	  if err != nil {
		  return nil, err
	   }
	   h, err := e.upsertHold(ctx, p, ep)
	  if err != nil {
		  return nil, err
	   }
	   if p.HoldID != h.ID {
		  p.HoldID = h.ID
		   p.UpdatedAt = e.now()
		  _ = e.store.UpdateProof(ctx, p)
	   }
	   incDispute()
	  incProof()
	  incHold()
	  res := &GateResult{Allowed: false, Decision: DecisionDispute, Epoch: ep, Proof: p, Hold: h, RejectReason: string(reason)}
	   return res, wrapProof(ErrDisputeHold, p.ID)
   }

  func (e *Engine) upsertProof(ctx context.Context, req CreditAssertRequest, ep *CloseEpoch, actual uint64, reason LateReason, dec CreditDecision, st LateState, holdID string) (*CloseLateCreditProof, error) {
	   claimed := claimedOrZero(req.ClaimedEpoch)
	   idem := req.IdemKey
	  if idem == "" {
		   idem = lateIdemKey(req.UserID, string(req.Source), req.TxRef, req.Amount, req.Asset, claimed, actual, reason)
	   }
	   if existing, err := e.store.GetProofByIdem(ctx, idem); err == nil {
		   return existing, nil
	   } else if !errors.Is(err, ErrNotFound) {
		  return nil, err
	  }
	   now := e.now()
	  book := BookDefault(req.Book)
	   if ep != nil && ep.Book != "" {
		   book = ep.Book
	  }
	  p := &CloseLateCreditProof{
		  ID: uuid.NewString(),
		  UserID: req.UserID, ClaimedUser: req.ClaimedUser, Book: book,
		   Source: req.Source, Asset: req.Asset, Amount: req.Amount, TxRef: req.TxRef,
		  ClaimedEpoch: claimed, ActualEpoch: actual, Reason: reason, State: st, Decision: dec,
		   IdemKey: idem,
		  Digest:  lateDigest(req.UserID, req.ClaimedUser, string(req.Source), req.TxRef, req.Amount, req.Asset, claimed, actual, reason, dec),
		   HoldID: holdID, WrongUser: req.ClaimedUser != "" && req.ClaimedUser != req.UserID,
		  ForceLive: req.ForceLive,
		   Actor:     req.Actor, Note: req.Note,
		   CreatedAt: now, UpdatedAt: now,
	   }
	   if st == LateClosed || st == LateRejected {
		   t := now
		   p.ClosedAt = &t
	   }
	   if err := e.store.InsertProof(ctx, p); err != nil {
		   if errors.Is(err, ErrDuplicate) {
			   return e.store.GetProofByIdem(ctx, idem)
		   }
		  return nil, err
	  }
	  return p, nil
  }

  func (e *Engine) upsertHold(ctx context.Context, p *CloseLateCreditProof, ep *CloseEpoch) (*DisputeHoldCase, error) {
	  idem := holdIdemKey(p.ID,
		   p.UserID, p.ActualEpoch)
	  if existing, err := e.store.GetHoldByIdem(ctx, idem); err == nil {
		   return existing, nil
	  } else if !errors.Is(err, ErrNotFound) {
		  return nil, err
	   }
	   if existing, err := e.store.GetHoldByProof(ctx, p.ID); err == nil {
		  return existing, nil
	  } else if !errors.Is(err, ErrNotFound) {
		   return nil, err
	   }
	   now := e.now()
	   n := uint64(0)
	   if ep != nil {
		  n = ep.N
	  }
	   h := &DisputeHoldCase{
		   ID: uuid.NewString(), ProofID: p.ID, UserID: p.UserID,
		  Asset: p.Asset, Amount: p.Amount, Source: p.Source,
		  EpochAtPark: n, State: HoldAfterPark(), IdemKey: idem,
		  Actor: p.Actor, CreatedAt: now, UpdatedAt: now,
	   }
	  if err := e.store.InsertHold(ctx, h); err != nil {
		   if errors.Is(err, ErrDuplicate) {
			   return e.store.GetHoldByIdem(ctx,
				  idem)
		  }
		  return nil, err
	   }
	  return h, nil
   }

// DisputeResolve 争议处置。resolved 升新证明摘要，不贷入新活户。
  func (e *Engine) DisputeResolve(ctx context.Context, req DisputeResolveRequest) (*GateResult, error) {
	  if err := e.ready(); err != nil {
		  return nil, err
	  }
	  req.Normalize()
	  var h *DisputeHoldCase
	  var err error
	  if req.HoldID != "" {
		  h, err = e.store.GetHoldByID(ctx, req.HoldID)
	  } else if req.ProofID != "" {
		   h, err = e.store.GetHoldByProof(ctx, req.ProofID)
	   } else {
		  return nil, ErrHoldNotFound
	   }
	   if err != nil {
		  return nil, err
	  }
	  target := ResolveTarget(req.Reject, req.ExportOnly)
	  if err := AssertHoldTransit(h.State, target); err != nil {
		   return nil, err
	   }
	  h.State = target
	  h.ResolveNote = req.Note
	  h.Actor = firstNonEmpty(req.Actor, h.Actor)
	  h.UpdatedAt = e.now()
	  if err := e.store.UpdateHold(ctx, h); err != nil {
		  return nil, err
	  }
	   p, err := e.store.GetProofByID(ctx, h.ProofID)
	   if err != nil {
		  return nil, err
	   }
	  nextLate := LateClosed
	   if req.Reject {
		   nextLate = LateRejected
	   }
	  if err := AssertLateTransit(p.State, nextLate); err == nil {
		  p.State = nextLate
		   t := e.now()
		  p.ClosedAt = &t
		  p.UpdatedAt = t
		   p.Note = firstNonEmpty(req.Note, p.Note)
		  p.Actor = firstNonEmpty(req.Actor, p.Actor)
		   _ = e.store.UpdateProof(ctx, p)
	  }
	   incResolve()
	  return &GateResult{Allowed: false, Decision: DecisionReject, Proof: p, Hold: h, RejectReason: "resolved_no_live_credit"}, nil
   }

// ExportProof 证明导出。缺摘要 fail-closed。
  func (e *Engine) ExportProof(ctx context.Context, id string) (*ProofExport, error) {
	   if err := e.ready(); err != nil {
		  return nil, err
	   }
	   p, err := e.store.GetProofByID(ctx, trim(id))
	  if err != nil {
		   return nil, err
	  }
	  if p.Digest == "" {
		  return nil, ErrExportDenied
	  }
	   ex := ToExport(p)
	   addExport(1)
	  return &ex, nil
   }

// EnqueuePending 迟贷扫描入队。

  func (e *Engine) EnqueuePending(ctx context.Context, p *PendingCredit) error {
	   if err := e.ready(); err != nil {
		  return err
	  }
	   if p.ID == "" {
		  p.ID = uuid.NewString()
	   }
	  if p.SeenAt.IsZero() {
		   p.SeenAt = e.now()
	   }
	  return e.store.InsertPending(ctx, p)
  }

// ScanLateCredits 扫描未处理迟贷并走 CreditAssert。

   func (e *Engine) ScanLateCredits(ctx context.Context, limit int) (int, error) {
	   if err := e.ready(); err != nil {
		  return 0, err
	  }
	   if limit <= 0 {
		   limit = e.cfg.ScanBatchSize
	  }
	   rows, err := e.store.ListUnscannedPending(ctx, limit)
	  if err != nil {
		   return 0, err
	  }
	  n := 0
	  for _, row := range rows {
		  _,
			  _ = e.CreditAssert(ctx, CreditAssertRequest{
			  UserID: row.UserID, Source: row.Source, Asset: row.Asset,
			   Amount: row.Amount,
			  TxRef: row.TxRef, ClaimedEpoch: row.ClaimedEpoch,
			   IdemKey: pendingIdemKey(row.UserID, row.Source, row.TxRef),
			  Actor: "late_scan",
		  })
		  row.Scanned = true
		  _ = e.store.UpdatePending(ctx, row)
		  n++
	   }
	  addScan(n)
	   return n, nil
  }

// PumpDisputeHolds 将 open 证明补建争议仓。
  func (e *Engine) PumpDisputeHolds(ctx context.Context, limit int) (int, error) {
	   if err := e.ready(); err != nil {
		  return 0, err
	   }
	  if limit <= 0 {
		   limit = e.cfg.HoldBatchSize
	  }
	   rows,
		   err := e.store.ListProofsByState(ctx, LateDisputed, limit)
	  if err != nil {
		   return 0, err
	  }
	  n := 0
	  for _, p := range rows {
		   if p.HoldID != "" {
			   continue
		  }
		  ep, _ := e.store.GetEpochByUserBook(ctx, p.UserID, p.Book)
		  h, err := e.upsertHold(ctx, p, ep)
		  if err != nil {
			   continue
		  }
		   p.HoldID = h.ID
		   p.UpdatedAt = e.now()
		   _ = e.store.UpdateProof(ctx, p)
		   n++
	  }
	  addHoldPump(n)
	   return n, nil
   }

// ProveOpen 导出未闭合证明（巡检）。
   func (e *Engine) ProveOpen(ctx context.Context, limit int) (int, error) {
	  if err := e.ready(); err != nil {
		   return 0, err
	   }
	  if limit <= 0 {
		  limit = e.cfg.ProofBatchSize
	   }
	  rows,
		   err := e.store.ListOpenProofs(ctx, limit)
	  if err != nil {
		   return 0, err
	   }
	   n := 0
	  for _, p := range rows {
		   if p.Digest == "" {
			   continue
		   }
		   n++
	  }
	  addExport(n)
	   return n, nil
  }

// GetProof 只读。

  func (e *Engine) GetProof(ctx context.Context, id string) (*CloseLateCreditProof, error) {
	   if err := e.ready(); err != nil {
		  return nil, err
	   }
	   return e.store.GetProofByID(ctx, trim(id))
  }

// ListProofs 按用户。

   func (e *Engine) ListProofs(ctx context.Context, userID string, limit int) ([]*CloseLateCreditProof, error) {
	   if err := e.ready(); err != nil {
		   return nil, err
	   }
	   return e.store.ListProofsByUser(ctx, trim(userID), limit)
   }

// GetHold 只读。

   func (e *Engine) GetHold(ctx context.Context, id string) (*DisputeHoldCase, error) {
	  if err := e.ready(); err != nil {
		   return nil, err
	  }
	  return e.store.GetHoldByID(ctx, trim(id))
   }

// ListAudit 只读。
   func (e *Engine) ListAudit(ctx context.Context, userID string, limit int) ([]*EpochAudit, error) {
	   if err := e.ready(); err != nil {
		   return nil, err
	   }
	   return e.store.ListAuditByUser(ctx, trim(userID), limit)
   }

// AssertObserve 观察贷记入口。
   func (e *Engine) AssertObserve(ctx context.Context, userID, asset, amount, txRef string, epoch *uint64) (*GateResult, error) {
	   return e.CreditAssert(ctx, HintToAssert(SourceObserve, userID, asset, amount, txRef, epoch))
   }

// AssertOutbox 出箱迟达入口。
   func (e *Engine) AssertOutbox(ctx context.Context, userID, asset, amount, txRef string, epoch *uint64) (*GateResult, error) {
	   return e.CreditAssert(ctx, HintToAssert(SourceOutbox, userID, asset, amount, txRef, epoch))
  }

// AssertFiat 法币回调入口。
  func (e *Engine) AssertFiat(ctx context.Context, userID, asset, amount, txRef string, epoch *uint64) (*GateResult, error) {
	  return e.CreditAssert(ctx, HintToAssert(SourceFiatCallback, userID, asset, amount, txRef, epoch))
   }
