package fiatbalgate

import "fmt"

var (
	// ErrUnavailable Store 或 Engine 为 nil（配置错误）。
	ErrUnavailable = fmt.Errorf("fiatbalgate unavailable")
	// ErrCaseNotFound 按 case_id 未找到案件记录。
	ErrCaseNotFound = fmt.Errorf("fiatbalgate case not found")
	// ErrProofNotFound 按 proof_id / case_id 未找到证明记录。
	ErrProofNotFound = fmt.Errorf("fiatbalgate proof not found")
	// ErrGenNotFound 按 gen_id / seq 未找到世代记录。
	ErrGenNotFound = fmt.Errorf("fiatbalgate gen not found")
	// ErrApprovalNotFound 按 approval_id / case_id 未找到清障确认记录。
	ErrApprovalNotFound = fmt.Errorf("fiatbalgate approval not found")
	// ErrAlreadyBlocking 该资产已存在 blocking 状态案件；不重复开案。
	ErrAlreadyBlocking = fmt.Errorf("fiatbalgate already blocking")
	// ErrInvalidTransition 状态机非法跳转（CAS 校验失败）。
	ErrInvalidTransition = fmt.Errorf("fiatbalgate invalid state transition")
	// ErrAlreadyProven 同一 Case 已存在证明记录（唯一约束冲突）。
	ErrAlreadyProven = fmt.Errorf("fiatbalgate proof already exists")
	// ErrAlreadyApproved 同一操作人重复提交清障确认（禁止自我双签）。
	ErrAlreadyApproved = fmt.Errorf("fiatbalgate already approved by same actor")
	// ErrCaseNotBlocking 清障操作要求案件处于 blocking 状态；当前不满足。
	ErrCaseNotBlocking = fmt.Errorf("fiatbalgate case not in blocking state")
	// ErrCaseCleared 案件已清障；不得重复操作。
	ErrCaseCleared = fmt.Errorf("fiatbalgate case already cleared")
	// ErrNoteRequired 清障备注不得为空。
	ErrNoteRequired = fmt.Errorf("fiatbalgate clear note required")
	// ErrBlockedByImbalance Post 路径被不平衡案挡写；调用方必须拒绝写入。
	ErrBlockedByImbalance = fmt.Errorf("fiatbalgate post blocked by imbalance case")
	// ErrScannerIDRequired 扫描请求必须携带 scanner_id。
	ErrScannerIDRequired = fmt.Errorf("fiatbalgate scanner_id required")
	// ErrAssetRequired 操作必须指定资产代码。
	ErrAssetRequired = fmt.Errorf("fiatbalgate asset required")
)

// ErrBlockDetail 携带具体不平衡案 ID 的挡写错误（可供调用方日志）。
type ErrBlockDetail struct {
	CaseID string
	Asset string
	State CaseState
}

   func (e *ErrBlockDetail) Error() string {
	  return fmt.Sprintf("%s: case=%s asset=%s state=%s", ErrBlockedByImbalance, e.CaseID, e.Asset, e.State)
   }

  func (e *ErrBlockDetail) Unwrap() error { return ErrBlockedByImbalance }
