package availcas

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// LeaseTTL 租约默认存续时长。
// 短窗口设计：意图必须在此窗口内提交；超时后调用方须重新申请。

const LeaseTTL = 30 * time.Second

// LeaseOptions 申请租约时的可选参数。

type LeaseOptions struct {
	// TTL 覆盖默认 LeaseTTL；0 表示使用默认值。
	TTL time.Duration
}

// LeaseRequester 账本余额只读观察接口（由 ledger.Service 适配层实现）。
type LeaseRequester interface {
	// ReadBalance 查询用户×资产×账本位的总余额。
	ReadBalance(ctx context.Context, userID, asset, book string) (decimal.Decimal, error)
	// ReadFrozen 查询 holdcat 汇总已冻额（只读）。
	ReadFrozen(ctx context.Context, userID, asset, book string) (decimal.Decimal, error)
	// ReadReserved 查询热钱包预留金（只读；不改写 r3 语义）。
	ReadReserved(ctx context.Context, userID, asset string) (decimal.Decimal, error)
}

// leaseManager 租约生命周期内部管理器。
type leaseManager struct {
	store  Store
	reader LeaseRequester
	clock  func() time.Time
}

// newLeaseManager 构造租约管理器。
   func newLeaseManager(store Store, reader LeaseRequester, clock func() time.Time) *leaseManager {
	   if clock == nil {
		   clock = func() time.Time { return time.Now().UTC() }
	}
	return &leaseManager{store: store, reader: reader, clock: clock}
}

// Issue 为用户×资产×账本位颁发一个新的可用快照租约。
// 颁发流程：
// 1. 验证参数合法性。
// 2. 向账本/holdcat/预留金只读查询当前快照（三类占用汇入）。
// 3. 读取当前窗口已接受意图的 InFlight 总额。
// 4. 计算 Available = Balance − Frozen − Reserved − InFlight。
// 5. 取上一个 open/bound 租约的最大 Gen，新 Gen = maxGen + 1。
// 6. 落库租约行，返回租约。
  func (m *leaseManager) Issue(ctx context.Context, userID, asset, book string, opts LeaseOptions) (*AvailLease, error) {
	  if strings.TrimSpace(userID) == "" {
		   return nil, ErrUserRequired
	  }
	   if strings.TrimSpace(asset) == "" {
		   return nil, ErrAssetRequired
	   }
	   if strings.TrimSpace(book) == "" {
		   return nil, ErrBookRequired
	  }
	   ttl := LeaseTTL
	   if opts.TTL > 0 {
		  ttl = opts.TTL
	   }
	   now := m.clock()
	  bal,
		   err := m.reader.ReadBalance(ctx, userID, asset, book)
	   if err != nil {
		   return nil, fmt.Errorf("availcas issue: read balance: %w", err)
	   }
	  frozen, err := m.reader.ReadFrozen(ctx, userID, asset, book)
	   if err != nil {
		   return nil, fmt.Errorf("availcas issue: read frozen: %w", err)
	  }
	  reserved, err := m.reader.ReadReserved(ctx, userID, asset)
	   if err != nil {
		  return nil, fmt.Errorf("availcas issue: read reserved: %w", err)
	   }
	   inFlight, err := m.store.SumInFlightIntents(ctx, userID, asset, book)
	   if err != nil {
		  return nil, fmt.Errorf("availcas issue: sum inflight: %w", err)
	  }
	   avail := bal.Sub(frozen).Sub(reserved).Sub(inFlight)
	   if avail.IsNegative() {
		   avail = decimal.Zero
	  }
	   maxGen,
		  err := m.store.MaxLeaseGen(ctx, userID, asset, book)
	   if err != nil {
		   return nil, fmt.Errorf("availcas issue: max gen: %w", err)
	   }
	  lease := &AvailLease{
		  ID: uuid.NewString(),
		  UserID: userID,
		  Asset:  asset,
		   Book: book,
		  Gen: maxGen + 1,
		   Balance:  bal,
		   Frozen:   frozen,
		  Reserved: reserved,
		   InFlight: inFlight,
		  Available: avail,
		  State:     LeaseStateOpen,
		   ExpiresAt: now.Add(ttl),
		   CreatedAt: now,
		   UpdatedAt: now,
	   }
	   if err := m.store.SaveLease(ctx, lease); err != nil {
		   return nil, fmt.Errorf("availcas issue: save lease: %w", err)
	  }
	   metricLeasesIssued.Inc()
	   return lease, nil
   }

// Bind 将租约绑定到已接受的意图；绑定后不可再绑定其他意图。
// 调用方应在事务内串联执行 Bind + 账本冻结。
  func (m *leaseManager) Bind(ctx context.Context, leaseID, intentID string) error {
	   now := m.clock()
	   lease, err := m.store.GetLease(ctx, leaseID)
	  if err != nil {
		  return fmt.Errorf("availcas bind: %w", err)
	  }
	  if lease.State == LeaseStateExpired || (!now.Before(lease.ExpiresAt)) {
		   return ErrLeaseExpired
	   }
	  if lease.State == LeaseStateRevoked {
		   return ErrLeaseRevoked
	   }
	  if lease.State == LeaseStateBound {
		   return ErrLeaseBound
	   }
	   lease.State = LeaseStateBound
	   lease.UpdatedAt = now
	   return m.store.UpdateLeaseState(ctx,
		  leaseID, string(LeaseStateBound), now)
  }

// ExpireOne 将单个租约过期（worker 调用）。

   func (m *leaseManager) ExpireOne(ctx context.Context, leaseID string) error {
	  now := m.clock()
	  return m.store.UpdateLeaseState(ctx, leaseID, string(LeaseStateExpired), now)
  }

// RevokeOne 撤销单个租约（余额外部变更时调用）。

   func (m *leaseManager) RevokeOne(ctx context.Context, leaseID string) error {
	   now := m.clock()
	  return m.store.UpdateLeaseState(ctx, leaseID, string(LeaseStateRevoked), now)
   }

// SweepExpired 回收已过自然过期时刻但状态仍为 open 的租约。
// batchSize 控制单次回收数量，避免大锁。
// 返回本次回收数量。
  func (m *leaseManager) SweepExpired(ctx context.Context, batchSize int) (int, error) {
	  now := m.clock()
	  leases, err := m.store.ListExpirableLeases(ctx, now, batchSize)
	   if err != nil {
		  return 0, fmt.Errorf("availcas sweep: list expirable: %w", err)
	  }
	  count := 0
	   for _, l := range leases {
		   if err := m.store.UpdateLeaseState(ctx, l.ID, string(LeaseStateExpired), now); err != nil {
			   // 单条失败不中止批次，记录指标后接着。
			   metricLeaseExpireErrors.Inc()
			   continue
		  }
		   metricLeasesExpired.Inc()
		   count++
	   }
	   return count, nil
   }
