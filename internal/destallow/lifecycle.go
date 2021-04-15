package destallow

import (
	"context"
	"account-assets.kucoin.net/funding/internal/destlife"
)

// LifecycleHook destallow 生命周期钩子集合。绑定到 Book 后在 Add/Remove/Require 路径上触发。

type LifecycleHook struct {
	cal  *destlife.Calendar
	lock *destlife.InflightLock
	guard *destlife.BypassGuard
}

// NewLifecycleHook 构造钩子。任一参数为 nil 则对应功能降级为 noop。
   func NewLifecycleHook(cal *destlife.Calendar, lock *destlife.InflightLock, guard *destlife.BypassGuard) *LifecycleHook {
	  return &LifecycleHook{cal: cal, lock: lock, guard: guard}
   }

// OnAdd 在 Book.Add 成功后调用，初始化生命周期记录。
// 失败只记日志，不回滚 Book.Add（生命周期记录可由 Worker 补偿）。

  func (h *LifecycleHook) OnAdd(ctx context.Context, userID, chain, addr, label string, bookGen int64) (entryLifeID string, _ error) {
	   if h == nil || h.cal == nil {
		   return "", nil
	  }
	  e, err := h.cal.InitAdded(ctx, userID, chain, addr, label, bookGen)
	  if err != nil {
		  return "", err
	  }
	  destlife.StateTransitions.WithLabelValues("", string(destlife.StateCooling)).Inc()
	   return e.EntryLifeID, nil
  }

// OnRemove 在 Book.Remove 前调用：
// - 无活跃在途引用：返回 nil，允许接着物理删除。
// - 有活跃在途引用：条目进入 revoking 队列，返回 ErrInflightBlock（调用方终止物理删除）。
   func (h *LifecycleHook) OnRemove(ctx context.Context, entryLifeID, actor, note string, bookGen int64) error {
	  if h == nil || h.lock == nil {
		   return nil
	   }
	   return h.lock.ForceRevokingOnRemove(ctx, entryLifeID, actor, note, bookGen)
  }

// OnRequire 在 Book.Require 通过基础检查后调用（可选强化路径）。
// 仅在 entryLifeID 已知时有效；调用方从 LifecycleBook 的元数据缓存获取。
   func (h *LifecycleHook) OnRequire(ctx context.Context, entryLifeID, userID, chain, addr string, bookGen int64) error {
	   if h == nil || h.cal == nil {
		   return nil
	   }
	   return destlife.LifecycleGate(ctx, h.cal, h.guard, h.lock, entryLifeID, userID, chain, addr, bookGen)
   }

// LifecycleBook 带生命周期钩子的 Book 包装。
// 所有写操作额外触发 destlife 状态机；读操作直接委托 Book。
type LifecycleBook struct {
	*Book
	hook *LifecycleHook
	// entryLifeIDs 内存缓存 (user|chain|addr) → entry_life_id，用于 OnRequire。
	// 环境应从 Calendar.GetByAddr 查询（此处为热路径）。
}

// NewLifecycleBook 构造带钩子的 LifecycleBook。
   func NewLifecycleBook(b *Book, hook *LifecycleHook) *LifecycleBook {
	   return &LifecycleBook{Book: b, hook: hook}
   }

// AddExactLife 带生命周期的精确增一条白名单。
// 成功后异步触发 Calendar.InitAdded。

  func (lb *LifecycleBook) AddExactLife(ctx context.Context, userID, chain, addr, label string) (Entry, string, error) {
	   if lb == nil {
		   return Entry{}, "", ErrMissing
	  }
	   e, err := lb.Book.AddExact(userID, chain, addr, label)
	   if err != nil {
		  return Entry{}, "", err
	   }
	   bookGen := int64(0)
	  entryLifeID := ""
	   if lb.hook != nil {
		   eid, herr := lb.hook.OnAdd(ctx, userID, chain, addr, label, bookGen)
		   if herr == nil {
			   entryLifeID = eid
		  }
	   }
	  return e, entryLifeID, nil
  }

// RemoveExactLife 带生命周期的精确删除。
// 若有活跃在途引用，条目进入 revoking 队列，返回 ErrInflightBlock 并终止物理删除。
  func (lb *LifecycleBook) RemoveExactLife(ctx context.Context, userID, chain, addr, entryLifeID, actor string, bookGen int64) error {
	   if lb == nil {
		  return ErrMissing
	   }
	   if lb.hook != nil {
		  if err := lb.hook.OnRemove(ctx, entryLifeID, actor, "remove_requested", bookGen); err != nil {
			  // ErrInflightBlock：终止物理删除，条目已进入 revoking。
			  return err
		  }
	   }
	  return lb.Book.RemoveExact(userID,
		   chain, addr)
  }

// RequireLife 带生命周期冷却校验的 Require。
// 依次执行：基础 Book.RequireStrict → LifecycleGate（冷却 + 绕过证明 + 世代一致）。
   func (lb *LifecycleBook) RequireLife(ctx context.Context, userID, chain, addr, entryLifeID string, bookGen int64) error {
	   if lb == nil {
		  return ErrMissing
	   }
	   if err := lb.Book.RequireStrict(userID, chain, addr); err != nil {
		  return err
	  }
	  if lb.hook != nil && entryLifeID != "" {
		   return lb.hook.OnRequire(ctx, entryLifeID, userID, chain, addr, bookGen)
	  }
	   return nil
   }
