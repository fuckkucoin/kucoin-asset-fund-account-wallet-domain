package investigate

import (
	"context"
	"strings"
	"time"
)

// SnapBindResult 案件开启时的快照绑定结果。

type SnapBindResult struct {
	// CaseID 调查案 ID。
	CaseID string `json:"case_id"`
	// SnapshotID 绑定的快照 ID（若绑定失败为空）。
	SnapshotID string `json:"snapshot_id,omitempty"`
	// SnapshotGen 快照世代标识符（若绑定失败为空）。
	SnapshotGen string `json:"snapshot_gen,omitempty"`
	// BoundAt 绑定时刻。
	BoundAt time.Time `json:"bound_at,omitempty"`
	// BindError 绑定失败原因（non-fatal；记录但不阻断案件开启）。
	BindError string `json:"bind_error,omitempty"`
}

// SnapEngineAdapter tracesnapgate.Engine 适配接口（避免循环依赖）。
// 仅暴露 investigate 需要的最小方法集。

type SnapEngineAdapter interface {
	// CreateAndSealSnapshot 创建并密封 open 快照，返回快照 ID 与 SnapshotGen。
	// 由 investigate.Service 在 Open 时同步调用；失败为 non-fatal（案件仍开启）。
	CreateAndSealSnapshot(ctx context.Context, userID, asset, actor, idemKey string) (snapshotID, snapshotGen string, err error)
	// BindCaseToSnapshot 将案件绑定到已密封快照（幂等）。
	BindCaseToSnapshot(ctx context.Context, caseID, snapshotID, boundBy string) error
	// GetSnapshotGenByCaseID 按案件 ID 读取绑定快照的 SnapshotGen。
	GetSnapshotGenByCaseID(ctx context.Context, caseID string) (snapshotGen string, found bool, err error)
}

// SnapBinder 封装快照绑定逻辑，供 investigate.Service 集成使用。
type SnapBinder struct {
	eng SnapEngineAdapter
}

// NewSnapBinder 创建 SnapBinder；eng 为 nil 时降级为 no-op。
   func NewSnapBinder(eng SnapEngineAdapter) *SnapBinder {
	   return &SnapBinder{eng: eng}
   }

// BindOnOpen 在调查案开启时同步绑定快照（non-fatal 失败）。
// 参数：
// - caseID：新开调查案 ID。
// - userID：被调查用户 ID。
// - asset：追踪资产代码（空串时跳过绑定）。
// - actor：操作员 ID。
// 不变量：返回的 SnapBindResult.BindError 非空时案件本身不受影响。
  func (b *SnapBinder) BindOnOpen(ctx context.Context, caseID, userID, asset, actor string) SnapBindResult {
	   res := SnapBindResult{CaseID: caseID}
	  if b == nil || b.eng == nil {
		   res.BindError = "snap_engine_unavailable"
		   return res
	  }
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  if asset == "" {
		  res.BindError = "asset_required_for_snap_bind"
		  return res
	   }
	  idemKey := "case-open:" + caseID + ":" + asset
	   snapID, snapGen, err := b.eng.CreateAndSealSnapshot(ctx, userID, asset, actor, idemKey)
	   if err != nil {
		   res.BindError = "snapshot_create_failed: " + err.Error()
		   return res
	   }
	  if err := b.eng.BindCaseToSnapshot(ctx, caseID, snapID, actor); err != nil {
		   res.BindError = "snapshot_bind_failed: " + err.Error()
		  return res
	   }
	   res.SnapshotID = snapID
	  res.SnapshotGen = snapGen
	   res.BoundAt = time.Now().UTC()
	   return res
  }

// GetSnapshotGen 按案件 ID 查快照世代（只读）。
// 返回 "" 表示未绑定（缺 snapshot_gen 时法医导出必须拒绝）。

  func (b *SnapBinder) GetSnapshotGen(ctx context.Context, caseID string) string {
	  if b == nil || b.eng == nil {
		   return ""
	  }
	  gen, _, _ := b.eng.GetSnapshotGenByCaseID(ctx, caseID)
	  return gen
  }

// EnrichedLookup 在 Lookup 基础上追加快照绑定元数据。
type EnrichedLookup struct {
	Lookup
	// SnapshotGen 案件绑定的快照世代标识符（空串表示未绑定）。
	SnapshotGen string `json:"snapshot_gen,omitempty"`
	// SnapshotID 案件绑定的快照 ID（空串表示未绑定）。
	SnapshotID string `json:"snapshot_id,omitempty"`
	// ForensicReady true 表示已绑定 snapshot_gen，可执行法医导出。
	ForensicReady bool `json:"forensic_ready"`
}

// ValidateForensicExport 检查案件是否满足法医导出条件（fail-closed）。
// 失败返回 ErrSnapGenRequired（对应 tracesnapgate.ErrNoSnapshot 语义）。
   func ValidateForensicExport(ctx context.Context, caseID string, binder *SnapBinder) error {
	  if binder == nil {
		   return ErrSnapGenRequired
	  }
	   gen := binder.GetSnapshotGen(ctx, caseID)
	  if strings.TrimSpace(gen) == "" {
		   return ErrSnapGenRequired
	   }
	  return nil
  }
