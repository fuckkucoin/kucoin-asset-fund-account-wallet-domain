// 现货账本：划转和充提状态机。
// 出金先冻再扣，入金观察到账再入账。
// 作者: Draven
// 日期: 2020.10.08
package ledger

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/cryptoenc"
	"account-assets.kucoin.net/funding/internal/events"
	"account-assets.kucoin.net/funding/internal/kyt"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/signpipe"
	"account-assets.kucoin.net/funding/internal/trace"
	"account-assets.kucoin.net/funding/internal/wallet"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/shopspring/decimal"
)

type ChainBridge interface {
	// Submit/Observe/Status/Inspect 是账本对链的唯一出口；链上 RPC 必须在 DB 事务外。
	Submit(ctx context.Context, chain, to string, amount decimal.Decimal) (string, error)
	Status(ctx context.Context, chain, hash string) (wallet.TxStatus, error)
	Observe(ctx context.Context, chain, hash string, amount decimal.Decimal) (string, error)
	Inspect(ctx context.Context, chain, hash string) (wallet.Observation, error)
}

// DepositAddressFunc 用户在该链的充值地址。对不上就别入账。

type DepositAddressFunc func(ctx context.Context,
	  userID, chain string) string

type Service struct {
	store Store
	risk  *RiskEngine
	lock  Locker
	enc *cryptoenc.Envelope
	bridge ChainBridge
	log    zerolog.Logger
	lockTTL time.Duration
	OnBooks func(ctx context.Context, userID string) // 入账后失效余额缓存并 hint 对账
	drift             *DriftGuard
	disableDriftBlock bool
	stuckFrozen       time.Duration
	stuckSubmitted    time.Duration
	kyt               kyt.Screener // 广播前筛一遍，挂了就拒
	confirmDepth int
	withdrawFeeBPS int
	earnInterestBPS int
	withdrawCooling time.Duration
	passThroughHold time.Duration
	passThroughBPS int
	depositAddr     DepositAddressFunc
	coverUndoFaults int32 // 单测注入：撤销补账前 N 次失败
	coverSweepMinAge time.Duration
	OnAlert func(kind, detail string)
}

// NewService 装配账本状态机：冻结、广播、确认入账共用一把用户锁。
  func NewService(store Store, risk *RiskEngine, lock Locker, enc *cryptoenc.Envelope, bridge ChainBridge, log zerolog.Logger, cfg config.TransferConfig) *Service {
	   ttl := cfg.LockTTL
	   if ttl == 0 {
		   ttl = 12 * time.Second
	  }
	   sf := cfg.StuckFrozen
	  if sf <= 0 {
		   sf = 15 * time.Second
	  }
	  ss := cfg.StuckSubmitted
	   if ss <= 0 {
		  ss = 30 * time.Second
	   }
	  depth := cfg.ConfirmDepth
	   if depth <= 0 {
		   depth = 1
	  }
	  return &Service{
		  store: store, risk: risk, lock: lock, enc: enc, bridge: bridge, log: log, lockTTL: ttl,
		  drift: NewDriftGuard(), disableDriftBlock: cfg.DisableDriftBlock,
		  stuckFrozen: sf, stuckSubmitted: ss,
		   kyt:            kyt.Static{Mode: kyt.Allow},
		   confirmDepth:   depth,
		   withdrawFeeBPS: cfg.WithdrawFeeBPS, earnInterestBPS: cfg.EarnInterestBPS,
		   withdrawCooling: cfg.WithdrawCooling, passThroughHold: cfg.PassThroughHold,
		   passThroughBPS: cfg.PassThroughBPS,
	  }
   }

// SetKYT 挂出金筛查。筛子出错直接拒，别带着未知风险上链。
   func (s *Service) SetKYT(sc kyt.Screener) {
	  if sc != nil {
		  s.kyt = sc
	  }
  }

// SetDepositAddress 挂上充值地址派生。入金只能记到该用户自己的入站地址，禁止串户入账。
   func (s *Service) SetDepositAddress(fn DepositAddressFunc) {
	  s.depositAddr = fn
   }

// withdrawChainAmount 链上实发：本金减手续费。账上仍按全额冻，fee 入账时再切。
   func (s *Service) withdrawChainAmount(tr *Transfer) decimal.Decimal {
	   if tr == nil {
		  return decimal.Zero
	   }
	   fee := s.withdrawFee(tr)
	   out := tr.Amount.Sub(fee)
	  if !out.IsPositive() {
		  return tr.Amount
	  }
	   return out
   }

type CreateReq struct {
	UserID string
	Direction Direction
	Asset     string
	Amount    decimal.Decimal
	Chain     string
	ToAddress string
	FromAddress string // 入金付款方；空则不写 hint
	Memo        string // 观察 memo；入账单回显
	Tag         string // 观察 tag
	TagName     string // memo / destination_tag / comment
	TxHash string
	OutputIndex int
	IdempotencyKey string
	WaitConfirm    bool
	CoverTrading   bool // 资金账不够时再从交易账补；与本单同把用户锁
}

// Create 出入金与内部划转的入口。
// 入参须带 UserID、Direction、Asset、Amount、IdempotencyKey。出金另要 Chain 与 ToAddress，入金带 TxHash。
// 幂等键命中且金额、方向、地址、哈希一致时按原单接着。不一致则拒绝，严禁当成另一笔再冻。
// 顺序：核对幂等，校验方向、地址、资产，再过账户冻结与账实不符闸。
// 出金还要过身份等级、解冻冷静期、热钱包库存、地址形状与地址簿。
// 持用户锁后建单并冻结。链上请求放在事务外。未落下哈希的失败才允许回滚冻结。
// 单据上已经有链上哈希时，严禁因后续步骤失败而解冻。
  func (s *Service) Create(ctx context.Context, req CreateReq) (tr *Transfer, err error) {
	   ctx, sp := trace.WithSpan(ctx, "ledger.Create")
	   defer func() { sp.End(s.log, err) }()
	  req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	   ctx = WithShardUser(ctx, req.UserID)
	  if req.IdempotencyKey == "" {
		   return nil, ErrMissingIdempotency
	   }
	   if existing, err := s.store.GetByIdempotency(ctx, req.IdempotencyKey); err == nil {
		   if err := s.sameIntent(existing, req); err != nil {
			  metrics.IdempotencyConflicts.Inc()
			  return existing, err
		  }
		   if g, ok := leaseFromContext(ctx); ok {
			   return s.progressLocked(ctx,
				  existing.ID, req.WaitConfirm, g)
		  }
		   return s.resume(ctx, existing.ID, req.WaitConfirm)
	  }
	  dir, err := ParseDirection(string(req.Direction))
	  if err != nil {
		   return nil, err
	  }
	  req.Direction = dir
	   if err := validateMovement(req); err != nil {
		   return nil, err
	   }
	  if req.Direction.OnChain() && req.TxHash != "" {
		   if err := s.rejectDuplicateChainHash(ctx, req.Chain, req.TxHash, req.OutputIndex, ""); err != nil {
			   return nil, err
		   }
	  }
	  if req.UserID != "system" {
		  if err := s.CheckUser(ctx, req.UserID); err != nil {
			   return nil, err
		   }
	   }
	  if req.Direction == WalletWithdraw {
		   if err := s.requireKYC(ctx, req.UserID, KYCBasic); err != nil {
			  return nil, err
		   }
		   if err := s.requireWithdrawCooling(ctx, req.UserID); err != nil {
			   return nil, err
		   }
		  if err := s.ensureHotInventory(ctx, req.Asset, req.Amount); err != nil {
			  return nil, err
		   }
	   }
	   if req.Direction == WalletDeposit {
		  if err := s.bindDepositAddress(ctx, &req); err != nil {
			   return nil, err
		  }
	   }
	  if req.Direction == SpotSend {
		  if err := s.CheckUser(ctx, strings.TrimSpace(req.ToAddress)); err != nil {
			   return nil, err
		   }
	   }
	   tr = &Transfer{
		   ID: uuid.NewString(),
		   IdempotencyKey: req.IdempotencyKey,
		   UserID:         req.UserID,
		  Direction:      req.Direction,
		  Asset: req.Asset,
		   Amount: req.Amount,
		  Chain: req.Chain,
		   ToAddress: req.ToAddress,
		  TxHash:      req.TxHash,
		   OutputIndex: req.OutputIndex,
		   State:     StateInitiated,
		   CreatedAt: time.Now().UTC(),
	  }
	   if s.enc != nil && req.ToAddress != "" && req.Direction != SpotSend {
		   blob, err := s.enc.Encrypt([]byte(req.ToAddress))
		  if err != nil {
			  return nil, err
		   }
		   tr.ToAddressEnc = blob
		  tr.ToAddressHint = cryptoenc.Hint(req.ToAddress)
	  }
	   if req.Direction == SpotSend {
		  tr.ToAddress = strings.TrimSpace(req.ToAddress)
		  tr.ToAddressHint = tr.ToAddress
	  }
	  if fa := strings.TrimSpace(req.FromAddress); fa != "" {
		   if len(fa) > 128 {
			  fa = fa[:128]
		   }
		  tr.FromAddressHint = fa
	  }
	  if m := strings.TrimSpace(req.Memo); m != "" {
		   if len(m) > 128 {
			   m = m[:128]
		   }
		   tr.MemoHint = m
	   }
	   if tg := strings.TrimSpace(req.Tag); tg != "" {
		   if len(tg) > 128 {
			   tg = tg[:128]
		  }
		  tr.TagHint = tg
	  }
	  if tn := strings.TrimSpace(req.TagName); tn != "" {
		  if len(tn) > 32 {
			  tn = tn[:32]
		  }
		   tr.TagNameHint = tn
	   }
	  var lease Guard
	  var destLease Guard
	   var lockErr error
	  held, hasHeld := leaseFromContext(ctx)
	  if hasHeld && req.Direction != SpotSend {
		  lease = held
	   } else if req.Direction == SpotSend {
		   lease,
			   destLease, lockErr = lockUsers(ctx, s.lock, s.lockTTL, req.UserID, strings.TrimSpace(req.ToAddress))
	   } else {
		   lease, lockErr = s.lock.Lock(ctx, UserLockKey(req.UserID), s.lockTTL)
	   }
	   if lockErr != nil {
		   return nil, lockErr
	  }
	  if !hasHeld {
		   defer lease.Unlock()
	  }
	   if destLease != nil {
		   defer destLease.Unlock()
	   }
	   // 主单未落库的残留补账必须先撤销，禁止带着半截 cover 再开新单。
	   if _, e2 := s.store.GetByIdempotency(ctx, req.IdempotencyKey); e2 != nil {
		  if uerr := s.mustUndoCover(ctx, lease, req); uerr != nil {
			   return nil, fmt.Errorf("%w: %v", ErrCoverStranded, uerr)
		  }
	  }
	   if err := s.coverSourceLocked(ctx, lease, req); err != nil {
		  if uerr := s.mustUndoCover(ctx, lease, req); uerr != nil {
			  return nil, fmt.Errorf("%w: cover=%v undo=%v", ErrCoverStranded, err, uerr)
		   }
		   return nil, err
	  }
	   releaseHash, err := s.holdChainHash(ctx, tr.Chain, tr.TxHash, tr.OutputIndex)
	   if err != nil {
		   if uerr := s.mustUndoCover(ctx, lease, req); uerr != nil {
			   return nil, fmt.Errorf("%w: %v", ErrCoverStranded, uerr)
		   }
		   return nil, err
	   }
	   released := false
	   release := func() {
		   if released {
			   return
		   }
		   released = true
		   releaseHash()
	   }
	   defer release()
	   if err := s.store.InTx(ctx, func(ctx context.Context) error {
		   if tr.Direction.OnChain() && tr.TxHash != "" {
			   if err := s.rejectDuplicateChainHash(ctx, tr.Chain, tr.TxHash, tr.OutputIndex, tr.ID); err != nil {
				  return err
			  }
		  }
		   if err := s.gateWithdrawSafety(ctx, tr); err != nil {
			   tr.State = StateFailed
			  tr.FailReason = err.Error()
			  _ = s.store.InsertTransfer(ctx, tr)
			   _ = s.audit(ctx, tr, "risk_reject", map[string]any{"reason": err.Error()})
			   return err
		  }
		   dec,
			   err := s.risk.Evaluate(ctx, s.store, tr)
		  if err != nil {
			  return err
		   }
		   tr.RiskFlags = dec.Flags
		  if dec.Reject {
			  tr.State = StateFailed
			  tr.FailReason = dec.Reason
			   _ = s.store.InsertTransfer(ctx, tr)
			  _ = s.audit(ctx, tr, "risk_reject", map[string]any{"reason": dec.Reason})
			   return fmt.Errorf("%w: %s", ErrRiskRejected, dec.Reason)
		   }
		  if err := s.enforceElevatedKYC(ctx, tr, dec); err != nil {
			   tr.State = StateFailed
			  tr.FailReason = err.Error()
			   _ = s.store.InsertTransfer(ctx, tr)
			  _ = s.audit(ctx, tr, "risk_reject", map[string]any{"reason": err.Error()})
			   return err
		   }
		   if err := s.store.InsertTransfer(ctx, tr); err != nil {
			  return err
		   }
		   return s.audit(ctx, tr, "initiated", nil)
	  }); err != nil {
		   release()
		   if errors.Is(err, ErrIdempotency) {
			  got, e2 := s.store.GetByIdempotency(ctx, req.IdempotencyKey)
			   if e2 != nil {
				  return nil, err
			   }
			  if e := s.sameIntent(got, req); e != nil {
				   metrics.IdempotencyConflicts.Inc()
				  return got, e
			   }
			   return s.progressLocked(ctx, got.ID, req.WaitConfirm, lease)
		  }
		   if existing, e2 := s.store.GetByIdempotency(ctx, req.IdempotencyKey); e2 == nil {
			   // 主单已可见但本跳失败：先撤补账再返回，避免 cover 悬挂与 in_use 清扫死锁。
			  if uerr := s.mustUndoCover(ctx, lease, req); uerr != nil {
				   return existing, fmt.Errorf("%w: %v", ErrCoverStranded, uerr)
			  }
			  return existing, err
		   }
		   // 主单未落库：撤销必须成功，否则 fail-closed，禁止留下半截补账。
		  if uerr := s.mustUndoCover(ctx, lease, req); uerr != nil {
			   _ = s.audit(ctx, tr, "cover_undo_fail", map[string]any{"reason": uerr.Error()})
			   return tr, fmt.Errorf("%w: risk=%v undo=%v", ErrCoverStranded, err, uerr)
		   }
		  return tr, err
	   }
	   release()
	   return s.progressLocked(ctx, tr.ID, req.WaitConfirm, lease)
   }

// validateMovement 链上单必须带链和可上架资产；提现必须有目标地址，且形态必须匹配该链。
// 形态闸在风控/白名单之前，垃圾地址不得进冻结，更不得进广播。

   func validateMovement(req CreateReq) error {
	  if req.Direction.OnChain() {
		  if req.Chain == "" {
			   return ErrMissingChain
		   }
		  if !wallet.AssetOnChain(req.Asset, req.Chain) {
			   return fmt.Errorf("%w: %s on %s", ErrAssetChain, req.Asset, req.Chain)
		   }
	   }
	  if req.Direction == WalletWithdraw {
		   if req.ToAddress == "" {
			  return ErrMissingAddress
		   }
		   if err := validateChainAddress(req.Chain, req.ToAddress); err != nil {
			  return err
		   }
	   }
	  if req.Direction == SpotSend {
		  if strings.TrimSpace(req.ToAddress) == "" {
			   return ErrCounterparty
		   }
		  if strings.TrimSpace(req.ToAddress) == strings.TrimSpace(req.UserID) {
			   return ErrSelfTransfer
		   }
	  }
	  if ExceedsLedgerMagnitude(req.Amount) {
		  return ErrAmountOverflow
	   }
	  return nil
   }

// gateWithdrawSafety 在冻结之前检查出金地址。非出金单直接返回。
// 地址为空则拒绝。EVM 地址必须是 0x 后接 40 位十六进制，长度不是 42 即拒绝。
// 形状不符严禁进入冻结。
// 目标等于本人入站地址，或与本人入站地址前后六位相撞，拒绝。
// 入金之后短时间内大额提出，按当前阈值拒绝。
   func (s *Service) gateWithdrawSafety(ctx context.Context, tr *Transfer) error {
	   if s == nil || tr == nil || tr.Direction != WalletWithdraw {
		   return nil
	  }
	  to := strings.TrimSpace(tr.ToAddress)
	   if to == "" {
		  return ErrMissingAddress
	  }
	   if wallet.IsEVM(tr.Chain) {
		   addr := strings.ToLower(to)
		   if len(addr) != 42 || !strings.HasPrefix(addr, "0x") {
			   return ErrInvalidAddress
		   }
	   }
	   if s.depositAddr != nil {
		   own := strings.TrimSpace(s.depositAddr(ctx, tr.UserID, tr.Chain))
		  if own != "" {
			   if sameChainAddress(tr.Chain, to, own) {
				   return fmt.Errorf("%w: %s", ErrOwnInbound, own)
			  }
			  if LookalikeInbound(tr.Chain, to, own) {
				   return fmt.Errorf("%w: %s", ErrAddressPoison, own)
			   }
		   }
	   }
	   if PassThroughHit(ctx, s.store, tr, s.passThroughHold, s.passThroughBPS) {
		  return ErrPassThrough
	   }
	   return nil
  }

// bindDepositAddress 入金前核对托管入站地址。请求里带了别人的地址就拒，空则盖上本用户派生地址。

   func (s *Service) bindDepositAddress(ctx context.Context, req *CreateReq) error {
	  if s == nil || req == nil {
		  return fmt.Errorf("%w: unavailable", ErrDepositAddress)
	   }
	   if s.depositAddr == nil {
		   return fmt.Errorf("%w: unavailable", ErrDepositAddress)
	  }
	  expected := strings.TrimSpace(s.depositAddr(ctx, req.UserID, req.Chain))
	  if expected == "" {
		  return fmt.Errorf("%w: unavailable", ErrDepositAddress)
	   }
	   got := strings.TrimSpace(req.ToAddress)
	  if got != "" && !sameChainAddress(req.Chain, got, expected) {
		  return fmt.Errorf("%w: want %s", ErrDepositAddress, expected)
	  }
	  req.ToAddress = expected
	  return nil
  }

   func sameChainAddress(chain, a, b string) bool {
	  a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	   if wallet.IsEVM(chain) || strings.EqualFold(chain, "EVM") {
		  return strings.EqualFold(a, b)
	   }
	   return a == b
  }

// enforceElevatedKYC 可疑金额出金要求 advanced KYC。

  func (s *Service) enforceElevatedKYC(ctx context.Context, tr *Transfer, dec RiskDecision) error {
	  if tr == nil || tr.Direction != WalletWithdraw {
		  return nil
	  }
	  need := false
	  if s.risk != nil && s.risk.SuspiciousRequiresAdvanced() {
		  for _, f := range dec.Flags {
			   if f == "suspicious_amount" {
				   need = true
				   break
			   }
		   }
	  }
	  if !need {
		   return nil
	   }
	   if err := s.requireKYC(ctx, tr.UserID, KYCAdvanced); err != nil {
		  return fmt.Errorf("%w: %v", ErrRiskRejected, err)
	   }
	   return nil
  }

// sameIntent 在幂等键命中时核对用户、方向、资产、链、金额、地址与链上哈希。
// 任一不一致即冲突。严禁把已占用的幂等键当成一笔新划转接着冻结。
   func (s *Service) sameIntent(existing *Transfer, req CreateReq) error {
	  if existing.UserID != req.UserID || existing.Direction != req.Direction || existing.Asset != req.Asset || existing.Chain != req.Chain || !existing.Amount.Equal(req.Amount) {
		   return fmt.Errorf("%w: existing transfer %s", ErrIdempotencyConflict, existing.ID)
	   }
	  gotAddr := existing.ToAddress
	   if gotAddr == "" {
		   gotAddr = s.DecryptAddress(existing)
	   }
	  if req.ToAddress != "" && gotAddr != "" && req.ToAddress != gotAddr {
		  return fmt.Errorf("%w: existing transfer %s", ErrIdempotencyConflict, existing.ID)
	  }
	  if req.TxHash != "" && existing.TxHash != "" && req.TxHash != existing.TxHash {
		   return fmt.Errorf("%w: existing transfer %s", ErrIdempotencyConflict, existing.ID)
	   }
	   if req.TxHash != "" && existing.OutputIndex != req.OutputIndex {
		   return fmt.Errorf("%w: existing transfer %s", ErrIdempotencyConflict, existing.ID)
	  }
	  return nil
  }

// LockUser 持用户锁并写入 ctx，供母子 saga 等多步过账复用同一把租约。
   func (s *Service) LockUser(ctx context.Context, userID string) (context.Context, Guard, error) {
	   if s == nil || s.lock == nil {
		  return ctx, nil, ErrBusy
	  }
	   if g, ok := leaseFromContext(ctx); ok {
		  return ctx, g, nil
	   }
	   lease, err := s.lock.Lock(ctx, UserLockKey(strings.TrimSpace(userID)), s.lockTTL)
	   if err != nil {
		   return ctx, nil, err
	  }
	  return ContextWithLease(ctx, lease), lease, nil
   }

// SetCoverUndoFaults 单测注入：随后 N 次补账撤销先失败再重试。
   func (s *Service) SetCoverUndoFaults(n int) {
	  if s == nil {
		  return
	   }
	  if n < 0 {
		  n = 0
	   }
	   atomic.StoreInt32(&s.coverUndoFaults, int32(n))
  }

   func (s *Service) consumeCoverUndoFault() error {
	  if s == nil {
		   return nil
	  }
	  for {
		  cur := atomic.LoadInt32(&s.coverUndoFaults)
		  if cur <= 0 {
			   return nil
		  }
		   if atomic.CompareAndSwapInt32(&s.coverUndoFaults, cur, cur-1) {
			   return errors.New("injected cover undo fault")
		   }
	   }
  }

   func (s *Service) mustUndoCover(ctx context.Context, lease Guard, req CreateReq) error {
	   var last error
	  for i := 0; i < 3; i++ {
		  last = s.undoCoverLocked(ctx,
			   lease, req)
		   if last == nil {
			   return nil
		  }
	  }
	   return last
  }

// ByIdempotency 按幂等键取已有单；补偿后判断该跳是否已冲正时用。
  func (s *Service) ByIdempotency(ctx context.Context, key string) (*Transfer, error) {
	  if s == nil || s.store == nil {
		  return nil, ErrNotFound
	   }
	   key = strings.TrimSpace(key)
	  if key == "" {
		  return nil, ErrMissingIdempotency
	   }
	  return s.store.GetByIdempotency(ctx, key)
  }

// resume 已有单接着往下走：内部划转一次事务入账，链上单走冻→广播→确认。

  func (s *Service) resume(ctx context.Context, id string, wait bool) (*Transfer, error) {
	   cur, err := s.store.GetTransfer(ctx, id)
	   if err != nil {
		   return nil, err
	   }
	   var lease Guard
	  var destLease Guard
	  if cur.Direction.UserSend() {
		  lease,
			  destLease, err = lockUsers(ctx, s.lock, s.lockTTL, cur.UserID, cur.Counterparty())
	  } else {
		  lease, err = s.lock.Lock(ctx, UserLockKey(cur.UserID), s.lockTTL)
	  }
	  if err != nil {
		  return nil, err
	   }
	  defer lease.Unlock()
	   if destLease != nil {
		   defer destLease.Unlock()
	  }
	   return s.progressLocked(ctx, id, wait, lease)
  }

// progressLocked 持用户锁推进状态机。
// 内部现货↔钱包：同一事务冻源入目标。
// 出金：冻钱包 → 事务外广播 → 确认后只扣冻结。
// 入金：观察 inbound（不冻用户）→ 确认后从 bridge 入钱包。

   func (s *Service) progressLocked(ctx context.Context, id string, wait bool, lease Guard) (*Transfer, error) {
	   ctx,
		  err := s.beginFence(ctx, id, lease)
	   if err != nil {
		  return nil, err
	   }
	  cur, err := s.getFenced(ctx, id)
	   if err != nil {
		  return nil, err
	  }
	  switch cur.State {
	  case StateCredited,
		   StateRolledBack, StateHold:
		  return cur, nil
	  case StateFailed:
		  if err := s.rollbackLocked(ctx, cur, cur.FailReason); err != nil {
			   return cur, err
		  }
		   return s.store.GetTransfer(ctx, id)
	   }
	   if cur.Direction.Internal() || cur.Direction.UserSend() {
		   if err := s.settleInternalLocked(ctx, id); err != nil {
			  got, _ := s.store.GetTransfer(ctx, id)
			  if got != nil {
				   return got, err
			   }
			  return nil, err
		  }
		  cur, _ = s.store.GetTransfer(ctx, id)
		   if cur != nil && s.OnBooks != nil {
			  s.OnBooks(ctx, cur.UserID)
			   if cur.Direction.UserSend() && cur.Counterparty() != "" && cur.Counterparty() != cur.UserID {
				  s.OnBooks(ctx, cur.Counterparty())
			   }
		   }
		  return s.store.GetTransfer(ctx, id)
	   }
	   if cur.Direction == WalletDeposit {
		  return s.progressDepositLocked(ctx, id, wait, lease)
	  }
	  if kytReview(cur) {
		   return cur, ErrKYTReview
	  }
	   if CanApply(cur.State, EventFreezeSpot) || CanApply(cur.State, EventLockWallet) {
		   if err := s.advanceFreeze(ctx, cur); err != nil {
			   got, _ := s.store.GetTransfer(ctx, id)
			   if got != nil {
				  return got, err
			  }
			   return nil, err
		   }
		  cur, _ = s.getFenced(ctx, id)
	   }
	  if cur != nil && CanApply(cur.State, EventSubmit) {
		  if err := touchLease(ctx, lease); err != nil {
			  return cur, err
		  }
		  to, err := s.destination(cur)
		   if err != nil {
			  return s.failProgress(ctx, id, err)
		   }
		  chainAmt := s.withdrawChainAmount(cur)
		   var hash string
		   err = withLeaseHeartbeat(ctx, lease, s.lockTTL, func(rctx context.Context) error {
			   if err := s.ensureHotInventory(rctx, cur.Asset, chainAmt); err != nil {
				  return err
			  }
			   if err := s.screenWithdraw(rctx, cur); err != nil {
				   return err
			  }
			   submitCtx := wallet.WithWithdrawMemo(wallet.WithSubmitNonce(wallet.WithUserID(rctx, cur.UserID), cur.ID), cur.MemoHint, cur.TagHint, cur.TagNameHint)
			   if s.enc != nil {
				   destTag := cur.TagHint
				  if destTag == "" {
					   destTag = cur.MemoHint
				  }
				   if err := signpipe.AllowCold(signpipe.ColdRef{
					  MovementID: cur.ID, Idem: cur.IdempotencyKey, Chain: cur.Chain, To: to, Amount: chainAmt.String(),
				  }); err != nil {
					  return err
				  }
				   sig, e2 := s.enc.SignWithdraw(cur.ID, cur.Chain, to, chainAmt.String(), destTag)
				  if e2 != nil {
					   return e2
				  }
				   submitCtx = wallet.WithWithdrawSig(submitCtx, sig)
				  metrics.KMSOps.WithLabelValues("sign_withdraw").Inc()
			   }
			   var e2 error
			  hash, e2 = s.bridge.Submit(submitCtx, cur.Chain, to, chainAmt)
			   return e2
		  })
		   if err != nil {
			  if errors.Is(err, ErrKYTReview) {
				  got, _ := s.store.GetTransfer(ctx, id)
				  if got != nil {
					   return got, err
				  }
				  return cur, err
			  }
			  return s.failProgress(ctx, id, err)
		   }
		  cur, err = s.afterChainHash(ctx, id, hash)
		   if err != nil {
			   return cur, err
		  }
	  }
	   if wait && cur != nil && (cur.State == StateChainSubmitted || cur.State == StateConfirmed) {
		  if err := s.confirmLocked(ctx, id); err != nil && !errors.Is(err, ErrPending) {
			   got,
				   _ := s.store.GetTransfer(ctx, id)
			  if got != nil {
				   return got, err
			  }
			  return cur, err
		  }
	   }
	  if cur != nil && s.OnBooks != nil {
		  s.OnBooks(ctx, cur.UserID)
	   }
	  return s.store.GetTransfer(ctx, id)
  }

// progressDepositLocked 入金不冻用户：Observe inbound → 确认后从 bridge 入钱包。
   func (s *Service) progressDepositLocked(ctx context.Context, id string, wait bool, lease Guard) (*Transfer, error) {
	  if _, ok := fenceFrom(ctx); !ok {
		  var err error
		  ctx,
			   err = s.beginFence(ctx, id, lease)
		   if err != nil {
			  return nil, err
		   }
	   }
	   cur, err := s.getFenced(ctx, id)
	  if err != nil {
		   return nil, err
	   }
	   if cur.State == StateInitiated && CanApply(cur.State, EventSubmit) {
		   if err := touchLease(ctx, lease); err != nil {
			   return cur, err
		  }
		   observe := cur.TxHash
		   if observe == "" {
			  observe = "dep:" + cur.ID
		  }
		  var hash string
		   err = withLeaseHeartbeat(ctx, lease, s.lockTTL, func(rctx context.Context) error {
			  var e2 error
			  hash, e2 = s.bridge.Observe(wallet.WithUserID(rctx, cur.UserID), cur.Chain, observe, cur.Amount)
			   return e2
		   })
		   if err != nil {
			   return s.failProgress(ctx, id, err)
		   }
		   cur, err = s.afterChainHash(ctx, id, hash)
		  if err != nil {
			   return cur, err
		   }
	  }
	   if wait && cur != nil && (cur.State == StateChainSubmitted || cur.State == StateConfirmed) {
		  if err := s.confirmLocked(ctx, id); err != nil && !errors.Is(err, ErrPending) {
			  got, _ := s.store.GetTransfer(ctx, id)
			  if got != nil {
				   return got, err
			   }
			   return cur, err
		  }
	  }
	  if cur != nil && s.OnBooks != nil {
		   s.OnBooks(ctx, cur.UserID)
	   }
	  return s.store.GetTransfer(ctx, id)
  }

// settleInternalLocked 现货↔钱包：同一把用户锁、同一笔事务冻源记目标，不上链。

   func (s *Service) settleInternalLocked(ctx context.Context, id string) error {
	  return s.store.InTx(ctx, func(ctx context.Context) error {
		   cur, err := s.getFenced(ctx, id)
		  if err != nil {
			  return err
		  }
		   if cur.State == StateCredited || cur.State == StateRolledBack {
			  return nil
		   }
		   if cur.State == StateFailed {
			  return s.rollbackLocked(ctx, cur, cur.FailReason)
		   }
		   if cur.Direction.Freezes() && (CanApply(cur.State, EventFreezeSpot) || CanApply(cur.State, EventLockWallet)) {
			  if err := s.applyFreeze(ctx,
				  cur); err != nil {
				  return err
			   }
			  cur, err = s.store.GetTransfer(ctx, id)
			   if err != nil {
				  return err
			   }
		   }
		   if CanApply(cur.State, EventSubmit) {
			  hash := cur.TxHash
			   if hash == "" {
				   hash = InternalTxHash(cur.ID)
			  }
			  next, err := Apply(cur.State, EventSubmit)
			  if err != nil {
				  return err
			   }
			   cur.TxHash = hash
			  cur.State = next
			  if err := s.putFenced(ctx, cur); err != nil {
				   return err
			  }
			  _ = s.audit(ctx, cur, "chain_submitted", map[string]any{"tx_hash": hash, "settlement": "internal"})
			  metrics.TransferByState.WithLabelValues(string(next), string(cur.Direction)).Inc()
		  }
		   if CanApply(cur.State, EventConfirm) {
			   next,
				   err := Apply(cur.State, EventConfirm)
			  if err != nil {
				  return err
			   }
			  cur.State = next
			   if err := s.putFenced(ctx, cur); err != nil {
				   return err
			   }
			  _ = s.audit(ctx, cur, "confirmed", map[string]any{"settlement": "internal"})
			   metrics.TransferByState.WithLabelValues(string(next), string(cur.Direction)).Inc()
		   }
		   return s.creditBooks(ctx, cur)
	   })
   }

// advanceFreeze 事务内冻源账可用，写成冻结科目；余额不够记 insufficient。
  func (s *Service) advanceFreeze(ctx context.Context, tr *Transfer) error {
	  return s.store.InTx(ctx, func(ctx context.Context) error {
		  cur, err := s.getFenced(ctx, tr.ID)
		  if err != nil {
			   return err
		  }
		   if err := s.applyFreeze(ctx, cur); err != nil {
			  return err
		   }
		  tr.State = cur.State
		   return nil
	   })
   }

// applyFreeze 在已持用户锁 + fence 的事务里冻源账。
// available 必须 ≥ Amount（DECIMAL 比较，禁止 float）；不够记 insufficient 并 fail，不写半边流水。
// 流水是 available→frozen 一对，ValidateJournal 轧平后才 Upsert。状态只许进 spot_frozen / wallet_locked。

  func (s *Service) applyFreeze(ctx context.Context, cur *Transfer) error {
	   book := sourceBook(cur.Direction)
	  ev := freezeEvent(cur.Direction)
	   next, err := Apply(cur.State, ev)
	   if err != nil {
		  return err
	  }
	   acc, err := s.store.GetAccount(ctx, cur.UserID, book, cur.Asset)
	   if err != nil {
		  return err
	  }
	  if acc.Available.LessThan(cur.Amount) {
		  cur.FailReason = ErrInsufficient.Error()
		  cur.State, _ = Apply(cur.State, EventFail)
		  _ = s.putFenced(ctx, cur)
		   _ = s.audit(ctx, cur, "insufficient", map[string]any{"available": acc.Available.String()})
		   return ErrInsufficient
	   }
	   lines := freezeLines(cur, book)
	  if err := s.store.AppendJournal(ctx, lines); err != nil {
		  return err
	  }
	   for _, e := range lines {
		  if err := ApplyJournalToAccount(acc, e); err != nil {
			  return err
		  }
	   }
	   if err := requireFreshBook(acc); err != nil {
		   return err
	   }
	   if err := s.store.UpsertAccount(ctx, acc); err != nil {
		  return err
	   }
	  cur.State = next
	   metrics.TransferByState.WithLabelValues(string(next), string(cur.Direction)).Inc()
	   if err := s.putFenced(ctx, cur); err != nil {
		   return err
	  }
	  return s.audit(ctx, cur, string(next), nil)
   }

// markSubmitted 把链上 hash 写到单据并推进到 chain_submitted；同 hash 不得挂两笔。
  func (s *Service) markSubmitted(ctx context.Context, id, hash string) error {
	   return s.store.InTx(ctx, func(ctx context.Context) error {
		  cur, err := s.getFenced(ctx, id)
		   if err != nil {
			  return err
		   }
		   if cur.State == StateChainSubmitted && cur.TxHash == hash {
			   return nil
		  }
		  if err := s.rejectDuplicateChainHash(ctx, cur.Chain, hash, cur.OutputIndex, cur.ID); err != nil {
			  return err
		   }
		   next, err := Apply(cur.State, EventSubmit)
		   if err != nil {
			   return err
		   }
		   cur.TxHash = hash
		  cur.State = next
		  metrics.TransferByState.WithLabelValues(string(next), string(cur.Direction)).Inc()
		   if err := s.putFenced(ctx, cur); err != nil {
			  return err
		   }
		  return s.audit(ctx, cur, "chain_submitted", map[string]any{"tx_hash": hash})
	   })
  }

// persistTxHash 在广播返回哈希后，先把哈希写到仍处于冻结的单据上，此时不改状态。
// 单据上已有哈希时，卡住恢复不得自动解冻。已有哈希与本次不一致则拒绝覆盖。
// 写入前占用该链上哈希，避免两笔同时挂上同一哈希。
  func (s *Service) persistTxHash(ctx context.Context, id, hash string) error {
	  if hash == "" {
		  return nil
	   }
	   cur, err := s.store.GetTransfer(ctx, id)
	   if err != nil {
		   return err
	   }
	   release, err := s.holdChainHash(ctx, cur.Chain, hash, cur.OutputIndex)
	   if err != nil {
		   return err
	   }
	   defer release()
	   return s.store.InTx(ctx, func(ctx context.Context) error {
		   cur, err := s.getFenced(ctx, id)
		   if err != nil {
			  return err
		   }
		   if cur.TxHash == hash {
			   return nil
		  }
		  if cur.TxHash != "" && cur.TxHash != hash {
			  return fmt.Errorf("tx hash mismatch for %s", id)
		  }
		  if err := s.rejectDuplicateChainHash(ctx, cur.Chain, hash, cur.OutputIndex, cur.ID); err != nil {
			   return err
		  }
		   cur.TxHash = hash
		  return s.putFenced(ctx, cur)
	   })
   }

// holdChainHash 在链上哈希落库完成前占用该哈希。
// 锁键为链、哈希、输出下标。空哈希和内部单不加锁。
// 这把锁不可重入。建单事务结束后必须先释放，再进入后续推进。严禁在仍持有本锁时再次获取。
// 解锁按令牌删除。令牌不符不得删掉其它执行体持有的锁。
  func (s *Service) holdChainHash(ctx context.Context, chain, hash string, outputIndex int) (func(), error) {
	   noop := func() {}
	   hash = strings.TrimSpace(hash)
	   if s == nil || s.lock == nil || hash == "" || IsInternalTxHash(hash) {
		   return noop, nil
	   }
	   g, err := s.lock.Lock(ctx, "lock:chain-tx:"+strings.ToUpper(strings.TrimSpace(chain))+":"+hash+":"+strconv.Itoa(outputIndex), s.lockTTL)
	   if err != nil {
		   return nil, err
	   }
	   return g.Unlock, nil
   }

// requireFreshBook 要求科目带本次从库里读到的余额快照。
// 没有快照说明这组余额不是本次读取的结果。严禁写回。
  func requireFreshBook(a *Account) error {
	   if a == nil || !a.snapOK {
		   return ErrStaleBook
	   }
	   return nil
   }

// rejectDuplicateChainHash 保证同一链、同一哈希、同一输出下标只挂一张链上单。
// 空哈希和内部单不查。查到的是本单自身则通过。查到其它单据则拒绝。

   func (s *Service) rejectDuplicateChainHash(ctx context.Context, chain, hash string, outputIndex int, selfID string) error {
	  if hash == "" || IsInternalTxHash(hash) {
		  return nil
	   }
	  got, err := s.store.FindByChainTxHash(ctx, chain, hash, outputIndex)
	  if err != nil {
		  if errors.Is(err, ErrNotFound) {
			  return nil
		  }
		   return err
	  }
	   if selfID != "" && got.ID == selfID {
		   return nil
	   }
	  return fmt.Errorf("%w: %s", ErrDuplicateHash, got.ID)
   }

// advanceSubmit 在数据库事务之外提交链上交易。
// 严禁把链上请求包进账本事务。节点无响应时事务会占住冻结直到超时。
   func (s *Service) advanceSubmit(ctx context.Context, tr *Transfer) error {
	  to, err := s.destination(tr)
	   if err != nil {
		  return err
	   }
	   hash, err := s.bridge.Submit(wallet.WithWithdrawMemo(wallet.WithSubmitNonce(wallet.WithUserID(ctx, tr.UserID), tr.ID), tr.MemoHint, tr.TagHint, tr.TagNameHint), tr.Chain, to, s.withdrawChainAmount(tr))
	   if err != nil {
		  return err
	   }
	  return s.recordBroadcastHash(ctx,
		   tr.ID, hash)
  }

// failProgress 在广播之前的失败路径上回滚冻结。
// 单据上已经有链上哈希时不得进入本函数。链上交易可能仍然有效，此时解冻会造成账实不符。
   func (s *Service) failProgress(ctx context.Context, id string, cause error) (*Transfer, error) {
	   rbErr := s.store.InTx(ctx, func(ctx context.Context) error {
		  cur, e2 := s.store.GetTransfer(ctx, id)
		  if e2 != nil {
			   return e2
		   }
		  return s.rollbackLocked(ctx, cur, cause.Error())
	   })
	  got, _ := s.store.GetTransfer(ctx, id)
	  if rbErr != nil && got != nil {
		   return got, rbErr
	  }
	   return got, cause
  }

// afterChainHash 先落 hash 再推进状态。hash 已被别的单占用则回滚这笔未用的冻结。

  func (s *Service) afterChainHash(ctx context.Context, id, hash string) (*Transfer, error) {
	  if err := s.recordBroadcastHash(ctx, id, hash); err != nil {
		  got, _ := s.store.GetTransfer(ctx, id)
		  if errors.Is(err, ErrDuplicateHash) && got != nil && got.TxHash != hash {
			   return s.failProgress(ctx, id, err)
		  }
		   if got != nil {
			   return got, err
		  }
		   return nil, err
	   }
	   return s.store.GetTransfer(ctx, id)
   }

// recordBroadcastHash 广播后先把 hash 钉在冻结/观察单上，再进 chain_submitted。RPC 和改状态之间崩溃不得当成从未提交。
   func (s *Service) recordBroadcastHash(ctx context.Context, id, hash string) error {
	  if err := s.persistTxHash(ctx, id, hash); err != nil {
		  if errors.Is(err, ErrDuplicateHash) {
			  return err
		   }
	   }
	   if err := s.markSubmitted(ctx, id, hash); err != nil {
		   _ = s.persistTxHash(ctx, id, hash)
		  return err
	  }
	   return nil
   }

// ConfirmAndCredit 确认后入账。
// 入参：单据 id。已 credited 的内部单直接返回；rolled_back 不再入账。
// 出参：nil 表示入账或已入账；ErrPending 留给 worker 按深度重试；链失败同锁解冻。
// 持用户锁 + 新 fence。链 Inspect 在事务外。深度不够或重组未 unwind 完不改账。
// 已落 hash 且链仍 live 时禁止当失败解冻。

   func (s *Service) ConfirmAndCredit(ctx context.Context, id string) (err error) {
	  ctx, sp := trace.WithSpan(ctx, "ledger.ConfirmAndCredit")
	   defer func() { sp.End(s.log, err) }()
	   head, err := s.store.GetTransfer(ctx, id)
	   if err != nil {
		  return err
	  }
	  if head.State == StateCredited && !head.Direction.OnChain() {
		   return nil
	  }
	   if head.State == StateRolledBack {
		  return fmt.Errorf("%w", ErrRolledBack)
	   }
	   ctx = WithShardUser(ctx, head.UserID)
	   if head.Direction.OnChain() && !IsInternalTxHash(head.TxHash) && head.TxHash != "" && s.bridge != nil {
		   if _, ok := wallet.ObservationFrom(ctx, head.Chain, head.TxHash); !ok {
			   obs, ierr := s.bridge.Inspect(ctx, head.Chain, head.TxHash)
			   if ierr != nil && head.State != StateCredited {
				   return ierr
			   }
			   if ierr == nil {
				   ctx = wallet.WithObservation(ctx, head.Chain, head.TxHash, obs)
			   }
		   }
	   }
	   lease, err := s.lock.Lock(ctx, UserLockKey(head.UserID), s.lockTTL)
	   if err != nil {
		   return err
	  }
	  defer lease.Unlock()
	  if err := touchLease(ctx, lease); err != nil {
		   return err
	   }
	  ctx, err = s.beginFence(ctx, id, lease)
	  if err != nil {
		  return err
	  }
	   err = s.confirmLocked(ctx, id)
	  if err == nil && s.OnBooks != nil {
		   s.OnBooks(ctx, head.UserID)
	   }
	   return err
  }

// confirmLocked 持锁看链上确认：够深度则入账，失败则解冻，pending 留给 worker 再扫。
  func (s *Service) confirmLocked(ctx context.Context, id string) error {
	  cur, err := s.getFenced(ctx, id)
	   if err != nil {
		  return err
	   }
	   if cur.State == StateHold || cur.State == StateRolledBack {
		   if cur.State == StateRolledBack {
			  return fmt.Errorf("%w", ErrRolledBack)
		  }
		  return ErrObservationMismatch
	   }
	   if cur.State == StateReorged {
		   next,
			  err := Apply(cur.State, EventSubmit)
		  if err != nil {
			  return err
		   }
		  cur.State = next
		   if err := s.putFenced(ctx, cur); err != nil {
			  return err
		  }
	   }
	  if cur.Direction.OnChain() && !IsInternalTxHash(cur.TxHash) && cur.TxHash != "" && s.bridge != nil {
		   obs, ok := wallet.ObservationFrom(ctx, cur.Chain, cur.TxHash)
		   var err error
		   if !ok {
			   obs, err = s.bridge.Inspect(ctx, cur.Chain, cur.TxHash)
			   if err != nil && cur.State != StateCredited {
				   return err
			   }
		   }
		   if ok || err == nil {
			   if err := s.applyObservation(ctx, cur, obs); err != nil {
				   return err
			   }
			   cur, err = s.getFenced(ctx, id)
			   if err != nil {
				   return err
			   }
			  if cur.State == StateHold {
				  return ErrObservationMismatch
			  }
			   if obs.Status == wallet.TxFailed {
				   return s.store.InTx(ctx, func(ctx context.Context) error {
					  cur, err := s.getFenced(ctx, id)
					   if err != nil {
						   return err
					  }
					  return s.rollbackLocked(ctx, cur, "chain tx failed")
				   })
			   }
			  if obs.Reorged || obs.Confirmations < s.needConf(cur.Chain) {
				  if cur.State == StateCredited {
					  return s.unwindReorg(ctx, cur)
				  }
				   return ErrPending
			  }
			  if cur.State == StateCredited {
				   return nil
			  }
			   if obs.Status != wallet.TxConfirmed && obs.Confirmations < s.needConf(cur.Chain) {
				   return ErrPending
			  }
			  cur.CreditedHeight = obs.Height
			   cur.CreditedBlock = obs.BlockHash
		  }
	  }
	  if cur.State == StateCredited {
		   return nil
	  }
	   return s.store.InTx(ctx, func(ctx context.Context) error {
		   cur, err := s.getFenced(ctx, id)
		  if err != nil {
			   return err
		  }
		  if cur.State == StateCredited {
			  return nil
		   }
		   if cur.State == StateRolledBack {
			  return fmt.Errorf("%w", ErrRolledBack)
		   }
		   if cur.State == StateHold {
			  return ErrObservationMismatch
		  }
		   if cur.State == StateChainSubmitted {
			   next, err := Apply(cur.State, EventConfirm)
			   if err != nil {
				   return err
			   }
			  cur.State = next
			  if err := s.putFenced(ctx, cur); err != nil {
				   return err
			  }
			  _ = s.audit(ctx, cur, "confirmed", nil)
			   metrics.TransferByState.WithLabelValues(string(next), string(cur.Direction)).Inc()
		  }
		   return s.creditBooks(ctx, cur)
	   })
   }

// creditBooks 只从 confirmed 推进 credited。入金额必须等于 ObservedAmount，拒绝调用方金额。
// 出金只消耗冻结（现货/funding 不动）。同 (chain, hash) 已被别的单占用则拒，避免双花入账。
// 已有 credit 侧流水则只推状态，不再追加分录（重试安全）。
   func (s *Service) creditBooks(ctx context.Context, cur *Transfer) error {
	   if cur.State == StateCredited {
		   return nil
	   }
	  if cur.State != StateConfirmed {
		  return fmt.Errorf("cannot credit from %s", cur.State)
	   }
	  if err := s.rejectDuplicateChainHash(ctx, cur.Chain, cur.TxHash, cur.OutputIndex, cur.ID); err != nil {
		  return err
	  }
	   prev, err := s.store.ListJournal(ctx, cur.ID)
	  if err != nil {
		  return err
	  }
	  if cur.Direction == WalletDeposit {
		  if cur.ObservedAmount.IsZero() || !cur.ObservedAmount.Equal(cur.Amount) {
			  return fmt.Errorf("%w: refuse client amount credit", ErrObservationMismatch)
		  }
		   if cur.ObservedAsset != "" && cur.ObservedAsset != cur.Asset {
			   return fmt.Errorf("%w: asset", ErrObservationMismatch)
		  }
	   }
	  if settlementSide(prev) == "credit" {
		  next, err := Apply(cur.State, EventCredit)
		  if err != nil {
			  return err
		   }
		  cur.State = next
		   return s.putFenced(ctx, cur)
	   }
	   settleFee := s.withdrawFee(cur)
	   if cur.Direction == EarnRedeem {
		   settleFee = s.earnInterest(cur)
	   }
	  lines := settleLinesFee(cur, NextJournalSeq(prev), settleFee)
	  if err := s.store.AppendJournal(ctx, lines); err != nil {
		   return err
	   }
	   touched := map[string]*Account{}
	  get := func(userID string, typ AccountType, asset string) (*Account, error) {
		  k := userID + "|" + string(typ) + "|" + asset
		   if a, ok := touched[k]; ok {
			   return a, nil
		  }
		   a, err := s.store.GetAccount(ctx, userID, typ, asset)
		   if err != nil {
			  return nil, err
		   }
		   touched[k] = a
		   return a, nil
	  }
	  for _, e := range lines {
		  acc, err := get(e.UserID, e.AccountType, e.Asset)
		   if err != nil {
			   return err
		   }
		   if err := ApplyJournalToAccount(acc, e); err != nil {
			  return err
		  }
	  }
	   for _, a := range touched {
		  if err := requireFreshBook(a); err != nil {
			   return err
		  }
		  if err := s.store.UpsertAccount(ctx, a); err != nil {
			   return err
		  }
	  }
	  next, err := Apply(cur.State, EventCredit)
	  if err != nil {
		   return err
	  }
	  cur.State = next
	  metrics.TransferByState.WithLabelValues(string(next),
		   string(cur.Direction)).Inc()
	  if err := s.putFenced(ctx, cur); err != nil {
		  return err
	  }
	  return s.audit(ctx, cur, "credited", map[string]any{"settlement": cur.Direction.Settlement()})
   }

// FailAndRollback 链上失败回滚冻结；与 ConfirmAndCredit 抢同一把用户锁。hash 仍 live 时不得当失败解冻。
  func (s *Service) FailAndRollback(ctx context.Context, id, reason string) error {
	   cur0, err := s.store.GetTransfer(ctx, id)
	   if err != nil {
		  return err
	   }
	   ctx = WithShardUser(ctx, cur0.UserID)
	   lease,
		  err := s.lock.Lock(ctx, UserLockKey(cur0.UserID), s.lockTTL)
	   if err != nil {
		  return err
	   }
	  defer lease.Unlock()
	   ctx,
		  err = s.beginFence(ctx, id, lease)
	   if err != nil {
		  return err
	  }
	   cur0, err = s.getFenced(ctx, id)
	   if err != nil {
		   return err
	  }
	  if err := s.guardOnChainUnfreeze(ctx, cur0); err != nil {
		   return err
	  }
	   err = s.store.InTx(ctx, func(ctx context.Context) error {
		  cur, err := s.getFenced(ctx, id)
		   if err != nil {
			  return err
		   }
		   return s.rollbackLocked(ctx, cur, reason)
	   })
	  if err == nil && s.OnBooks != nil {
		  s.OnBooks(ctx, cur0.UserID)
	   }
	  return err
  }

// rollbackLocked 调用方已持用户锁和 fence。
// credited / rolled_back 直接返回，禁止二次解冻把钱变出来。
// 没有冻结流水（入金或冻前失败）只推状态；已有 unfreeze 分录也不再写。
// 有冻结则 frozen→available 一对，轧平后才落账。链 hash 仍 live 时不得走到这里，见 guardOnChainUnfreeze。
  func (s *Service) rollbackLocked(ctx context.Context, cur *Transfer, reason string) error {
	  if cur.State == StateCredited || cur.State == StateRolledBack {
		   return nil
	   }
	   if CanApply(cur.State, EventFail) {
		  cur.State, _ = Apply(cur.State, EventFail)
		  cur.FailReason = reason
		   if err := s.putFenced(ctx, cur); err != nil {
			  return err
		  }
	   }
	  if !CanApply(cur.State, EventRollback) {
		   return nil
	  }
	   prev,
		   err := s.store.ListJournal(ctx, cur.ID)
	  if err != nil {
		  return err
	  }
	   hasFreeze,
		  hasUnfreeze, _ := journalFlags(prev)
	  if !hasFreeze {
		  // 充值或冻前失败，没有可还的库存。
		   cur.State, _ = Apply(cur.State, EventRollback)
		   if err := s.putFenced(ctx, cur); err != nil {
			  return err
		   }
		  metrics.TransferByState.WithLabelValues(string(cur.State), string(cur.Direction)).Inc()
		   return s.audit(ctx, cur, "rolled_back", map[string]any{"reason": reason})
	   }
	  book := sourceBook(cur.Direction)
	  if hasUnfreeze {
		  cur.State,
			   _ = Apply(cur.State, EventRollback)
		  return s.putFenced(ctx, cur)
	  }
	  lines := rollbackLines(cur, book, NextJournalSeq(prev))
	  if err := s.store.AppendJournal(ctx, lines); err != nil {
		   return err
	  }
	   acc, err := s.store.GetAccount(ctx, cur.UserID, book, cur.Asset)
	   if err != nil {
		  return err
	   }
	   for _, e := range lines {
		  if err := ApplyJournalToAccount(acc, e); err != nil {
			   return err
		   }
	   }
	   if err := requireFreshBook(acc); err != nil {
		   return err
	   }
	   if err := s.store.UpsertAccount(ctx, acc); err != nil {
		   return err
	  }
	   cur.State, _ = Apply(cur.State, EventRollback)
	  if err := s.putFenced(ctx, cur); err != nil {
		   return err
	   }
	  metrics.TransferByState.WithLabelValues(string(cur.State), string(cur.Direction)).Inc()
	   return s.audit(ctx, cur, "rolled_back", map[string]any{"reason": reason})
  }

// touchLease 广播前续租，避免长 RPC 把用户锁续丢。

  func touchLease(ctx context.Context, lease Guard) error {
	   if lease == nil {
		   return nil
	  }
	  return lease.Touch(ctx)
   }

// guardOnChainUnfreeze 链上 hash 仍 pending/confirmed 时禁止解冻；只有链报失败才能还冻结。

   func (s *Service) guardOnChainUnfreeze(ctx context.Context, cur *Transfer) error {
	   if cur == nil || !cur.Direction.OnChain() {
		   return nil
	   }
	   switch cur.State {
	   case StateChainSubmitted, StateConfirmed:
	  default:
		  return nil
	   }
	   if cur.TxHash == "" || IsInternalTxHash(cur.TxHash) {
		   return nil
	   }
	   if s.bridge == nil {
		  return fmt.Errorf("%w: no chain bridge", ErrSubmittedLive)
	  }
	  st, err := s.bridge.Status(ctx, cur.Chain, cur.TxHash)
	  if err != nil {
		   return err
	  }
	  if st == wallet.TxFailed {
		  return nil
	   }
	   return fmt.Errorf("%w: chain status %s", ErrSubmittedLive, st)
   }

// Get 按单号取划转；HTTP/gRPC 查询都走这里。
   func (s *Service) Get(ctx context.Context, id string) (*Transfer, error) {
	  return s.store.GetTransfer(ctx, id)
  }

// List 用户划转列表，给资金记录页。

  func (s *Service) List(ctx context.Context, userID string, limit int) ([]*Transfer, error) {
	  return s.store.ListUserTransfers(ctx, userID, limit)
   }

// ListIncomingInner 本人作为对手方收到的内部转入。

  func (s *Service) ListIncomingInner(ctx context.Context, userID string, limit int) ([]*Transfer, error) {
	   if s == nil || s.store == nil {
		  return nil, ErrNotFound
	  }
	  return s.store.ListIncomingInner(ctx, strings.TrimSpace(userID), limit)
  }

// ListAudit 单据审计事件，给排障和核验留痕。

   func (s *Service) ListAudit(ctx context.Context, entityType, entityID string, limit int) ([]AuditEvent, error) {
	  return s.store.ListAudit(ctx, entityType, entityID, limit)
   }

// RecoverStuck 扫超时冻结/已提交单：从未广播的冻结可回滚；已提交只确认不自动解冻，避免晚到账双花。
  func (s *Service) RecoverStuck(ctx context.Context) {
	   now := time.Now().UTC()
	  initiated,
		  err := s.store.ListByState(ctx, StateInitiated, 200)
	   if err == nil {
		   for _, tr := range initiated {
			  if tr.TxHash != "" && !IsInternalTxHash(tr.TxHash) {
				   // 已经有 hash，不能自动解冻。
				   if tr.Direction == WalletDeposit {
					  lease, e2 := s.lock.Lock(ctx, UserLockKey(tr.UserID), s.lockTTL)
					   if e2 != nil {
						   continue
					  }
					   fctx, e3 := s.beginFence(ctx, tr.ID, lease)
					  if e3 == nil {
						  _ = s.markSubmitted(fctx, tr.ID, tr.TxHash)
					  }
					  lease.Unlock()
				  }
				  continue
			   }
			   age := now.Sub(tr.CreatedAt)
			   if tr.CreatedAt.IsZero() || age < s.stuckFrozen {
				  continue
			   }
			   metrics.FrozenStuck.WithLabelValues(string(StateInitiated)).Inc()
			  _ = s.FailAndRollback(ctx, tr.ID, "stuck initiated without progress")
		   }
	   }
	   for _, st := range []State{StateSpotFrozen, StateWalletLocked} {
		   list,
			   err := s.store.ListByState(ctx, st, 200)
		   if err != nil {
			  continue
		  }
		  for _, tr := range list {
			   if tr.TxHash != "" || kytReview(tr) {
				  continue
			   }
			   age := now.Sub(tr.CreatedAt)
			   if tr.CreatedAt.IsZero() || age < s.stuckFrozen {
				   continue
			   }
			   metrics.FrozenStuck.WithLabelValues(string(st)).Inc()
			  _ = s.FailAndRollback(ctx, tr.ID, "stuck freeze without chain submit")
		   }
	   }
	   list, err := s.store.ListByState(ctx, StateChainSubmitted, 200)
	  if err != nil {
		  return
	  }
	   for _, tr := range list {
		   age := now.Sub(tr.CreatedAt)
		   if tr.CreatedAt.IsZero() || age < s.stuckSubmitted {
			  continue
		  }
		   metrics.FrozenStuck.WithLabelValues(string(StateChainSubmitted)).Inc()
		  _ = s.audit(ctx, tr, "stuck_submitted", map[string]any{"age_ms": age.Milliseconds()})
		   if tr.Direction.OnChain() {
			  _ = s.ConfirmAndCredit(ctx, tr.ID)
		  }
	  }
	   credited, err := s.store.ListByState(ctx, StateCredited, 200)
	   if err == nil {
		  for _, tr := range credited {
			  if tr.Direction.OnChain() && tr.TxHash != "" && !IsInternalTxHash(tr.TxHash) {
				  _ = s.ConfirmAndCredit(ctx, tr.ID)
			  }
		  }
	   }
	  reorged,
		   err := s.store.ListByState(ctx, StateReorged, 200)
	  if err == nil {
		   for _, tr := range reorged {
			  _ = s.ConfirmAndCredit(ctx, tr.ID)
		   }
	   }
   }

  func (s *Service) Store() Store { return s.store }

// audit 写审计并同事务入出箱，worker 再投递 wallet.movements。

   func (s *Service) audit(ctx context.Context, tr *Transfer, action string, detail map[string]any) error {
	  if detail == nil {
		   detail = map[string]any{}
	  }
	  detail["state"] = string(tr.State)
	  detail["amount"] = tr.Amount.String()
	   detail["movement_id"] = tr.ID
	   kind := tr.Direction.Settlement()
	  detail["kind"] = kind
	  tid := trace.ID(ctx)
	  if tid != "" {
		  detail["trace_id"] = tid
	   }
	  if err := s.store.AppendAudit(ctx, AuditEvent{
		   EntityType: "transfer",
		   EntityID:   tr.ID,
		  Action:     action,
		   Actor:  "system",
		   Detail: detail,
	   }); err != nil {
		  return err
	   }
	   metrics.OutboxEvents.WithLabelValues("enqueued").Inc()
	   return s.store.AppendOutbox(ctx, OutboxEvent{
		   ID: uuid.NewString(),
		   AggregateType: events.AggregateTransfer,
		   AggregateID:   tr.ID,
		   Kind:          events.KindFromSettlement(kind),
		   Action:  action,
		  UserID:  tr.UserID,
		   TraceID: tid,
		  State:   string(tr.State),
		   Payload: detail,
	   })
  }

// CoverFundingFromWallet 资金账可用不足时，从钱包账补划到资金账。
// KuCoin MAIN 对应本系统 wallet+spot；对外划转/闪兑按 MAIN 总额过账。
  func (s *Service) CoverFundingFromWallet(ctx context.Context, userID, asset string, need decimal.Decimal, idem string) error {
	   if s == nil || s.store == nil || !need.IsPositive() {
		  return nil
	   }
	   userID = strings.TrimSpace(userID)
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	  idem = strings.TrimSpace(idem)
	  if userID == "" || asset == "" {
		  return ErrNotFound
	   }
	   if idem == "" {
		  return ErrMissingIdempotency
	   }
	   spot, err := s.store.GetAccount(ctx, userID, AccountSpot, asset)
	   if err != nil {
		  return err
	   }
	   if spot.Available.GreaterThanOrEqual(need) {
		  return nil
	  }
	   short := need.Sub(spot.Available)
	  wal, err := s.store.GetAccount(ctx, userID, AccountWallet, asset)
	  if err != nil {
		  return err
	   }
	   if wal.Available.LessThan(short) {
		  short = wal.Available
	   }
	  if !short.IsPositive() {
		  return nil
	   }
	   _, err = s.Create(ctx, CreateReq{
		   UserID: userID, Direction: WalletToSpot, Asset: asset, Amount: short,
		  IdempotencyKey: idem,
	  })
	  return err
  }

// CoverFundingFromTrading 资金账仍不足时，从交易账补划。闪兑 BOTH 用 Funding+Trading 合计。

  func (s *Service) CoverFundingFromTrading(ctx context.Context, userID, asset string, need decimal.Decimal, idem string) error {
	   if s == nil || s.store == nil || !need.IsPositive() {
		   return nil
	   }
	  userID = strings.TrimSpace(userID)
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   idem = strings.TrimSpace(idem)
	  if userID == "" || asset == "" {
		   return ErrNotFound
	  }
	   if idem == "" {
		   return ErrMissingIdempotency
	  }
	  spot, err := s.store.GetAccount(ctx, userID, AccountSpot, asset)
	   if err != nil {
		  return err
	  }
	  if spot.Available.GreaterThanOrEqual(need) {
		   return nil
	   }
	  short := need.Sub(spot.Available)
	  trd, err := s.store.GetAccount(ctx, userID, AccountTrading, asset)
	  if err != nil {
		  return err
	  }
	   if trd.Available.LessThan(short) {
		  short = trd.Available
	   }
	   if !short.IsPositive() {
		  return nil
	   }
	  _, err = s.Create(ctx, CreateReq{
		   UserID: userID, Direction: TradingToFunding, Asset: asset, Amount: short,
		  IdempotencyKey: idem,
	  })
	   return err
  }

// EnsureFunding 资金账可用必须达到 need。先补钱包，再按需补交易账；仍不足则拒。
  func (s *Service) EnsureFunding(ctx context.Context, userID, asset string, need decimal.Decimal, idem string, fromTrading bool) error {
	  if err := s.CoverFundingFromWallet(ctx, userID, asset, need, idem+":w"); err != nil {
		   return err
	   }
	  if fromTrading {
		   if err := s.CoverFundingFromTrading(ctx, userID, asset, need, idem+":t"); err != nil {
			  return err
		  }
	   }
	  if s == nil || s.store == nil || !need.IsPositive() {
		   return nil
	  }
	   spot, err := s.store.GetAccount(ctx, strings.TrimSpace(userID), AccountSpot, strings.ToUpper(strings.TrimSpace(asset)))
	  if err != nil {
		   return err
	   }
	  if spot.Available.LessThan(need) {
		   return ErrInsufficient
	   }
	   return nil
  }

// DecryptAddress 提现目标只存密文，广播前才解开；空明文不得 Submit。

  func (s *Service) DecryptAddress(tr *Transfer) string {
	  if tr == nil {
		  return ""
	  }
	  if strings.TrimSpace(tr.ToAddress) != "" {
		  return strings.TrimSpace(tr.ToAddress)
	   }
	   if s.enc == nil || len(tr.ToAddressEnc) == 0 {
		   return ""
	   }
	   b, err := s.enc.Decrypt(tr.ToAddressEnc)
	  if err != nil {
		  return ""
	  }
	   metrics.KMSOps.WithLabelValues("decrypt_address").Inc()
	  return strings.TrimSpace(string(b))
   }

// destination 提现广播地址。库里只存密文；解开后仍要过形态闸，空地址或垃圾形态不得 Submit。
  func (s *Service) destination(tr *Transfer) (string, error) {
	  addr := s.DecryptAddress(tr)
	  if tr != nil && tr.Direction == WalletWithdraw {
		  if addr == "" {
			  return "", ErrMissingAddress
		   }
		  if err := validateChainAddress(tr.Chain, addr); err != nil {
			   return "", err
		  }
	   }
	  return addr, nil
  }
