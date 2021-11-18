package ratelimit

// QueryGuard 公共查询口。默认 30 容量 / 10 每秒，按 actor 隔离。
type QueryGuard struct {
	tab *Table
}

  func NewQuery(capacity int, refill float64) *QueryGuard {
	  if capacity <= 0 {
		   capacity = 30
	  }
	   if refill <= 0 {
		   refill = 10
	   }
	   return &QueryGuard{tab: NewTable(capacity, refill)}
   }

   func (g *QueryGuard) Allow(actor string) error {
	   if g == nil {
		   return ErrUnavailable
	  }
	   key, err := ActorKey(actor, ActionQuery)
	   if err != nil {
		   return err
	   }
	   return g.tab.Allow(key)
  }

   func (g *QueryGuard) Table() *Table {
	  if g == nil {
		  return NewTable(30, 10)
	  }
	   return g.tab
  }
