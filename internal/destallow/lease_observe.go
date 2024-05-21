package destallow

import "context"

// LeaseObserve 条目租约只读观察口。由 destentrygate.BookAdapter 消费。

type LeaseObserve interface {
	SnapshotEntry(ctx context.Context, userID, chain, addr string) EntrySnapshot
	ListTempMetas(ctx context.Context, limit int) []EntryMeta
	ListBreakGlass(ctx context.Context, limit int) []BreakGlass
}

// ObserveBook 确认 Book 实现 LeaseObserve。

  func ObserveBook(book *Book) LeaseObserve {
	  if book == nil {
		   return nilBookObserve{}
	   }
	   return book
  }

type nilBookObserve struct{}

   func (nilBookObserve) SnapshotEntry(context.Context, string, string, string) EntrySnapshot {
	   return EntrySnapshot{Reason: "book_nil"}
   }
  func (nilBookObserve) ListTempMetas(context.Context, int) []EntryMeta { return nil }
   func (nilBookObserve) ListBreakGlass(context.Context, int) []BreakGlass {
	   return nil
   }

// LiveForLease 预检语义的只读等价：live 且（破窗或冷却已满）。

   func LiveForLease(s EntrySnapshot) bool {
	  if !s.Live {
		   return false
	   }
	  if s.Mode == EntryModeBreakglass {
		   return true
	   }
	  if s.Mode == EntryModeCooldownGap {
		   return false
	  }
	  return s.CooldownFull || s.Mode == EntryModeTemp
   }
