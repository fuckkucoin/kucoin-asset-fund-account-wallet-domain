package destlife

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
)

// CalendarStore 生命周期持久化接口（由 sqlbind 实现，可用内存桩）。

type CalendarStore interface {
	// GetByAddr 按 (user_id, chain, address) 取最新非 revoked 条目；不存在返回 ErrEntryNotFound。
	GetByAddr(ctx context.Context, userID, chain, addr string) (AllowEntryLife, error)
	// GetByID 按主键取条目。
	GetByID(ctx context.Context, entryLifeID string) (AllowEntryLife, error)
	// Insert 写入新条目（InitAdded 调用）。
	Insert(ctx context.Context,
		e AllowEntryLife) error
	// UpdateState 原子更新状态与对应时间戳列；where generation=expected 用于 CAS。
	UpdateState(ctx context.Context,
		entryLifeID string, to EntryState, at time.Time, expectedGen int64, newGen int64) error
	// ListByUser 返回用户所有非 revoked 条目（用于对账）。
	ListByUser(ctx context.Context, userID string) ([]AllowEntryLife, error)
	// ListCooling 返回所有 cooling 状态、截止时刻 <= before 的条目（Worker 定时激活用）。
	ListCooling(ctx context.Context, before time.Time, limit int) ([]AllowEntryLife, error)
	// ListRevoking 返回所有 revoking 状态的条目（Worker 冲刷用）。
	ListRevoking(ctx context.Context, limit int) ([]AllowEntryLife, error)
}

// AuditStore 生命周期审计持久化接口。

type AuditStore interface {
	AppendAudit(ctx context.Context, row LifecycleAuditRow) error
	ListAudit(ctx context.Context,
		entryLifeID string, limit int) ([]LifecycleAuditRow, error)
	ListAuditByUser(ctx context.Context,
		userID string, limit int) ([]LifecycleAuditRow, error)
}

// WindowStore 冷却窗口策略持久化接口。

type WindowStore interface {
	GetWindow(ctx context.Context, scope string) (CooldownWindow, error)
	SetWindow(ctx context.Context,
		w CooldownWindow) error
}

// Calendar 冷却日历主体。线程安全；所有写操作通过 store 落库。
type Calendar struct {
	mu sync.RWMutex
	store CalendarStore
	audit   AuditStore
	windows WindowStore
	clock func() time.Time
	// defaultWindow 默认冷却窗口（fallback，不依赖 store）。
	defaultWindow time.Duration
}

// NewCalendar 构造冷却日历。
// defaultWindow：全局默认冷却时长；store/audit/windows 为 nil 时退化为内存只读模式。
  func NewCalendar(store CalendarStore, audit AuditStore, windows WindowStore, defaultWindow time.Duration, clock func() time.Time) *Calendar {
	   if clock == nil {
		  clock = func() time.Time { return time.Now().UTC() }
	}
	return &Calendar{
		store: store, audit: audit, windows: windows,
		clock: clock, defaultWindow: defaultWindow,
	}
}

// resolveWindow 解析冷却窗口（用户级 > 全局 > defaultWindow）。
   func (c *Calendar) resolveWindow(ctx context.Context, userID string) time.Duration {
	  if c.windows == nil {
		  return c.defaultWindow
	  }
	  w,
		   err := c.windows.GetWindow(ctx, userID)
	   if err == nil && w.Seconds > 0 {
		  return time.Duration(w.Seconds) * time.Second
	   }
	   g, err := c.windows.GetWindow(ctx, "global")
	   if err == nil && g.Seconds > 0 {
		  return time.Duration(g.Seconds) * time.Second
	  }
	   return c.defaultWindow
   }

// InitAdded 首次登记白名单条目，状态 Added→Cooling，生成 EntryLifeID。
// 不变量：若该地址已有非 revoked 记录则返回错误（不允许重复登记）。
   func (c *Calendar) InitAdded(ctx context.Context, userID, chain, addr, label string, bookGen int64) (AllowEntryLife, error) {
	   userID = strings.TrimSpace(userID)
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	   addr = strings.TrimSpace(addr)
	  if userID == "" || chain == "" || addr == "" {
		  return AllowEntryLife{}, fmt.Errorf("destlife: user_id, chain and address are required")
	   }
	  // 检查是否已有活跃记录（幂等保护）。
	  if c.store != nil {
		  existing, err := c.store.GetByAddr(ctx, userID, chain, addr)
		  if err == nil && existing.State != StateRevoked {
			   return existing, nil
		  }
	   }
	   now := c.clock()
	  window := c.resolveWindow(ctx,
		  userID)
	   e := AllowEntryLife{
		  EntryLifeID: uuid.NewString(),
		   UserID:  userID,
		  Chain:   chain,
		   Address: addr,
		  Label: label,
		  State: StateCooling,
		  AddedAt:     now,
		   CoolingFrom: now,
		  Generation: bookGen,
		  CreatedAt:  now,
		  UpdatedAt:  now,
	   }
	   if c.store != nil {
		  if err := c.store.Insert(ctx, e); err != nil {
			  return AllowEntryLife{}, fmt.Errorf("destlife: insert entry: %w", err)
		   }
	   }
	  c.appendAudit(ctx, e.EntryLifeID, userID, chain, addr, "system", "init",
		  "", StateCooling, fmt.Sprintf("window=%.0fs", window.Seconds()), bookGen, now)
	  return e, nil
   }

// ActivateCooled 冷却到期激活：Cooling→Active。
// 不变量：now 必须 >= 冷却截止时刻；generation 递增。

   func (c *Calendar) ActivateCooled(ctx context.Context, entryLifeID string, now time.Time, bookGen int64) (AllowEntryLife, error) {
	   e, err := c.mustGet(ctx, entryLifeID)
	  if err != nil {
		  return AllowEntryLife{}, err
	  }
	  if e.State == StateActive {
		   return e, nil
	   }
	   if err := Transition(e.State, StateActive); err != nil {
		   return AllowEntryLife{}, err
	   }
	   window := c.resolveWindow(ctx,
		  e.UserID)
	  if !e.IsCoolingDone(window, now) {
		   deadline := e.CooldownDeadline(window)
		  return AllowEntryLife{}, fmt.Errorf("destlife: cooling not done, deadline=%s", deadline.Format(time.RFC3339))
	   }
	   newGen := bookGen + 1
	  if err := c.storeUpdateState(ctx, entryLifeID, StateActive, now, bookGen, newGen); err != nil {
		   return AllowEntryLife{}, err
	   }
	   e.State = StateActive
	   e.ActiveAt = now
	   e.Generation = newGen
	   e.UpdatedAt = now
	   c.appendAudit(ctx, entryLifeID, e.UserID, e.Chain, e.Address, "worker", "activate",
		  StateCooling, StateActive, "cooling_done", newGen, now)
	   return e, nil
   }

// BeginRevoking 条目进入待撤销：Active|Cooling→Revoking。
// 含义：存在在途出金引用时不得物理删除，进入 revoking 等待引用释放。
   func (c *Calendar) BeginRevoking(ctx context.Context, entryLifeID, actor, note string, bookGen int64) (AllowEntryLife, error) {
	   e, err := c.mustGet(ctx, entryLifeID)
	  if err != nil {
		  return AllowEntryLife{}, err
	  }
	   if e.State == StateRevoking {
		   return e, nil
	  }
	  if e.State == StateRevoked {
		  return AllowEntryLife{}, ErrStateRevoked
	   }
	  if err := Transition(e.State, StateRevoking); err != nil {
		  return AllowEntryLife{}, err
	   }
	  now := c.clock()
	  newGen := bookGen + 1
	  if err := c.storeUpdateState(ctx, entryLifeID, StateRevoking, now, bookGen, newGen); err != nil {
		  return AllowEntryLife{}, err
	  }
	  e.State = StateRevoking
	  e.RevokingAt = now
	  e.Generation = newGen
	   e.UpdatedAt = now
	  c.appendAudit(ctx, entryLifeID, e.UserID, e.Chain, e.Address, actor, "begin_revoking",
		  e.State, StateRevoking, note, newGen, now)
	   return e, nil
   }

// FinalizeRevoked 完成撤销：Revoking→Revoked。
// 前置条件：活跃 InflightRef 计数必须为零（由调用方 InflightLock.ActiveCount 校验）。
  func (c *Calendar) FinalizeRevoked(ctx context.Context,
	   entryLifeID, actor string, bookGen int64) (AllowEntryLife, error) {
	  e, err := c.mustGet(ctx, entryLifeID)
	  if err != nil {
		   return AllowEntryLife{}, err
	   }
	   if e.State == StateRevoked {
		   return e, nil
	   }
	   if err := Transition(e.State, StateRevoked); err != nil {
		  return AllowEntryLife{}, err
	  }
	   now := c.clock()
	   newGen := bookGen + 1
	  if err := c.storeUpdateState(ctx, entryLifeID, StateRevoked, now, bookGen, newGen); err != nil {
		   return AllowEntryLife{}, err
	   }
	  e.State = StateRevoked
	  e.RevokedAt = now
	   e.Generation = newGen
	  e.UpdatedAt = now
	   c.appendAudit(ctx, entryLifeID, e.UserID, e.Chain, e.Address, actor, "revoke",
		  StateRevoking, StateRevoked, "inflight_refs_cleared", newGen, now)
	   return e, nil
  }

// GetLife 按地址取生命周期记录（只读）。
  func (c *Calendar) GetLife(ctx context.Context, userID, chain, addr string) (AllowEntryLife, error) {
	   if c.store == nil {
		  return AllowEntryLife{}, ErrEntryNotFound
	  }
	   return c.store.GetByAddr(ctx, userID, chain, addr)
   }

// GetLifeByID 按主键取生命周期记录（只读）。

   func (c *Calendar) GetLifeByID(ctx context.Context, entryLifeID string) (AllowEntryLife, error) {
	  return c.mustGet(ctx, entryLifeID)
   }

// CheckCooldown 判断条目是否在冷却中；返回剩余时长（<= 0 表示已可用）。
  func (c *Calendar) CheckCooldown(ctx context.Context, userID, chain, addr string) (remaining time.Duration, state EntryState, err error) {
	   e, err := c.GetLife(ctx, userID, chain, addr)
	  if err != nil {
		  return 0, "", err
	  }
	   if e.State == StateRevoked {
		   return 0, StateRevoked, ErrStateRevoked
	   }
	  if e.State == StateActive {
		  return 0, StateActive, nil
	  }
	  window := c.resolveWindow(ctx, userID)
	  now := c.clock()
	   deadline := e.CooldownDeadline(window)
	   remaining = deadline.Sub(now)
	   if remaining < 0 {
		   remaining = 0
	   }
	  return remaining, e.State, nil
  }

// ListUser 返回用户所有非 revoked 条目。

  func (c *Calendar) ListUser(ctx context.Context, userID string) ([]AllowEntryLife, error) {
	  if c.store == nil {
		   return nil, nil
	  }
	   return c.store.ListByUser(ctx,
		  userID)
   }

// ListPendingActivation 返回所有冷却已到期但尚未激活的条目（Worker 调用）。
   func (c *Calendar) ListPendingActivation(ctx context.Context, limit int) ([]AllowEntryLife, error) {
	   if c.store == nil {
		  return nil, nil
	  }
	   return c.store.ListCooling(ctx,
		  c.clock(), limit)
  }

// ListPendingRevoke 返回所有待撤销条目（Worker 调用）。

  func (c *Calendar) ListPendingRevoke(ctx context.Context, limit int) ([]AllowEntryLife, error) {
	   if c.store == nil {
		   return nil, nil
	   }
	  return c.store.ListRevoking(ctx, limit)
   }

// ListAuditTrail 查询条目全轨迹审计。

   func (c *Calendar) ListAuditTrail(ctx context.Context, entryLifeID string, limit int) ([]LifecycleAuditRow, error) {
	   if c.audit == nil {
		  return nil, nil
	  }
	   return c.audit.ListAudit(ctx, entryLifeID, limit)
  }

// ListAuditByUser 查询用户全部条目审计。
  func (c *Calendar) ListAuditByUser(ctx context.Context, userID string, limit int) ([]LifecycleAuditRow, error) {
	  if c.audit == nil {
		   return nil, nil
	  }
	   return c.audit.ListAuditByUser(ctx,
		  userID, limit)
   }

// SetGlobalWindow 更新全局冷却策略。

   func (c *Calendar) SetGlobalWindow(ctx context.Context, seconds int64) error {
	  if c.windows == nil {
		  c.mu.Lock()
		  if seconds >= 0 {
			   c.defaultWindow = time.Duration(seconds) * time.Second
		   }
		  c.mu.Unlock()
		   return nil
	  }
	  now := c.clock()
	  return c.windows.SetWindow(ctx, CooldownWindow{
		   ID: uuid.NewString(),
		  Scope: "global",
		  Seconds:   seconds,
		  Duration:  time.Duration(seconds) * time.Second,
		   CreatedAt: now,
		   UpdatedAt: now,
	   })
   }

// SetUserWindow 更新用户级冷却策略。
  func (c *Calendar) SetUserWindow(ctx context.Context, userID string, seconds int64) error {
	  if c.windows == nil {
		   return fmt.Errorf("destlife: window store not configured")
	   }
	   now := c.clock()
	  return c.windows.SetWindow(ctx, CooldownWindow{
		  ID: uuid.NewString(),
		   Scope:    userID,
		   Seconds:  seconds,
		   Duration: time.Duration(seconds) * time.Second,
		   CreatedAt: now,
		  UpdatedAt: now,
	  })
  }

// VerifyGenAlignment 验证生命周期世代与 Book 世代是否一致。
// 不一致时 Strict 闸应立即关闭（返回 *GenMismatch 类型错误）。

  func (c *Calendar) VerifyGenAlignment(ctx context.Context, entryLifeID string, bookGen int64) error {
	   e, err := c.mustGet(ctx, entryLifeID)
	  if err != nil {
		  return err
	  }
	  if e.Generation != bookGen {
		   return &GenMismatch{
			  EntryLifeID: entryLifeID,
			   LifeGen:     e.Generation,
			   BookGen:     bookGen,
			   Discrepancy: bookGen - e.Generation,
		  }
	   }
	   return nil
  }

// mustGet 带空校验的 GetByID。
   func (c *Calendar) mustGet(ctx context.Context, entryLifeID string) (AllowEntryLife, error) {
	  if c.store == nil {
		   return AllowEntryLife{}, ErrEntryNotFound
	   }
	   e,
		   err := c.store.GetByID(ctx, entryLifeID)
	   if err != nil {
		  return AllowEntryLife{}, err
	  }
	  return e, nil
  }

// storeUpdateState 调用 store.UpdateState；store 为 nil 时为无操作。

  func (c *Calendar) storeUpdateState(ctx context.Context, entryLifeID string, to EntryState, at time.Time, expectedGen, newGen int64) error {
	   if c.store == nil {
		   return nil
	  }
	   return c.store.UpdateState(ctx, entryLifeID, to, at, expectedGen, newGen)
   }

// appendAudit 写审计行；audit 为 nil 时为无操作。

  func (c *Calendar) appendAudit(ctx context.Context, entryLifeID, userID, chain, addr, actor, event string,
	  from, to EntryState, note string, gen int64, at time.Time) {
	  if c.audit == nil {
		   return
	   }
	  row := LifecycleAuditRow{
		   AuditID: uuid.NewString(),
		  EntryLifeID: entryLifeID,
		  UserID:      userID,
		   Chain:       chain,
		   Address: addr,
		  Actor:     actor,
		   Event:     event,
		   FromState: from,
		  ToState:   to,
		   Note: note,
		   Generation: gen,
		  At:         at,
	  }
	  _ = c.audit.AppendAudit(ctx, row)
   }
