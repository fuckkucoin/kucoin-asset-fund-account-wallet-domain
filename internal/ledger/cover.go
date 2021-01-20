// 现货资金账本核心域之轧差补平。

// Author: Axel
// Date: 2021.01.05

package ledger

import (
	"context"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/auditchain/ingest"
	"account-assets.kucoin.net/funding/internal/metrics"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// coverSourceLocked 持用户锁后、主单落库前补齐源账。
// 资金账出发的冻单先从钱包补；出金从资金账补钱包；CoverTrading 再补交易账。

  func coverAttemptKey(base string, n int) string {
	  if n <= 1 {
		   return base
	   }
	   return fmt.Sprintf("%s:%d", base, n)
  }

  func coverBases(req CreateReq) []string {
	  if req.Direction == WalletWithdraw {
		  return []string{req.IdempotencyKey + ":auto-s"}
	  }
	   if req.Direction.Freezes() && sourceBook(req.Direction) == AccountSpot {
		   out := []string{req.IdempotencyKey + ":auto-w"}
		   if req.CoverTrading {
			   out = append(out, req.IdempotencyKey+":auto-t")
		   }
		   return out
	  }
	   return nil
  }

   func (s *Service) liveCoverIdem(ctx context.Context, base string) string {
	  for n := 1; n < 32; n++ {
		  key := coverAttemptKey(base, n)
		   got, err := s.store.GetByIdempotency(ctx, key)
		   if err != nil || got == nil {
			  return key
		  }
		   undo, uerr := s.store.GetByIdempotency(ctx, key+":undo")
		   if uerr == nil && undo != nil && undo.State == StateCredited {
			   continue
		   }
		  return key
	   }
	   return coverAttemptKey(base, 32)
   }

   func (s *Service) coverSourceLocked(ctx context.Context, lease Guard, req CreateReq) error {
	   if s == nil || s.store == nil || !req.Amount.IsPositive() {
		   return nil
	   }
	  if req.Direction == WalletWithdraw {
		   return s.coverBookLocked(ctx, lease, req.UserID, req.Asset, req.Amount, s.liveCoverIdem(ctx, req.IdempotencyKey+":auto-s"), SpotToWallet)
	   }
	   if req.Direction.Freezes() && sourceBook(req.Direction) == AccountSpot {
		  if err := s.coverBookLocked(ctx, lease, req.UserID, req.Asset, req.Amount, s.liveCoverIdem(ctx, req.IdempotencyKey+":auto-w"), WalletToSpot); err != nil {
			  return err
		   }
		  if req.CoverTrading {
			  return s.coverBookLocked(ctx, lease, req.UserID, req.Asset, req.Amount, s.liveCoverIdem(ctx, req.IdempotencyKey+":auto-t"), TradingToFunding)
		   }
	  }
	   return nil
  }

// coverBookLocked 把 dest 可用补到 need：从 source 划 short。已有同幂等键则续跑，不二次冻。
// 调用方必须已持用户锁；同用户并发 Create 串行后按 dest 可用合并补账，禁止各划一笔把源账抽空。
  func (s *Service) coverBookLocked(ctx context.Context, lease Guard, userID, asset string, need decimal.Decimal, idem string, dir Direction) error {
	   userID = strings.TrimSpace(userID)
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	  idem = strings.TrimSpace(idem)
	   if userID == "" || asset == "" || idem == "" || !need.IsPositive() {
		  return nil
	  }
	  dst, err := s.store.GetAccount(ctx, userID, destBook(dir), asset)
	   if err != nil {
		   return err
	  }
	  if dst.Available.GreaterThanOrEqual(need) {
		   return nil
	   }
	  short := need.Sub(dst.Available)
	  src, err := s.store.GetAccount(ctx, userID, sourceBook(dir), asset)
	   if err != nil {
		   return err
	  }
	  if src.Available.LessThan(short) {
		  short = src.Available
	  }
	  if !short.IsPositive() {
		   return nil
	   }
	   if existing, err := s.store.GetByIdempotency(ctx, idem); err == nil && existing != nil {
		   if existing.State == StateCredited || existing.State == StateRolledBack {
			   return nil
		   }
		   fctx, e2 := s.beginFence(ctx, existing.ID, lease)
		   if e2 != nil {
			  return e2
		   }
		   return s.settleInternalLocked(fctx, existing.ID)
	   }
	  tr := &Transfer{
		  ID: uuid.NewString(), IdempotencyKey: idem, UserID: userID,
		   Direction: dir, Asset: asset, Amount: short,
		  State: StateInitiated, CreatedAt: time.Now().UTC(),
	  }
	   if err := s.store.InsertTransfer(ctx, tr); err != nil {
		  if existing, e2 := s.store.GetByIdempotency(ctx, idem); e2 == nil && existing != nil {
			   if existing.State == StateCredited {
				  return nil
			   }
			  fctx, e3 := s.beginFence(ctx, existing.ID, lease)
			   if e3 != nil {
				   return e3
			  }
			  return s.settleInternalLocked(fctx, existing.ID)
		   }
		  return err
	  }
	  fctx, err := s.beginFence(ctx, tr.ID, lease)
	   if err != nil {
		  return err
	  }
	   return s.settleInternalLocked(fctx, tr.ID)
   }

   func inverseCover(dir Direction) (Direction, bool) {
	   switch dir {
	  case WalletToSpot:
		   return SpotToWallet, true
	   case SpotToWallet:
		   return WalletToSpot, true
	   case TradingToFunding:
		   return FundingToTrading, true
	  default:
		   return "", false
	   }
  }

// undoCoverLocked 主单未落库时把自动补账划回去。按补账单实额反转，禁止按主单金额空头回滚。

  func (s *Service) undoCoverLocked(ctx context.Context, lease Guard, req CreateReq) error {
	   if s == nil || s.store == nil {
		   return nil
	   }
	  var first error
	  for _, base := range coverBases(req) {
		  for n := 1; n < 32; n++ {
			  key := coverAttemptKey(base, n)
			   got, err := s.store.GetByIdempotency(ctx, key)
			  if err != nil || got == nil {
				   break
			  }
			   if err := s.reverseCoverLocked(ctx, lease, key, key+":undo"); err != nil && first == nil {
				  first = err
			   }
		   }
	  }
	   return first
   }

// reverseCoverLocked 已入账的补账单按原额反方向划回；同撤销键可续跑。
  func (s *Service) reverseCoverLocked(ctx context.Context, lease Guard, coverIdem, undoIdem string) error {
	  coverIdem = strings.TrimSpace(coverIdem)
	  undoIdem = strings.TrimSpace(undoIdem)
	  if coverIdem == "" || undoIdem == "" {
		   return nil
	  }
	  got, err := s.store.GetByIdempotency(ctx, coverIdem)
	  if err != nil || got == nil || got.State != StateCredited || !got.Amount.IsPositive() {
		   return nil
	   }
	  if err := s.consumeCoverUndoFault(); err != nil {
		   return err
	  }
	  inv, ok := inverseCover(got.Direction)
	   if !ok {
		  return nil
	  }
	  if existing, err := s.store.GetByIdempotency(ctx, undoIdem); err == nil && existing != nil {
		   if existing.State == StateCredited || existing.State == StateRolledBack {
			   return nil
		  }
		  fctx,
			   e2 := s.beginFence(ctx, existing.ID, lease)
		  if e2 != nil {
			   return e2
		  }
		   return s.settleInternalLocked(fctx, existing.ID)
	   }
	   tr := &Transfer{
		   ID: uuid.NewString(), IdempotencyKey: undoIdem, UserID: got.UserID,
		  Direction: inv,
		   Asset:     got.Asset, Amount: got.Amount,
		  State: StateInitiated, CreatedAt: time.Now().UTC(),
	   }
	  if err := s.store.InsertTransfer(ctx, tr); err != nil {
		   if existing, e2 := s.store.GetByIdempotency(ctx, undoIdem); e2 == nil && existing != nil {
			   if existing.State == StateCredited {
				  return nil
			  }
			  fctx,
				  e3 := s.beginFence(ctx, existing.ID, lease)
			  if e3 != nil {
				   return e3
			  }
			  return s.settleInternalLocked(fctx, existing.ID)
		   }
		   return err
	   }
	   fctx,
		  err := s.beginFence(ctx, tr.ID, lease)
	   if err != nil {
		  return err
	  }
	  if err := s.settleInternalLocked(fctx, tr.ID); err != nil {
		  return err
	  }
	  // 读己写：撤销入账必须在本租约内可见，禁止只信进程内状态。
	  seen, err := s.store.GetByIdempotency(ctx, undoIdem)
	  if err != nil || seen == nil || seen.State != StateCredited {
		  return fmt.Errorf("%w: undo not visible after settle", ErrCoverStranded)
	   }
	   return nil
  }

// IsAutoCoverKey 自动补账幂等键（不含已撤销的 :undo）。清扫与划转工作台共用，避免两套后缀。
  func IsAutoCoverKey(key string) bool { return isCoverIdem(key) }

// CoverParentKey 从补账幂等键还原主单键。解析不到返回空串。

  func CoverParentKey(key string) string { return coverParentIdem(key) }

   func isCoverIdem(key string) bool {
	   key = strings.TrimSpace(key)
	  if key == "" || strings.Contains(key, ":undo") {
		  return false
	  }
	   return strings.Contains(key, ":auto-s") || strings.Contains(key, ":auto-w") || strings.Contains(key, ":auto-t")
  }

   func coverParentIdem(key string) string {
	  for _, mark := range []string{":auto-s", ":auto-w", ":auto-t"} {
		   if i := strings.Index(key, mark); i >= 0 {
			  return key[:i]
		  }
	   }
	   return ""
  }

  func (s *Service) coverSweepAge() time.Duration {
	   if s != nil && s.coverSweepMinAge > 0 {
		   return s.coverSweepMinAge
	   }
	   ttl := time.Duration(0)
	  if s != nil {
		   ttl = s.lockTTL
	   }
	  if ttl <= 0 {
		  ttl = 12 * time.Second
	   }
	   return ttl + 2*time.Second
  }

// SetCoverSweepMinAge 单测缩短时钟偏差等待；保持锁 TTL+偏斜。
  func (s *Service) SetCoverSweepMinAge(d time.Duration) {
	   if s == nil {
		   return
	  }
	   s.coverSweepMinAge = d
   }

type CoverSweepReport struct {
	Scanned int
	Undone int
	Stranded int
	Skipped int
}

   func (s *Service) alert(kind, detail string) {
	   if s != nil && s.OnAlert != nil {
		   s.OnAlert(kind, detail)
	  }
  }

// SweepStrandedCovers 后台清残留补账：不依赖同幂等键 Create。未过锁 TTL+时钟偏斜的跳过，避免与在途 Create 打架。
   func (s *Service) SweepStrandedCovers(ctx context.Context, limit int) (CoverSweepReport, error) {
	  var rep CoverSweepReport
	  if s == nil || s.store == nil {
		  return rep, nil
	  }
	  if limit <= 0 {
		  limit = 200
	  }
	   if ctx == nil {
		  ctx = context.Background()
	   }
	   minAge := s.coverSweepAge()
	  now := time.Now().UTC()
	   list, err := s.store.ListByState(ctx, StateCredited, limit)
	   if err != nil {
		   return rep, err
	  }
	  for _, tr := range list {
		  if tr == nil || !isCoverIdem(tr.IdempotencyKey) {
			  continue
		   }
		   rep.Scanned++
		   if now.Sub(tr.CreatedAt) < minAge {
			  rep.Skipped++
			   metrics.CoverSweep.WithLabelValues("young").Inc()
			   continue
		  }
		   parent := coverParentIdem(tr.IdempotencyKey)
		   main, merr := s.store.GetByIdempotency(ctx, parent)
		  if merr == nil && main != nil && main.State != StateFailed && main.State != StateRolledBack {
			  rep.Skipped++
			   metrics.CoverSweep.WithLabelValues("in_use").Inc()
			   continue
		  }
		  undo, uerr := s.store.GetByIdempotency(ctx, tr.IdempotencyKey+":undo")
		   if uerr == nil && undo != nil && undo.State == StateCredited {
			   rep.Skipped++
			   metrics.CoverSweep.WithLabelValues("already_undone").Inc()
			   continue
		  }
		   locked,
			  lease, lerr := s.LockUser(ctx, tr.UserID)
		  if lerr != nil {
			  rep.Stranded++
			   metrics.CoverSweep.WithLabelValues("lock_fail").Inc()
			   metrics.CoverStranded.Inc()
			  s.alert("cover_stranded", tr.IdempotencyKey+": "+lerr.Error())
			  continue
		   }
		   var last error
		   for i := 0; i < 3; i++ {
			  last = s.reverseCoverLocked(locked, lease, tr.IdempotencyKey, tr.IdempotencyKey+":undo")
			  if last == nil {
				   break
			   }
		   }
		  if lease != nil {
			  lease.Unlock()
		   }
		   err := last
		  if err != nil {
			  rep.Stranded++
			  metrics.CoverSweep.WithLabelValues("fail_closed").Inc()
			   metrics.CoverStranded.Inc()
			   _ = s.store.AppendAudit(locked, AuditEvent{
				  EntityType: "cover", EntityID: tr.ID, Action: "sweep_fail", Actor: "cover-sweeper",
				   Detail: map[string]any{"idem": tr.IdempotencyKey, "reason": err.Error()}, CreatedAt: now,
			  })
			   s.alert("cover_stranded", tr.IdempotencyKey+": "+err.Error())
			  ingest.Try(locked, ingest.CoverFact(tr.UserID, tr.Asset, tr.Amount.String(), tr.ID, "ledger.cover.fail", "cover-sweeper", now))
			  continue
		  }
		   rep.Undone++
		  metrics.CoverSweep.WithLabelValues("undone").Inc()
		   _ = s.store.AppendAudit(locked, AuditEvent{
			   EntityType: "cover", EntityID: tr.ID, Action: "sweep_undo", Actor: "cover-sweeper",
			  Detail: map[string]any{"idem": tr.IdempotencyKey, "parent": parent}, CreatedAt: now,
		   })
		  ingest.Try(locked, ingest.CoverFact(tr.UserID, tr.Asset, tr.Amount.String(), tr.ID, "ledger.cover.undo", "cover-sweeper", now))
	   }
	   return rep, nil
   }
