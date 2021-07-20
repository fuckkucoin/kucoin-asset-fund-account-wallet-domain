package holdwritegate

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"account-assets.kucoin.net/funding/internal/validate"
)

const (
	DefaultBlockLeaseTTL = 72 * time.Hour
)

// HoldGapQuerier 冻结间隙查询接口，用于验证 gap 是否已修复。
type HoldGapQuerier interface {
	QueryGap(ctx context.Context, userID, asset, book string) (gapAmount string, gapOpen bool, err error)
}

// Engine 挡写世代引擎。
type Engine struct {
	mu sync.Mutex
	store Store
	gapQuerier HoldGapQuerier
	clock         func() time.Time
	blockLeaseTTL time.Duration
	blockFSM      *blockLeaseFSM
	escalationFSM *escalationFSM
}

type Config struct {
	Store      Store
	GapQuerier HoldGapQuerier
	Clock      func() time.Time
	BlockLeaseTTL time.Duration
}

  func New(cfg Config) *Engine {
	  if cfg.Store == nil {
		  cfg.Store = NewMemStore()
	  }
	  if cfg.Clock == nil {
		  cfg.Clock = func() time.Time { return time.Now().UTC() }
	   }
	   if cfg.BlockLeaseTTL <= 0 {
		   cfg.BlockLeaseTTL = DefaultBlockLeaseTTL
	  }
	   return &Engine{
		   store:         cfg.Store,
		  gapQuerier:    cfg.GapQuerier,
		  clock:         cfg.Clock,
		   blockLeaseTTL: cfg.BlockLeaseTTL,
		   blockFSM:      newBlockLeaseFSM(),
		   escalationFSM: newEscalationFSM(),
	  }
   }

// ──────────────────────────────────────────
// EscalateAdvice 建议升级：发出 hold_block_gen 并注册写门
// ──────────────────────────────────────────

   func (e *Engine) EscalateAdvice(ctx context.Context, req EscalateAdviceReq) (AdviceEscalation, HoldBlockLease, error) {
	  if e == nil {
		  return AdviceEscalation{}, HoldBlockLease{}, ErrEngineNil
	  }
	   uid, err := validate.UserID(req.UserID)
	   if err != nil {
		  return AdviceEscalation{}, HoldBlockLease{}, ErrUserRequired
	  }
	   asset := strings.ToUpper(strings.TrimSpace(req.Asset))
	  if asset == "" {
		   return AdviceEscalation{}, HoldBlockLease{}, ErrAssetRequired
	   }
	   book := strings.ToLower(strings.TrimSpace(req.Book))
	   if book == "" {
		  book = "funding"
	  }
	   adviceID := strings.TrimSpace(req.AdviceID)
	   if adviceID == "" {
		   return AdviceEscalation{}, HoldBlockLease{}, ErrAdviceIDRequired
	  }
	   reason := strings.TrimSpace(req.Reason)
	  if reason == "" {
		  return AdviceEscalation{}, HoldBlockLease{}, ErrReasonRequired
	   }
	   kind := req.Kind
	  if kind == "" {
		  kind = EscalationDriftAdvice
	   }
	  e.mu.Lock()
	  defer e.mu.Unlock()
	   if prev, err := e.store.GetEscalationByAdvice(ctx, adviceID); err == nil {
		  if prev.State != EscalationClosed {
			  lease,
				   _ := e.store.GetBlockLease(ctx, prev.LeaseID)
			  return prev, lease, nil
		  }
	   }
	  now := e.clock()
	  lease, err := e.store.GetActiveLeaseBySubject(ctx, uid, asset, book)
	   if err != nil {
		  gen, _ := e.store.LatestGenForSubject(ctx, uid, asset, book)
		  lease = HoldBlockLease{
			  ID: uuid.NewString(),
			   UserID: uid,
			  Asset: asset,
			   Book:  book,
			   AdviceID: adviceID,
			  BlockGen:  gen + 1,
			   State:     BlockLeaseOpen,
			  Reason:    reason,
			   GateCount: 0,
			  LeaseExpiry: now.Add(e.blockLeaseTTL),
			  CreatedAt:   now,
			  UpdatedAt:   now,
		   }
		   lease, err = e.store.SaveBlockLease(ctx, lease)
		  if err != nil {
			   return AdviceEscalation{}, HoldBlockLease{}, err
		  }
	  }
	  newState, err := e.blockFSM.Transition(lease.State, BlockEvActivate)
	  if err == nil && newState != lease.State {
		  lease.State = newState
		  aNow := now
		   lease.ActivatedAt = &aNow
		  lease.UpdatedAt = now
		  lease.GateCount++
		  lease, _ = e.store.SaveBlockLease(ctx, lease)
	   } else if lease.State == BlockLeaseBlocking {
		  lease.GateCount++
		  lease.UpdatedAt = now
		   lease, _ = e.store.SaveBlockLease(ctx, lease)
	  }
	  esc := AdviceEscalation{
		   ID:      uuid.NewString(),
		   LeaseID: lease.ID,
		   UserID: uid,
		  Asset:    asset,
		   Book:     book,
		  AdviceID: adviceID,
		  Kind:     kind,
		   State:    EscalationOpen,
		   BlockGen: lease.BlockGen,
		   Detail:    strings.TrimSpace(req.Detail),
		  CreatedAt: now,
		   UpdatedAt: now,
	  }
	  esc, err = e.store.SaveEscalation(ctx, esc)
	   if err != nil {
		   return AdviceEscalation{}, lease, err
	   }
	  return esc, lease, nil
  }

// ──────────────────────────────────────────
// CheckWriteGate 检查写门：活跃 blocking 下拒绝写入
// ──────────────────────────────────────────

  func (e *Engine) CheckWriteGate(ctx context.Context, req WriteGateCheckReq) (WriteGateCheckResp, error) {
	   if e == nil {
		   return WriteGateCheckResp{},
			   ErrEngineNil
	  }
	  uid, err := validate.UserID(req.UserID)
	  if err != nil {
		  return WriteGateCheckResp{}, ErrUserRequired
	  }
	  asset := strings.ToUpper(strings.TrimSpace(req.Asset))
	  if asset == "" {
		  return WriteGateCheckResp{}, ErrAssetRequired
	  }
	  book := strings.ToLower(strings.TrimSpace(req.Book))
	  if book == "" {
		   book = "funding"
	   }
	  writeID := strings.TrimSpace(req.WriteID)
	   if writeID == "" {
		  return WriteGateCheckResp{}, ErrWriteIDRequired
	   }
	   e.mu.Lock()
	   defer e.mu.Unlock()
	   now := e.clock()
	   lease, err := e.store.GetActiveLeaseBySubject(ctx, uid, asset, book)
	   if err != nil {
		   return WriteGateCheckResp{Allowed: true}, nil
	   }
	   if lease.State != BlockLeaseBlocking {
		   return WriteGateCheckResp{Allowed: true},
			   nil
	  }
	   if now.After(lease.LeaseExpiry) {
		   return WriteGateCheckResp{Allowed: true}, nil
	   }
	  entry := WriteGateEntry{
		   ID: uuid.NewString(),
		   LeaseID: lease.ID,
		  UserID: uid,
		   Asset:  asset,
		   Book:    book,
		  WriteID: writeID,
		   WriteKind: strings.TrimSpace(req.WriteKind),
		   BlockGen:  lease.BlockGen,
		  Rejected:  true,
		   Detail: fmt.Sprintf("blocked by lease %s gen=%d", lease.ID, lease.BlockGen),
		   CreatedAt: now,
	  }
	  _, _ = e.store.SaveWriteGateEntry(ctx, entry)
	   return WriteGateCheckResp{
		  Allowed: false,
		  Reason: fmt.Sprintf("active block_gen=%d lease=%s", lease.BlockGen, lease.ID),
		   LeaseID:  lease.ID,
		   BlockGen: lease.BlockGen,
	   }, nil
  }

// ──────────────────────────────────────────
// SubmitCloseProof 提交 Gap 关闭证明
// ──────────────────────────────────────────

  func (e *Engine) SubmitCloseProof(ctx context.Context, req SubmitCloseProofReq) (GapCloseProof, error) {
	   if e == nil {
		   return GapCloseProof{},
			  ErrEngineNil
	   }
	   leaseID := strings.TrimSpace(req.LeaseID)
	   if leaseID == "" {
		  return GapCloseProof{}, ErrLeaseIDRequired
	   }
	  e.mu.Lock()
	  defer e.mu.Unlock()
	   now := e.clock()
	   lease, err := e.store.GetBlockLease(ctx, leaseID)
	   if err != nil {
		  return GapCloseProof{}, err
	   }
	  escID := strings.TrimSpace(req.EscalationID)
	  proof := GapCloseProof{
		   ID:      uuid.NewString(),
		  LeaseID: lease.ID,
		   EscalationID: escID,
		  UserID:       lease.UserID,
		   Asset: lease.Asset,
		   Book:     lease.Book,
		   AdviceID: lease.AdviceID,
		  BlockGen: lease.BlockGen,
		   GapBefore: strings.TrimSpace(req.GapBefore),
		   GapAfter:  strings.TrimSpace(req.GapAfter),
		   GapClosed: req.GapClosed,
		   Validated: false,
		   Sealed: false,
		  CreatedAt: now,
	  }
	  proof, err = e.store.SaveCloseProof(ctx, proof)
	  if err != nil {
		  return GapCloseProof{}, err
	   }
	   return proof, nil
  }

// ──────────────────────────────────────────
// ValidateProof 校验 Gap 关闭证明
// ──────────────────────────────────────────

  func (e *Engine) ValidateProof(ctx context.Context, req ValidateProofReq) (GapCloseProof, error) {
	   if e == nil {
		  return GapCloseProof{}, ErrEngineNil
	   }
	   proofID := strings.TrimSpace(req.ProofID)
	  if proofID == "" {
		  return GapCloseProof{},
			   ErrProofIDRequired
	   }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	   now := e.clock()
	   proof, err := e.store.GetCloseProof(ctx, proofID)
	   if err != nil {
		  return GapCloseProof{}, err
	   }
	  if proof.Validated {
		  return proof, nil
	  }
	   if !proof.GapClosed {
		   return GapCloseProof{}, ErrGapNotClosed
	  }
	   proof.Validated = true
	  vNow := now
	   proof.ValidatedAt = &vNow
	   proof.ValidatedBy = strings.TrimSpace(req.ValidatedBy)
	  proof, err = e.store.SaveCloseProof(ctx, proof)
	   if err != nil {
		   return GapCloseProof{}, err
	  }
	  if proof.EscalationID != "" {
		  if esc, err := e.store.GetEscalation(ctx, proof.EscalationID); err == nil {
			   if esc.State == EscalationOpen {
				  newState, fsmErr := e.escalationFSM.Transition(esc.State, EscalationEvProve)
				   if fsmErr == nil {
					   esc.State = newState
					   pNow := now
					   esc.ProvenAt = &pNow
					  esc.ProofIDs = append(esc.ProofIDs,
						   proof.ID)
					   esc.UpdatedAt = now
					  _, _ = e.store.SaveEscalation(ctx, esc)
				   }
			   }
		   }
	   }
	   return proof, nil
   }

// ──────────────────────────────────────────
// DualControlClear 双人审批清除 block
// ──────────────────────────────────────────

  func (e *Engine) DualControlClear(ctx context.Context, req DualControlClearReq) (HoldBlockLease, error) {
	   if e == nil {
		   return HoldBlockLease{}, ErrEngineNil
	  }
	  leaseID := strings.TrimSpace(req.LeaseID)
	   if leaseID == "" {
		  return HoldBlockLease{},
			   ErrLeaseIDRequired
	  }
	   proofID := strings.TrimSpace(req.ProofID)
	   if proofID == "" {
		   return HoldBlockLease{}, ErrClearWithoutProof
	  }
	  if strings.TrimSpace(req.ApproverA) == "" || strings.TrimSpace(req.ApproverB) == "" {
		   return HoldBlockLease{}, ErrDualControlRequired
	   }
	   if strings.EqualFold(strings.TrimSpace(req.ApproverA), strings.TrimSpace(req.ApproverB)) {
		  return HoldBlockLease{}, ErrDualControlRequired
	   }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	  now := e.clock()
	  lease, err := e.store.GetBlockLease(ctx, leaseID)
	   if err != nil {
		  return HoldBlockLease{}, err
	  }
	   if lease.State == BlockLeaseCleared {
		   return lease, nil
	  }
	   proof, err := e.store.GetCloseProof(ctx, proofID)
	  if err != nil {
		  return HoldBlockLease{}, ErrClearWithoutProof
	   }
	  if proof.LeaseID != lease.ID {
		  return HoldBlockLease{}, ErrProofMismatch
	  }
	  if !proof.Validated {
		   return HoldBlockLease{}, ErrProofNotValidated
	  }
	   if !proof.GapClosed {
		   return HoldBlockLease{}, ErrGapNotClosed
	  }
	  if !proof.Sealed {
		   proof.Sealed = true
		   sNow := now
		  proof.SealedAt = &sNow
		  _, _ = e.store.SaveCloseProof(ctx, proof)
	  }
	  newState, err := e.blockFSM.Transition(lease.State, BlockEvClear)
	  if err != nil {
		   if lease.State == BlockLeaseExpired {
			  return HoldBlockLease{}, ErrAlreadyCleared
		   }
		   return HoldBlockLease{}, err
	   }
	   lease.State = newState
	  cNow := now
	   lease.ClearedAt = &cNow
	   lease.ClearProofID = proof.ID
	   lease.ClearedBy = fmt.Sprintf("%s+%s", strings.TrimSpace(req.ApproverA), strings.TrimSpace(req.ApproverB))
	   lease.UpdatedAt = now
	  if resolution := strings.TrimSpace(req.Resolution); resolution != "" {
		  lease.Reason = lease.Reason + "; cleared: " + resolution
	   }
	   lease, err = e.store.SaveBlockLease(ctx, lease)
	   if err != nil {
		   return HoldBlockLease{}, err
	  }
	   escs,
		   _ := e.store.ListEscalationsByLease(ctx, lease.ID, 100)
	  for _, esc := range escs {
		   if esc.State == EscalationOpen || esc.State == EscalationProven {
			  newEscState, fsmErr := e.escalationFSM.Transition(esc.State, EscalationEvClose)
			   if fsmErr == nil {
				   esc.State = newEscState
				  clNow := now
				   esc.ClosedAt = &clNow
				   esc.Resolution = "block_cleared"
				   esc.UpdatedAt = now
				   _, _ = e.store.SaveEscalation(ctx, esc)
			   } else if esc.State == EscalationOpen {
				   provenState, _ := e.escalationFSM.Transition(esc.State, EscalationEvProve)
				   esc.State = provenState
				  pNow := now
				  esc.ProvenAt = &pNow
				  esc.UpdatedAt = now
				   _, _ = e.store.SaveEscalation(ctx, esc)
				   closedState, _ := e.escalationFSM.Transition(esc.State, EscalationEvClose)
				   esc.State = closedState
				   clNow := now
				   esc.ClosedAt = &clNow
				   esc.Resolution = "block_cleared"
				   esc.UpdatedAt = now
				  _,
					  _ = e.store.SaveEscalation(ctx, esc)
			  }
		   }
	  }
	   return lease, nil
  }

// ──────────────────────────────────────────
// BindOrphanTag 孤儿标签绑定到同一 block_gen 家族
// ──────────────────────────────────────────

   func (e *Engine) BindOrphanTag(ctx context.Context, req BindOrphanTagReq) (OrphanTagBind, error) {
	   if e == nil {
		   return OrphanTagBind{}, ErrEngineNil
	  }
	   leaseID := strings.TrimSpace(req.LeaseID)
	   if leaseID == "" {
		  return OrphanTagBind{}, ErrLeaseIDRequired
	   }
	  holdcatID := strings.TrimSpace(req.HoldcatID)
	  if holdcatID == "" {
		  return OrphanTagBind{}, ErrOrphanBindFailed
	   }
	  uid := strings.TrimSpace(req.UserID)
	  asset := strings.ToUpper(strings.TrimSpace(req.Asset))
	   book := strings.ToLower(strings.TrimSpace(req.Book))
	   if book == "" {
		  book = "funding"
	   }
	  e.mu.Lock()
	  defer e.mu.Unlock()
	  now := e.clock()
	  lease, err := e.store.GetBlockLease(ctx, leaseID)
	   if err != nil {
		   return OrphanTagBind{}, err
	   }
	  if lease.State == BlockLeaseCleared || lease.State == BlockLeaseExpired {
		   return OrphanTagBind{}, ErrBlockLeaseExpired
	  }
	   bind := OrphanTagBind{
		  ID:      uuid.NewString(),
		  LeaseID: lease.ID,
		  HoldcatID: holdcatID,
		  UserID:    uid,
		  Asset:    asset,
		  Book:     book,
		  BlockGen: lease.BlockGen,
		  BoundAt: now,
	   }
	   bind, err = e.store.SaveOrphanTagBind(ctx, bind)
	   if err != nil {
		   return OrphanTagBind{}, err
	   }
	  return bind, nil
  }

// ──────────────────────────────────────────
// ExpireLease 过期 block 租约
// ──────────────────────────────────────────

   func (e *Engine) ExpireLease(ctx context.Context, leaseID string) (HoldBlockLease, error) {
	  if e == nil {
		  return HoldBlockLease{}, ErrEngineNil
	  }
	   leaseID = strings.TrimSpace(leaseID)
	   if leaseID == "" {
		   return HoldBlockLease{}, ErrLeaseIDRequired
	  }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	  now := e.clock()
	   lease, err := e.store.GetBlockLease(ctx, leaseID)
	  if err != nil {
		  return HoldBlockLease{}, err
	   }
	   if lease.State == BlockLeaseExpired {
		  return lease, nil
	  }
	   if lease.State == BlockLeaseCleared {
		  return lease, nil
	   }
	   newState, err := e.blockFSM.Transition(lease.State, BlockEvExpire)
	  if err != nil {
		  return HoldBlockLease{}, err
	   }
	   lease.State = newState
	  eNow := now
	  lease.ExpiredAt = &eNow
	   lease.UpdatedAt = now
	   lease, err = e.store.SaveBlockLease(ctx, lease)
	   if err != nil {
		  return HoldBlockLease{}, err
	   }
	  return lease, nil
   }

// ──────────────────────────────────────────
// SealProof 封口证明
// ──────────────────────────────────────────

  func (e *Engine) SealProof(ctx context.Context, proofID string) (GapCloseProof, error) {
	  if e == nil {
		   return GapCloseProof{}, ErrEngineNil
	  }
	   proofID = strings.TrimSpace(proofID)
	  if proofID == "" {
		  return GapCloseProof{},
			   ErrProofIDRequired
	   }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	   now := e.clock()
	   proof, err := e.store.GetCloseProof(ctx, proofID)
	   if err != nil {
		  return GapCloseProof{}, err
	   }
	   if proof.Sealed {
		  return proof, nil
	   }
	  proof.Sealed = true
	  sNow := now
	  proof.SealedAt = &sNow
	  proof, err = e.store.SaveCloseProof(ctx, proof)
	  if err != nil {
		  return GapCloseProof{}, err
	  }
	   return proof, nil
   }

// ──────────────────────────────────────────
// Query 方法
// ──────────────────────────────────────────

  func (e *Engine) GetBlockLease(ctx context.Context, id string) (HoldBlockLease, error) {
	  if e == nil {
		  return HoldBlockLease{}, ErrEngineNil
	   }
	  return e.store.GetBlockLease(ctx, strings.TrimSpace(id))
   }

   func (e *Engine) GetActiveLeaseBySubject(ctx context.Context, userID, asset, book string) (HoldBlockLease, error) {
	  if e == nil {
		  return HoldBlockLease{}, ErrEngineNil
	   }
	   return e.store.GetActiveLeaseBySubject(ctx, userID, strings.ToUpper(strings.TrimSpace(asset)), strings.ToLower(strings.TrimSpace(book)))
  }

  func (e *Engine) GetEscalation(ctx context.Context, id string) (AdviceEscalation, error) {
	   if e == nil {
		  return AdviceEscalation{}, ErrEngineNil
	  }
	  return e.store.GetEscalation(ctx, strings.TrimSpace(id))
  }

   func (e *Engine) GetCloseProof(ctx context.Context, id string) (GapCloseProof, error) {
	  if e == nil {
		  return GapCloseProof{}, ErrEngineNil
	   }
	  return e.store.GetCloseProof(ctx, strings.TrimSpace(id))
   }

   func (e *Engine) BlockQuery(ctx context.Context, req BlockQueryReq) ([]HoldBlockLease, error) {
	  if e == nil {
		  return nil, ErrEngineNil
	   }
	  uid := strings.TrimSpace(req.UserID)
	  if uid == "" {
		   return nil, ErrUserRequired
	  }
	  return e.store.ListBlockLeasesByUser(ctx, uid, 100)
   }

  func (e *Engine) BlockGenQuery(ctx context.Context, userID, asset, book string) (BlockGenResp, error) {
	   if e == nil {
		  return BlockGenResp{}, ErrEngineNil
	   }
	  uid, err := validate.UserID(userID)
	  if err != nil {
		   return BlockGenResp{}, ErrUserRequired
	  }
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	   if asset == "" {
		  return BlockGenResp{}, ErrAssetRequired
	  }
	  book = strings.ToLower(strings.TrimSpace(book))
	  if book == "" {
		   book = "funding"
	   }
	  gen,
		   _ := e.store.LatestGenForSubject(ctx, uid, asset, book)
	  resp := BlockGenResp{
		  UserID: uid,
		  Asset:  asset,
		  Book:   book,
		   CurrentGen: gen,
	   }
	  if lease, err := e.store.GetActiveLeaseBySubject(ctx, uid, asset, book); err == nil {
		   resp.ActiveLease = lease.ID
		  resp.State = lease.State
	   }
	   return resp, nil
   }

   func (e *Engine) ProofExport(ctx context.Context, req ProofExportReq) ([]GapCloseProof, error) {
	   if e == nil {
		  return nil, ErrEngineNil
	  }
	  limit := req.Limit
	  if limit <= 0 {
		   limit = 100
	   }
	   leaseID := strings.TrimSpace(req.LeaseID)
	  userID := strings.TrimSpace(req.UserID)
	   if leaseID != "" {
		   return e.store.ListProofsByLease(ctx, leaseID, limit)
	   }
	  if userID != "" {
		  return e.store.ListProofsByUser(ctx, userID, limit)
	   }
	  return nil, fmt.Errorf("%w: lease_id or user_id required", ErrLeaseIDRequired)
  }

  func (e *Engine) EscalationQuery(ctx context.Context, req EscalationQueryReq) ([]AdviceEscalation, error) {
	   if e == nil {
		  return nil, ErrEngineNil
	  }
	   limit := req.Limit
	  if limit <= 0 {
		  limit = 100
	  }
	   leaseID := strings.TrimSpace(req.LeaseID)
	   userID := strings.TrimSpace(req.UserID)
	   state := strings.TrimSpace(req.State)
	   if leaseID != "" {
		   return e.store.ListEscalationsByLease(ctx,
			   leaseID, limit)
	  }
	  if state != "" {
		   return e.store.ListEscalationsByState(ctx,
			  EscalationState(state), limit)
	   }
	  if userID != "" {
		   return e.store.ListEscalationsByUser(ctx, userID, limit)
	  }
	  return nil, fmt.Errorf("lease_id, user_id, or state required")
   }

  func (e *Engine) ListWriteGateEntries(ctx context.Context, leaseID string, limit int) ([]WriteGateEntry, error) {
	   if e == nil {
		   return nil, ErrEngineNil
	  }
	   if limit <= 0 {
		   limit = 100
	   }
	   return e.store.ListWriteGateByLease(ctx, strings.TrimSpace(leaseID), limit)
   }

   func (e *Engine) ListOrphanBinds(ctx context.Context, leaseID string, limit int) ([]OrphanTagBind, error) {
	  if e == nil {
		   return nil, ErrEngineNil
	   }
	   if limit <= 0 {
		   limit = 100
	   }
	   return e.store.ListOrphanBindsByLease(ctx, strings.TrimSpace(leaseID), limit)
   }

   func (e *Engine) ListBlockingLeases(ctx context.Context, limit int) ([]HoldBlockLease, error) {
	  if e == nil {
		   return nil, ErrEngineNil
	  }
	   if limit <= 0 {
		   limit = 100
	   }
	  return e.store.ListBlockingLeases(ctx, limit)
  }
