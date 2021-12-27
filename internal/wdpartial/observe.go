package wdpartial

// OccupySnapshot 单笔 item 的占用状态快照。

type OccupySnapshot struct {
	UserID         string
	Asset          string
	Amount         string
	FreezeOccupied bool   // 是否存在冻结占用
	LedgerOccupied bool   // 是否有账本侧待处理记录（未到终态）
	LedgerID       string // 账本记录 ID（若 LedgerOccupied 则填写）
}

// OccupyObserver 只读占用观察器接口。
// 具体实现由上层绑定（环境挂接业务冻结/账本查询；环境使用 MockObserver）。
// 本包不做任何写操作，严格只读消费。

type OccupyObserver interface {
	// ObserveFreeze 查询指定用户/资产/账本 ID 是否存在冻结占用。
	// 若 ledgerID 为空，则按 (userID, asset) 做宽松查询。
	ObserveFreeze(userID, asset, ledgerID string) (frozen bool, err error)
	// ObserveLedger 查询账本记录是否存在且未到终态。
	// 返回 (occupied, ledgerID, error)；若记录不存在或已终态，occupied=false。
	ObserveLedger(userID, asset, idempotencyKey string) (occupied bool, ledgerID string, err error)
}

// MockObserver 内存占用观察器，供使用。
type MockObserver struct {
	Frozen map[string]bool // key: "userID:asset:ledgerID" → 是否冻结
	Occupied map[string]string // key: "userID:asset:idemKey" → ledgerID（非空=有占用）
}

// NewMockObserver 构造一个空 MockObserver。
   func NewMockObserver() *MockObserver {
	  return &MockObserver{
		  Frozen:   make(map[string]bool),
		  Occupied: make(map[string]string),
	   }
   }

// SetFreeze 设置冻结状态（仅用于）。

   func (m *MockObserver) SetFreeze(userID, asset, ledgerID string, frozen bool) {
	  key := userID + ":" + asset + ":" + ledgerID
	  if frozen {
		  m.Frozen[key] = true
	   } else {
		   delete(m.Frozen, key)
	   }
   }

// SetLedger 设置账本占用（仅用于）。

   func (m *MockObserver) SetLedger(userID, asset, idempotencyKey, ledgerID string) {
	  key := userID + ":" + asset + ":" + idempotencyKey
	   if ledgerID != "" {
		  m.Occupied[key] = ledgerID
	  } else {
		   delete(m.Occupied, key)
	   }
   }

// ObserveFreeze 冻结查询。

   func (m *MockObserver) ObserveFreeze(userID, asset, ledgerID string) (bool, error) {
	  if m == nil {
		  return false, ErrObserver
	  }
	   key := userID + ":" + asset + ":" + ledgerID
	   return m.Frozen[key], nil
   }

// ObserveLedger 账本查询。
   func (m *MockObserver) ObserveLedger(userID, asset, idempotencyKey string) (bool, string, error) {
	  if m == nil {
		  return false, "", ErrObserver
	   }
	  key := userID + ":" + asset + ":" + idempotencyKey
	  lid, ok := m.Occupied[key]
	   if !ok || lid == "" {
		   return false, "", nil
	  }
	  return true, lid, nil
   }

// nilObserver 空观察器，ObserveFreeze/ObserveLedger 均返回 false（无占用）。
// 用于未绑定观察器时的兜底（不报错，仅返回保守估计）。

type nilObserver struct{}

  func (nilObserver) ObserveFreeze(_, _, _ string) (bool, error)         { return false, nil }
  func (nilObserver) ObserveLedger(_, _, _ string) (bool, string, error) { return false, "", nil }

// safeObserver 包装观察器，nil 时退化为 nilObserver。
  func safeObserver(o OccupyObserver) OccupyObserver {
	   if o == nil {
		   return nilObserver{}
	   }
	   return o
   }
