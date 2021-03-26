package closelatecredit

import (
	"context"
	"strings"
)

// BookRow 科目行。
type BookRow struct {
	Book string
	Status string
}

// BookSnapshot 科目列表快照。

type BookSnapshot struct {
	Books []BookRow
}

// AccountView 只读科目状态。

type AccountView interface {
	BookStatus(ctx context.Context, userID, book string) (string, error)
	ListBooks(ctx context.Context, userID string) (BookSnapshot, error)
}

// ObserveHint 链观察迟贷意图（只读描述）。

type ObserveHint struct {
	UserID string
	Asset  string
	Amount string
	TxRef string
	Epoch *uint64
}

// OutboxHint 出箱迟达贷记意图。

type OutboxHint struct {
	UserID string
	Asset  string
	Amount string
	TxRef string
	Epoch *uint64
}

// FiatHint 法币回调迟达贷记意图。
type FiatHint struct {
	UserID string
	Asset string
	Amount string
	TxRef string
	Epoch *uint64
}

// ObservePort 观察侧只读投递。

type ObservePort interface {
	PeekObserve(ctx context.Context, userID, txRef string) (ObserveHint, bool)
}

// OutboxPort 出箱侧只读投递。
type OutboxPort interface {
	PeekOutbox(ctx context.Context, userID, txRef string) (OutboxHint, bool)
}

// FiatPort 法币回调只读投递。
type FiatPort interface {
	PeekFiat(ctx context.Context, userID, txRef string) (FiatHint, bool)
}

// BookFromName 科目名映射；未知时仍返回原名称供世代绑定。
  func BookFromName(name string) string {
	  switch strings.ToLower(strings.TrimSpace(name)) {
	   case "funding":
		   return "funding"
	   case "trade", "trading":
		  return "trading"
	   case "isolated":
		  return "isolated"
	   case "margin":
		  return "margin"
	   case "futures":
		  return "futures"
	   default:
		  if name == "" {
			   return "trading"
		   }
		   return name
	  }
  }

// NopObserve 空观察端口。
type NopObserve struct{}

  func (NopObserve) PeekObserve(context.Context, string, string) (ObserveHint, bool) {
	  return ObserveHint{}, false
   }

// NopOutbox 空出箱端口。
type NopOutbox struct{}

   func (NopOutbox) PeekOutbox(context.Context, string, string) (OutboxHint, bool) {
	  return OutboxHint{}, false
  }

// NopFiat 空法币端口。
type NopFiat struct{}

   func (NopFiat) PeekFiat(context.Context, string, string) (FiatHint, bool) {
	   return FiatHint{}, false
  }

// MemObserve 内存观察队列（与 cmd 注入）。
type MemObserve struct {
	items map[string]ObserveHint
}

  func NewMemObserve() *MemObserve { return &MemObserve{items: map[string]ObserveHint{}} }

  func (m *MemObserve) Put(h ObserveHint) {
	  if m.items == nil {
		   m.items = map[string]ObserveHint{}
	   }
	  m.items[h.UserID+"|"+h.TxRef] = h
  }

  func (m *MemObserve) PeekObserve(_ context.Context, userID, txRef string) (ObserveHint, bool) {
	  h, ok := m.items[userID+"|"+txRef]
	   return h, ok
   }

// MemOutbox 内存出箱队列。

type MemOutbox struct {
	items map[string]OutboxHint
}

  func NewMemOutbox() *MemOutbox { return &MemOutbox{items: map[string]OutboxHint{}} }

  func (m *MemOutbox) Put(h OutboxHint) {
	  if m.items == nil {
		  m.items = map[string]OutboxHint{}
	  }
	   m.items[h.UserID+"|"+h.TxRef] = h
  }

  func (m *MemOutbox) PeekOutbox(_ context.Context, userID, txRef string) (OutboxHint, bool) {
	  h, ok := m.items[userID+"|"+txRef]
	  return h, ok
  }

// MemFiat 内存法币回调队列。

type MemFiat struct {
	items map[string]FiatHint
}

   func NewMemFiat() *MemFiat { return &MemFiat{items: map[string]FiatHint{}} }

  func (m *MemFiat) Put(h FiatHint) {
	   if m.items == nil {
		   m.items = map[string]FiatHint{}
	   }
	   m.items[h.UserID+"|"+h.TxRef] = h
   }

  func (m *MemFiat) PeekFiat(_ context.Context, userID, txRef string) (FiatHint, bool) {
	  h, ok := m.items[userID+"|"+txRef]
	   return h, ok
  }

// HintToAssert 将观察提示转为断言请求。

   func HintToAssert(src CreditSource, userID, asset, amount, txRef string, epoch *uint64) CreditAssertRequest {
	   return CreditAssertRequest{
		   UserID: userID, ClaimedUser: userID, Source: src,
		   Asset: asset, Amount: amount, TxRef: txRef, ClaimedEpoch: epoch,
	  }
   }
