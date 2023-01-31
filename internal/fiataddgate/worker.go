package fiataddgate

import (
	"context"
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/internal/fiatadd"
	"github.com/rs/zerolog"
)

// ForkPatrolConfig 分叉巡检配置。

type ForkPatrolConfig struct {
	Interval time.Duration
	BatchLimit int
}

// ForkPatrolWorker 分叉案例巡检 worker。
// 定期扫描 open 案例，打印日志/指标，供监控告警使用。

type ForkPatrolWorker struct {
	forks ForkCaseStore
	log zerolog.Logger
	cfg ForkPatrolConfig
	metrics ForkMetrics
}

// ForkMetrics 分叉指标接口（可注入 Prometheus counter）。
type ForkMetrics interface {
	IncOpenForkCase(kind ForkKind)
	IncForkPatrolScan()
}

// noopMetrics 空指标实现（默认）。
type noopMetrics struct{}

  func (noopMetrics) IncOpenForkCase(ForkKind) {}
   func (noopMetrics) IncForkPatrolScan()       {}

// NewForkPatrolWorker 构造分叉巡检 worker。
   func NewForkPatrolWorker(forks ForkCaseStore, log zerolog.Logger, cfg ForkPatrolConfig, m ForkMetrics) *ForkPatrolWorker {
	   if m == nil {
		  m = noopMetrics{}
	  }
	   if cfg.BatchLimit <= 0 {
		   cfg.BatchLimit = 50
	   }
	  if cfg.Interval <= 0 {
		   cfg.Interval = 30 * time.Second
	   }
	   return &ForkPatrolWorker{forks: forks, log: log, cfg: cfg, metrics: m}
  }

// Run 启动巡检循环，直到 ctx 取消。

  func (w *ForkPatrolWorker) Run(ctx context.Context) error {
	   t := time.NewTicker(w.cfg.Interval)
	  defer t.Stop()
	  for {
		   select {
		   case <-ctx.Done():
			   return ctx.Err()
		   case <-t.C:
			   if err := w.scan(ctx); err != nil {
				  w.log.Error().Err(err).Msg("fiataddgate: fork patrol scan")
			   }
		   }
	  }
   }

// RunOnce 单次扫描（供使用）。

  func (w *ForkPatrolWorker) RunOnce(ctx context.Context) error {
	  return w.scan(ctx)
   }

   func (w *ForkPatrolWorker) scan(ctx context.Context) error {
	   w.metrics.IncForkPatrolScan()
	  cases, err := w.forks.ListOpen(ctx, w.cfg.BatchLimit)
	   if err != nil {
		   return fmt.Errorf("fork patrol: list open: %w", err)
	   }
	   for _, fc := range cases {
		  w.metrics.IncOpenForkCase(fc.Kind)
		   w.log.Warn().
			   Str("case_id", fc.CaseID).
			   Str("session_id", fc.SessionID).
			  Int64("confirm_gen", fc.ConfirmGen).
			   Str("kind", string(fc.Kind)).
			  Str("opened_at", fc.OpenedAt.Format(time.RFC3339)).
			  Msg("fiataddgate: open fork case detected; requires ops intervention")
	  }
	   if len(cases) > 0 {
		   w.log.Error().Int("count", len(cases)).Msg("fiataddgate: open fork cases require attention")
	   }
	   return nil
   }

// RecycleConfig 未决 confirming 回收配置。

type RecycleConfig struct {
	Interval   time.Duration
	BatchLimit int
	// StaleAfter confirming 超过此时长且租约已过期才纳入回收。
	StaleAfter time.Duration
}

// PendingConfirmingRecycler 回收长时间停留在 confirming 的 Session。
// 策略：
// 1. 拉取已过期（ExpiresAt < now）且 state=issued 的 ConfirmLease。
// 2. 读取对应 Session：若已是 credited/forked，将租约推进到 credited/forked。
// 3. 若 Session 仍在 confirming 且探测无 ledger 贷记，将 Session 回滚到 pending，租约推进到 expired。
// 4. 若 Session 仍在 confirming 且探测到 ledger 贷记，打开 ForkCase，租约推进到 forked。

type PendingConfirmingRecycler struct {
	book   *fiatadd.Book
	leases LeaseStore
	forks  ForkCaseStore
	ledger TransferReader
	log zerolog.Logger
	cfg RecycleConfig
	clock func() time.Time
}

// NewPendingConfirmingRecycler 构造回收 worker。
   func NewPendingConfirmingRecycler(
	  book *fiatadd.Book,
	   leases LeaseStore,
	   forks ForkCaseStore,
	   ledger TransferReader,
	   log zerolog.Logger,
	  cfg RecycleConfig,
   ) *PendingConfirmingRecycler {
	  if cfg.BatchLimit <= 0 {
		  cfg.BatchLimit = 20
	   }
	   if cfg.Interval <= 0 {
		  cfg.Interval = 60 * time.Second
	  }
	  if cfg.StaleAfter <= 0 {
		  cfg.StaleAfter = 90 * time.Second
	   }
	  return &PendingConfirmingRecycler{
		   book: book, leases: leases, forks: forks, ledger: ledger,
		   log: log, cfg: cfg, clock: func() time.Time { return time.Now().UTC() },
	   }
   }

  func (r *PendingConfirmingRecycler) WithClock(c func() time.Time) *PendingConfirmingRecycler {
	  if r != nil && c != nil {
		  r.clock = c
	  }
	   return r
  }

  // Run 启动回收循环，直到 ctx 取消。
  func (r *PendingConfirmingRecycler) Run(ctx context.Context) error {
	   t := time.NewTicker(r.cfg.Interval)
	  defer t.Stop()
	   for {
		  select {
		  case <-ctx.Done():
			   return ctx.Err()
		  case <-t.C:
			  if err := r.runOnce(ctx); err != nil {
				   r.log.Error().Err(err).Msg("fiataddgate: recycle confirming")
			  }
		  }
	  }
  }

// RunOnce 单次回收（供）。

   func (r *PendingConfirmingRecycler) RunOnce(ctx context.Context) error {
	  return r.runOnce(ctx)
  }

  func (r *PendingConfirmingRecycler) runOnce(ctx context.Context) error {
	   now := r.clock()
	   expired, err := r.leases.ListExpired(ctx, now, r.cfg.BatchLimit)
	  if err != nil {
		  return fmt.Errorf("recycle: list expired leases: %w", err)
	  }
	  for _, lease := range expired {
		  if err := r.processLease(ctx, lease, now); err != nil {
			  r.log.Error().Err(err).
				   Str("lease_id", lease.LeaseID).
				  Str("session_id", lease.SessionID).
				  Msg("fiataddgate: process expired lease")
		  }
	   }
	   return nil
  }

  func (r *PendingConfirmingRecycler) processLease(ctx context.Context, lease ConfirmLease, now time.Time) error {
	  sess, err := r.book.Get(ctx, "", lease.SessionID)
	   if err != nil {
		  return fmt.Errorf("recycle: load session %s: %w", lease.SessionID, err)
	   }
	  switch sess.Status {
	   case "credited":
		  // Session 已完成：租约推进到 credited
		  t := now
		  return r.leases.Transition(ctx, lease.LeaseID, LeaseIssued, LeaseCredited, ConfirmLease{
			   CreditID:   sess.CreditID,
			   CreditedAt: &t,
			  Note:       "recycler: session already credited",
		  })
	   case "forked":
		  // Session 已分叉：租约推进到 forked
		  t := now
		  return r.leases.Transition(ctx, lease.LeaseID, LeaseIssued, LeaseForked, ConfirmLease{
			  ForkedAt: &t,
			   Note:     "recycler: session already forked",
		  })
	   case "confirming":
		  // Session 仍在 confirming：探测 ledger
		   return r.recycleConfirming(ctx, lease, sess, now)
	   default:
		  // pending 或其他：直接过期租约
		   t := now
		   return r.leases.Transition(ctx, lease.LeaseID, LeaseIssued, LeaseExpired, ConfirmLease{
			   ExpiredAt: &t,
			   Note:      fmt.Sprintf("recycler: session status=%s; lease expired", sess.Status),
		  })
	  }
   }

  func (r *PendingConfirmingRecycler) recycleConfirming(ctx context.Context, lease ConfirmLease, sess fiatadd.Session, now time.Time) error {
	   // 检查 ledger 是否已有对应贷记
	   ledgerCredited := false
	  var creditID string
	   if r.ledger != nil {
		   idemKey := fmt.Sprintf("fiat-add-gate:%s:%d", sess.ID, sess.ConfirmGen)
		  snap, err := r.ledger.GetByIdempotencyKey(ctx, idemKey)
		   if err == nil && snap.ID != "" {
			   ledgerCredited = true
			  creditID = snap.ID
		  }
	   }
	   if ledgerCredited {
		   // ledger 已贷，Session 仍 confirming → 分叉
		  t := now
		   _ = r.leases.Transition(ctx, lease.LeaseID, LeaseIssued, LeaseForked, ConfirmLease{
			  CreditID: creditID, ForkedAt: &t,
			   Note: fmt.Sprintf("recycler: ledger credited but session still confirming; credit_id=%s", creditID),
		  })
		  fc := FiatAddForkCase{
			   CaseID: newCaseID(),
			   SessionID:      sess.ID,
			   ConfirmGen:     sess.ConfirmGen,
			  LeaseID:        lease.LeaseID,
			   LedgerCreditID: creditID,
			   SessionStatus:  sess.Status,
			   Kind: ForkLedgerCreditedSessionPending,
			   State:    ForkCaseOpen,
			   OpenedAt: now,
			  Note:     fmt.Sprintf("recycle: ledger credit=%s found but session confirming (expired lease=%s)", creditID, lease.LeaseID),
		   }
		   _, openErr := r.forks.Open(ctx, fc)
		  if openErr != nil {
			  r.log.Error().Err(openErr).Str("session_id", sess.ID).Msg("fiataddgate: open fork case on recycle")
		  }
		  _, _ = r.book.MarkForked(ctx, sess.ID, sess.ConfirmGen, creditID)
		   r.log.Warn().Str("session_id", sess.ID).Str("credit_id", creditID).
			  Msg("fiataddgate: recycler opened fork case (ledger credited, session confirming)")
		   return nil
	  }
	   // ledger 无贷记：租约已过期，Session 可回滚到 pending
	  // 注意：不直接改 Session（调用 book 的 MarkForked/CommitCredit 不适用），
	  // 此处仅过期租约，保留 Session 在 confirming 状态供下一轮或人工处理。
	  // 要将 Session 推回 pending 须用 book 内部接口；此处记录日志并过期租约。
	  t := now
	  if err := r.leases.Transition(ctx, lease.LeaseID, LeaseIssued, LeaseExpired, ConfirmLease{
		   ExpiredAt: &t,
		   Note:      fmt.Sprintf("recycler: no ledger credit found; lease expired for gen=%d", sess.ConfirmGen),
	   }); err != nil {
		  return fmt.Errorf("recycle: expire lease %s: %w", lease.LeaseID, err)
	   }
	   r.log.Info().Str("session_id", sess.ID).Int64("confirm_gen", sess.ConfirmGen).
		   Msg("fiataddgate: recycler expired stale confirming lease; no ledger credit found")
	   return nil
  }
