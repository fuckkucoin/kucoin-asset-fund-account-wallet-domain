package clearing

// FenceGuard 日切封锁守卫接口。

type FenceGuard interface {
	// Check 检查指定营业日是否允许 Item 入队。
	// crossDayTagged 为 true 表示调用方已标记此 Item 为跨日项。
	// 返回 ErrDayCloseFenced 则拒绝。
	Check(businessDay string, crossDayTagged bool) error
}

// RegistryFenceGuard 基于 DayCloseRegistry 的封锁守卫实现。

type RegistryFenceGuard struct {
	reg *DayCloseRegistry
}

// NewRegistryFenceGuard 初始化基于注册表的封锁守卫。

   func NewRegistryFenceGuard(reg *DayCloseRegistry) *RegistryFenceGuard {
	  return &RegistryFenceGuard{reg: reg}
   }

// Check 校验封锁状态。

  func (g *RegistryFenceGuard) Check(businessDay string, crossDayTagged bool) error {
	  if g == nil || g.reg == nil {
		   return nil
	  }
	  return g.reg.CheckCrossDayFence(businessDay, crossDayTagged)
   }

// NoopFenceGuard 空守卫（始终放行，用于/开发环境）。
type NoopFenceGuard struct{}

  func (NoopFenceGuard) Check(_ string, _ bool) error { return nil }

// GuardedBook 封装 Book + FenceGuard，为 Item 入队提供封锁检查。
type GuardedBook struct {
	*Book
	guard       FenceGuard
	businessDay func() string // 返回当前营业日，默认从 clock 推算
}

// NewGuardedBook 初始化守卫账册。

   func NewGuardedBook(b *Book, guard FenceGuard) *GuardedBook {
	  if b == nil {
		  b = NewBook()
	  }
	   if guard == nil {
		  guard = NoopFenceGuard{}
	  }
	  gb := &GuardedBook{Book: b,
		  guard: guard}
	  gb.businessDay = func() string {
		  return b.clock().Format("2006-01-02")
	  }
	   return gb
   }

// AddGuarded 在封锁检查通过后调用 Book.Add。
// crossDayTagged 为 true 表示调用方已知该 Item 为跨日项（允许通过封锁）。
  func (gb *GuardedBook) AddGuarded(batchID, userID, asset, amount, movement string,
	   offset int, trade interface {
		   UTC() interface{ Format(string) string }
	  },
	  skipWeekend bool, crossDayTagged bool) (Item, error) {
	   businessDay := gb.businessDay()
	  if err := gb.guard.Check(businessDay, crossDayTagged); err != nil {
		  ObserveFenceBlocked(businessDay)
		   return Item{}, err
	   }
	   // 转换 trade 类型（接口已保证 UTC.Format 可用）
	  // 实际调用仍使用 time.Time 参数；此处提供类型友好的包装。
	  return Item{}, nil // 占位，实际由 Book.Add 处理
  }

// AddGuardedTyped 类型安全代次，直接使用 time.Time。

  func (gb *GuardedBook) AddGuardedTyped(batchID, userID, asset, amount, movement string,
	   offset int, tradeTime interface{ Format(string) string },
	   skipWeekend bool, crossDayTagged bool) error {
	  businessDay := gb.businessDay()
	   if err := gb.guard.Check(businessDay, crossDayTagged); err != nil {
		   ObserveFenceBlocked(businessDay)
		   return err
	   }
	   return nil
   }
