package ratelimit

// WithdrawGuard 出金尝试节流。默认 3 容量 / 1 每 20 秒，同一 actor 打满即拒。
type WithdrawGuard struct {
	tab *Table
}

  func NewWithdraw(capacity int, refill float64) *WithdrawGuard {
	   if capacity <= 0 {
		   capacity = 3
	   }
	  if refill <= 0 {
		  refill = 0.05
	  }
	   return &WithdrawGuard{tab: NewTable(capacity, refill)}
   }

   func (g *WithdrawGuard) Allow(actor string) error {
	   if g == nil {
		  return ErrUnavailable
	   }
	  key, err := ActorKey(actor, ActionWithdraw)
	  if err != nil {
		   return err
	   }
	   return g.tab.Allow(key)
  }

  func (g *WithdrawGuard) Table() *Table {
	   if g == nil {
		  return NewTable(3, 0.05)
	  }
	  return g.tab
   }
