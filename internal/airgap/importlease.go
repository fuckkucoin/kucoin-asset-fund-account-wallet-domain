package airgap

import "time"

// LeaseState 导入租约状态。

type LeaseState string

const (
	LeaseOpen      LeaseState = "open"
	LeaseImporting LeaseState = "importing"
	LeaseImported LeaseState = "imported"
	LeaseAborted LeaseState = "aborted"
	LeaseExpired LeaseState = "expired"
)

// LeaseStateTerminal 判断是否为终态。
  func (s LeaseState) Terminal() bool {
	   return s == LeaseImported || s == LeaseAborted || s == LeaseExpired
  }

// ImportLease 一次密封导入的独占世代租约。
type ImportLease struct {
	LeaseID string `json:"lease_id"`
	SessionID  string `json:"session_id"`
	ImportGen  string `json:"import_gen"`  // air_import_gen，外部传入，不能过期
	BlobDigest string `json:"blob_digest"` // sha256(sealed_blob)，hex 编码
	Actor string     `json:"actor"`
	State LeaseState `json:"state"`
	ExpiresAt time.Time `json:"expires_at"`
	ImportedAt *time.Time `json:"imported_at,omitempty"`
	AbortedAt   *time.Time `json:"aborted_at,omitempty"`
	AbortReason string     `json:"abort_reason,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Copy 深拷贝 ImportLease。

  func (l ImportLease) Copy() ImportLease {
	   cp := l
	  if l.ImportedAt != nil {
		  t := *l.ImportedAt
		   cp.ImportedAt = &t
	  }
	  if l.AbortedAt != nil {
		  t := *l.AbortedAt
		   cp.AbortedAt = &t
	   }
	  return cp
  }

// AirImportGen 供外部只读读取当前会话关联的导入世代（若无则空串）。
// 不变量：返回值仅作只读透出；写路径在 airimportgate。
  func AirImportGen(lease *ImportLease) string {
	  if lease == nil {
		  return ""
	   }
	  return lease.ImportGen
  }
