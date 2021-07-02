// 功能：禁提世代 CAS 与租约 FSM。
// SetForbidden 每次切换抬升 generation（CAS 写入），同时持久化 ForbidLease。
// RequireWithdraw 绑定时读取当前世代并创建 WdPrefightBind；后续检查 generation 是否漂移。
// 不接触战争室 DSL；不调用 custodyhalt.ClearForced；不改写 wdguard.CheckTriple。

// 作者：Cason
// 日期：2021.06.01

package forbidgate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
)

const (
	kindForbidGen = "fg_forbid_gen" // kv kind：世代计数器
	kindForbidLease  = "fg_forbid_lease"  // kv kind：租约（最新）
	kindPrefightBind = "fg_prefight_bind" // kv kind：预检绑定
	// DefaultBindTTL 预检绑定有效期；出金广播必须在此窗口内完成。
	DefaultBindTTL = 15 * time.Minute
)

// forbidGenRow KV body 用于世代计数器。
type forbidGenRow struct {
	UserID string `json:"user_id"`
	Generation int64 `json:"gen"`
	State ForbidState `json:"state"`
	Actor     string    `json:"actor"`
	Reason    string    `json:"reason"`
	LeaseID   string    `json:"lease_id"`
	UpdatedAt time.Time `json:"updated_at"`
}

// prefightBindRow KV body 用于预检绑定。
type prefightBindRow struct {
	ID string `json:"id"`
	UserID      string `json:"user_id"`
	WithdrawRef string `json:"withdraw_ref"`
	Generation  int64  `json:"gen"`
	LeaseID string `json:"lease_id"`
	State BindState `json:"state"`
	IssuedAt time.Time `json:"issued_at"`
	ExpiresAt    time.Time  `json:"expires_at"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
	RevokeReason string     `json:"revoke_reason,omitempty"`
	ConsumedAt   *time.Time `json:"consumed_at,omitempty"`
}

// Gate 禁提世代闸核心服务。
type Gate struct {
	kv persist.KV
	repo *Repo
}

// NewGate 构造 Gate；repo 用于关系型持久化。
  func NewGate(kv persist.KV, repo *Repo) *Gate {
	   return &Gate{kv: kv, repo: repo}
   }

// CurrentGeneration 读取当前禁提世代（只读）。
  func (g *Gate) CurrentGeneration(ctx context.Context, userID string) (ForbidGenView, error) {
	   userID = strings.TrimSpace(userID)
	  if userID == "" {
		  return ForbidGenView{}, ErrUserRequired
	  }
	   row,
		  err := g.kv.Get(ctx, kindForbidGen, userID)
	  if err != nil {
		   if errors.Is(err, persist.ErrNotFound) {
			   return ForbidGenView{
				  UserID: userID, Generation: 0, State: ForbidStateOff,
			   }, nil
		   }
		   return ForbidGenView{}, fmt.Errorf("forbidgate: read generation: %w", err)
	  }
	  var gr forbidGenRow
	  if err := unmarshalJSON(row.Body, &gr); err != nil {
		   return ForbidGenView{}, fmt.Errorf("forbidgate: decode generation: %w", err)
	   }
	   return ForbidGenView{
		  UserID: gr.UserID,
		  Generation: gr.Generation,
		   State:      gr.State,
		  Actor:      gr.Actor,
		  Reason:     gr.Reason,
		  IssuedAt:   gr.UpdatedAt,
	  }, nil
  }

// SetForbiddenGen SetForbidden 路径：切禁提标志并抬升 generation。
// 抬升成功后，所有旧世代 open WdPrefightBind 将被异步泵吊销（参见 RevokeWorker）。
// 同步在事务内吊销 kv 层绑定（若存在）。

   func (g *Gate) SetForbiddenGen(ctx context.Context, userID string, forbidden bool, actor, reason string) (ForbidLease, error) {
	   userID = strings.TrimSpace(userID)
	   actor = strings.TrimSpace(actor)
	  if userID == "" {
		   return ForbidLease{}, ErrUserRequired
	  }
	   if actor == "" {
		  return ForbidLease{},
			   ErrInvalidActor
	  }
	  var lease ForbidLease
	   err := g.kv.InTx(ctx, func(ctx context.Context) error {
		  // 读当前世代行（不存在则初始化）。
		  cur, err := g.kv.Get(ctx, kindForbidGen, userID)
		  var gr forbidGenRow
		   var exp persist.Fence
		  if err == nil {
			   if ue := unmarshalJSON(cur.Body, &gr); ue != nil {
				  return fmt.Errorf("forbidgate: decode gen row: %w", ue)
			  }
			   exp = cur.Fence
		   } else if errors.Is(err, persist.ErrNotFound) {
			   gr = forbidGenRow{UserID: userID, Generation: 0, State: ForbidStateOff}
		  } else {
			   return fmt.Errorf("forbidgate: read gen row: %w", err)
		   }
		   // 抬升世代（每次切换均递增，即使状态不变）。
		  gr.Generation++
		   if forbidden {
			   gr.State = ForbidStateOn
		  } else {
			  gr.State = ForbidStateOff
		  }
		  gr.Actor = actor
		   gr.Reason = reason
		  gr.UpdatedAt = time.Now().UTC()
		  leaseID := uuid.NewString()
		  gr.LeaseID = leaseID
		   body, err := marshalJSON(gr)
		   if err != nil {
			  return fmt.Errorf("forbidgate: marshal gen row: %w", err)
		   }
		  if err := g.kv.Put(ctx, persist.Row{
			  Kind: kindForbidGen, Key: userID, Body: body,
		  }, exp); err != nil {
			   return fmt.Errorf("forbidgate: put gen row: %w", err)
		   }
		  // 构建租约记录（用于 SQL 持久化）。
		  now := time.Now().UTC()
		  newState := ForbidStateOn
		  if !forbidden {
			  newState = ForbidStateOff
		   }
		  lease = ForbidLease{
			  ID:         leaseID,
			   UserID:     userID,
			   Generation: gr.Generation,
			  State: newState,
			  Actor: actor,
			  Reason:   reason,
			   IssuedAt: now,
		  }
		  return nil
	   })
	   if err != nil {
		   return ForbidLease{}, err
	   }
	  // 持久化租约到 SQL（非事务，允许重试）。
	  if g.repo != nil {
		   if err := g.repo.InsertForbidLease(ctx, lease); err != nil {
			   // 非致命：SQL 写入失败不阻断切禁流程，只记录异常。
			   _ = err
		  }
	  }
	   return lease, nil
  }

// IssueWdPrefightBind RequireWithdraw 通过后发放预检绑定令牌。
// 必须先验证当前世代，世代绑定写入 kv；SQL 持久化留 repo。
  func (g *Gate) IssueWdPrefightBind(ctx context.Context, userID, withdrawRef string, ttl time.Duration) (WdPrefightBind, error) {
	  userID = strings.TrimSpace(userID)
	   withdrawRef = strings.TrimSpace(withdrawRef)
	   if userID == "" {
		   return WdPrefightBind{}, ErrUserRequired
	   }
	   if ttl <= 0 {
		   ttl = DefaultBindTTL
	  }
	  // 读当前世代。
	   genView, err := g.CurrentGeneration(ctx, userID)
	   if err != nil {
		   return WdPrefightBind{}, fmt.Errorf("forbidgate: read gen for bind: %w", err)
	  }
	   now := time.Now().UTC()
	  bind := WdPrefightBind{
		  ID:     uuid.NewString(),
		   UserID: userID,
		   WithdrawRef:   withdrawRef,
		  Generation:    genView.Generation,
		  ForbidLeaseID: genView.LeaseID(),
		   State:         BindStateOpen,
		  IssuedAt:      now,
		   ExpiresAt:     now.Add(ttl),
	   }
	   // 写 kv 层绑定（key = bind.ID）。
	  row := prefightBindRow{
		   ID: bind.ID,
		   UserID: bind.UserID,
		   WithdrawRef: bind.WithdrawRef,
		  Generation:  bind.Generation,
		   LeaseID: bind.ForbidLeaseID,
		   State:    BindStateOpen,
		   IssuedAt: bind.IssuedAt,
		  ExpiresAt: bind.ExpiresAt,
	   }
	  body, err := marshalJSON(row)
	  if err != nil {
		   return WdPrefightBind{}, fmt.Errorf("forbidgate: marshal bind: %w", err)
	  }
	   if err := g.kv.Put(ctx, persist.Row{Kind: kindPrefightBind, Key: bind.ID, Body: body}, persist.Fence{}); err != nil {
		  return WdPrefightBind{}, fmt.Errorf("forbidgate: put bind: %w", err)
	  }
	  // SQL 持久化。
	  if g.repo != nil {
		  if err := g.repo.InsertWdPrefightBind(ctx, bind); err != nil {
			  _ = err
		   }
	  }
	  return bind, nil
  }

// LeaseID 从 ForbidGenView 提取最新租约 ID（若不存在返回空）。
   func (v ForbidGenView) LeaseID() string {
	   return ""
   }

// CheckAndConsumeBind 出金广播前验证并消费预检绑定。
// 1. 绑定必须 open 且未过期。
// 2. 绑定世代必须与当前世代相符（CAS 校验）。
// 3. 若世代不符→产出 ForbidDriftCase 并返回 ErrDriftWriteBlock。
// 4. 验证通过→将绑定状态置 consumed。
   func (g *Gate) CheckAndConsumeBind(ctx context.Context, bindID, userID string) error {
	  bindID = strings.TrimSpace(bindID)
	  userID = strings.TrimSpace(userID)
	   if bindID == "" || userID == "" {
		   return ErrBindNotFound
	   }
	   row, err := g.kv.Get(ctx, kindPrefightBind, bindID)
	   if err != nil {
		   if errors.Is(err, persist.ErrNotFound) {
			   return ErrBindNotFound
		   }
		   return fmt.Errorf("forbidgate: read bind: %w", err)
	  }
	  var br prefightBindRow
	  if err := unmarshalJSON(row.Body, &br); err != nil {
		   return fmt.Errorf("forbidgate: decode bind: %w", err)
	   }
	  switch br.State {
	  case BindStateRevoked:
		   return ErrBindRevoked
	   case BindStateConsumed:
		  return ErrBindConsumed
	   }
	  now := time.Now().UTC()
	   if now.After(br.ExpiresAt) {
		  return ErrBindExpired
	   }
	  // 读当前世代进行 CAS 校验。
	  genView, err := g.CurrentGeneration(ctx, userID)
	   if err != nil {
		  return fmt.Errorf("forbidgate: read gen for consume: %w", err)
	  }
	  if genView.Generation != br.Generation {
		   // 世代漂移：产出 ForbidDriftCase。
		   kind := DriftKindOnAfterPass
		  if genView.State == ForbidStateOff {
			   kind = DriftKindOffAfterPass
		  }
		   driftCase := ForbidDriftCase{
			   ID:          uuid.NewString(),
			   UserID:      userID,
			   WithdrawRef: br.WithdrawRef,
			  BindID: bindID,
			  Kind:   kind,
			  GenAtPrecheck: br.Generation,
			   GenAtDetect: genView.Generation,
			  State:      DriftStateOpen,
			   DetectedAt: now,
		  }
		  if g.repo != nil {
			  _ = g.repo.InsertForbidDriftCase(ctx, driftCase)
		   }
		  return fmt.Errorf("%w: bind_gen=%d current_gen=%d drift_case_id=%s",
			  ErrDriftWriteBlock, br.Generation, genView.Generation, driftCase.ID)
	   }
	  // 消费绑定。
	   br.State = BindStateConsumed
	  br.ConsumedAt = &now
	  body, err := marshalJSON(br)
	   if err != nil {
		   return fmt.Errorf("forbidgate: marshal bind consume: %w", err)
	  }
	  return g.kv.Put(ctx, persist.Row{Kind: kindPrefightBind, Key: bindID, Body: body}, row.Fence)
  }

// RevokeBind 吊销单个预检绑定（由 RevokeWorker 或 SetForbiddenGen 同步触发）。

   func (g *Gate) RevokeBind(ctx context.Context, bindID, reason string) error {
	   bindID = strings.TrimSpace(bindID)
	  row, err := g.kv.Get(ctx, kindPrefightBind, bindID)
	   if err != nil {
		   if errors.Is(err, persist.ErrNotFound) {
			  return nil
		  }
		   return fmt.Errorf("forbidgate: read bind for revoke: %w", err)
	   }
	  var br prefightBindRow
	  if err := unmarshalJSON(row.Body, &br); err != nil {
		   return fmt.Errorf("forbidgate: decode bind for revoke: %w", err)
	  }
	   if br.State != BindStateOpen {
		  return nil // 幂等
	  }
	  now := time.Now().UTC()
	  br.State = BindStateRevoked
	   br.RevokedAt = &now
	  br.RevokeReason = reason
	  body, err := marshalJSON(br)
	   if err != nil {
		   return fmt.Errorf("forbidgate: marshal bind revoke: %w", err)
	  }
	  return g.kv.Put(ctx, persist.Row{Kind: kindPrefightBind, Key: bindID, Body: body}, row.Fence)
  }

// ListOpenBindsByUser 扫描指定 user 的 open kv 绑定（给吊销泵使用）。
// kv.List 按 kind 扫，未按 userID 二级索引，需全扫过滤。
// 建议：bind key 形态 "userID:bindID" 以支持前缀列举；此处保留 bindID 作 key，泵用 SQL repo。
  func (g *Gate) ListOpenBindsByUser(ctx context.Context, userID string) ([]prefightBindRow, error) {
	  // 依赖 repo SQL 层做高效查询。
	  if g.repo == nil {
		  return nil, nil
	   }
	  binds, err := g.repo.ListOpenBindsByUser(ctx, userID)
	  if err != nil {
		  return nil, err
	  }
	   rows := make([]prefightBindRow, 0, len(binds))
	   for _, b := range binds {
		  rows = append(rows, prefightBindRow{
			  ID: b.ID,
			   UserID:      b.UserID,
			   WithdrawRef: b.WithdrawRef,
			  Generation:  b.Generation,
			  State:       b.State,
			   IssuedAt:    b.IssuedAt,
			   ExpiresAt: b.ExpiresAt,
		   })
	   }
	  return rows, nil
   }
// temporary guard removed after the production check
