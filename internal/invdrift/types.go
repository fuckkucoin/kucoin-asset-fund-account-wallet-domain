package invdrift

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

// BatchState 批次状态机：open → scanning → graded → sealed | resolved

type BatchState string

const (
	BatchStateOpen BatchState = "open" // 已创建，尚未开始扫描
	BatchStateScanning BatchState = "scanning" // 正在读取两侧快照
	BatchStateGraded BatchState = "graded" // 分级完成，等待封堵或自动收敛
	BatchStateSealed BatchState = "sealed" // 存在 crit 条目，已触发封堵建议
	BatchStateResolved BatchState = "resolved" // 偏离已收敛，封堵建议全部过期
)

// AdviceState 封堵建议状态：proposed → acked | expired

type AdviceState string

const (
	AdviceStateProposed AdviceState = "proposed" // 已发出，等待确认
	AdviceStateAcked    AdviceState = "acked"    // 运维已确认处置
	AdviceStateExpired  AdviceState = "expired"  // 偏离已收敛，建议自动过期
)

// DriftGrade 偏离分级
type DriftGrade string

const (
	GradeInfo DriftGrade = "info" // 偏差 < 警戒阈，仅记录
	GradeWarn DriftGrade = "warn" // 警戒阈 ≤ 偏差 < 触发阈
	GradeCrit DriftGrade = "crit" // 偏差 ≥ 触发阈，触发封堵建议
)

// SealKind 封堵建议类型
type SealKind string

const (
	SealKindBlockSign      SealKind = "block_sign"       // 挡新签出金
	SealKindBlockSweepOver SealKind = "block_sweep_over" // 挡超额归集意图
	SealKindBlockLargeWdPre SealKind = "block_large_wd_pre" // 挡大额出金预检
)

// SideLabel 数据来源侧标签

type SideLabel string

const (
	SideLedger  SideLabel = "ledger"  // 账本系统科目（system.bridge/cold/warm）
	SideCustody SideLabel = "custody" // 托管库存观察
	SideHotInv SideLabel = "hotinv" // 热账观察
)

// DriftBatch 一次扫描批次，包含该时段所有资产的偏离汇总。
type DriftBatch struct {
	ID    string     `json:"id"`
	State BatchState `json:"state"`
	TriggerBy  string `json:"trigger_by,omitempty"`  // 触发者 actor，空表示调度触发
	DataCenter string `json:"data_center,omitempty"` // 机房/集群标识
	ScanAt time.Time `json:"scan_at"` // 开始扫描时间
	GradedAt *time.Time `json:"graded_at,omitempty"`
	SealedAt *time.Time `json:"sealed_at,omitempty"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	CritCount int `json:"crit_count"`
	WarnCount int `json:"warn_count"`
	InfoCount int `json:"info_count"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DriftItem 批次内单个资产的偏离条目。
type DriftItem struct {
	ID      string `json:"id"`
	BatchID string `json:"batch_id"`
}

// SealAdvice 封堵建议；由 crit DriftItem 触发，由外层闸决定是否实际挡写。

type SealAdvice struct {
	ID string `json:"id"`
	BatchID string `json:"batch_id"`
	ItemID string   `json:"item_id"`
	Asset  string   `json:"asset"`
	Kind   SealKind `json:"kind"`
	State AdviceState `json:"state"`
	Grade DriftGrade `json:"grade"`
	ProposedAt time.Time  `json:"proposed_at"`
	AckedAt    *time.Time `json:"acked_at,omitempty"`
	AckedBy    string     `json:"acked_by,omitempty"`
	ExpiredAt  *time.Time `json:"expired_at,omitempty"`
	ExpireReason string `json:"expire_reason,omitempty"` // 过期原因：resolved/manual
	TTLSeconds int `json:"ttl_seconds"`
	Note      string    `json:"note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// DriftProof 偏离证明：两侧快照摘要+差额+哈希，供审计只读。
// 篡改任一摘要字段会使 IntegrityOK 返回 false。
type DriftProof struct {
	ID            string `json:"id"`
	BatchID       string `json:"batch_id"`
	Asset         string `json:"asset"`
	LedgerDigest  string `json:"ledger_digest"`  // sha256(ledger_snapshot_json)
	CustodyDigest string `json:"custody_digest"` // sha256(custody_snapshot_json)
	HotInvDigest string `json:"hotinv_digest"` // sha256(hotinv_snapshot_json)
	LedgerSnap   string `json:"ledger_snap"`   // JSON 序列化快照原文
	CustodySnap string `json:"custody_snap"`
	HotInvSnap  string `json:"hotinv_snap"`
	DeltaHot    string `json:"delta_hot"`
	DeltaCold string `json:"delta_cold"`
	CompositeHash string `json:"composite_hash"` // sha256(ledger+custody+hotinv digest concat)
	Grade DriftGrade `json:"grade"`
	RecordedAt time.Time `json:"recorded_at"`
	CreatedAt  time.Time `json:"created_at"`
}

// IntegrityOK 校验三侧摘要与原文一致，且复合哈希正确。
// 篡改任一字段返回 false。
   func (p DriftProof) IntegrityOK() bool {
	  if !digestMatch(p.LedgerSnap, p.LedgerDigest) {
		  return false
	   }
	  if !digestMatch(p.CustodySnap, p.CustodyDigest) {
		  return false
	   }
	  if !digestMatch(p.HotInvSnap, p.HotInvDigest) {
		  return false
	  }
	  want := compositeHash(p.LedgerDigest, p.CustodyDigest, p.HotInvDigest)
	  return want == p.CompositeHash
  }

  func digestMatch(data, digest string) bool {
	  h := sha256.Sum256([]byte(data))
	   return hex.EncodeToString(h[:]) == digest
  }

   func compositeHash(a, b, c string) string {
	  h := sha256.Sum256([]byte(a + "|" + b + "|" + c))
	   return hex.EncodeToString(h[:])
   }

// BuildProof 从两侧快照构建 DriftProof，自动填充摘要与复合哈希。
   func BuildProof(batchID, asset string, ledger, custody, hotinv any, deltaHot, deltaCold string, grade DriftGrade) (DriftProof, error) {
	   lSnap, err := json.Marshal(ledger)
	   if err != nil {
		  return DriftProof{}, err
	   }
	   cSnap, err := json.Marshal(custody)
	  if err != nil {
		   return DriftProof{}, err
	   }
	   hSnap, err := json.Marshal(hotinv)
	  if err != nil {
		   return DriftProof{}, err
	   }
	   lh := sha256.Sum256(lSnap)
	  ch := sha256.Sum256(cSnap)
	  hh := sha256.Sum256(hSnap)
	   ld := hex.EncodeToString(lh[:])
	   cd := hex.EncodeToString(ch[:])
	   hd := hex.EncodeToString(hh[:])
	  now := time.Now().UTC()
	  return DriftProof{
		   BatchID:      batchID,
		   Asset:        asset,
		   LedgerDigest: ld,
		   CustodyDigest: cd,
		  HotInvDigest:  hd,
		   LedgerSnap:    string(lSnap),
		   CustodySnap: string(cSnap),
		  HotInvSnap:  string(hSnap),
		   DeltaHot:    deltaHot,
		   DeltaCold:   deltaCold,
		   CompositeHash: compositeHash(ld, cd, hd),
		   Grade: grade,
		  RecordedAt: now,
		   CreatedAt:  now,
	   }, nil
  }

// BatchSummary 批次汇总（用于 API 列表响应）
type BatchSummary struct {
	ID string `json:"id"`
	State BatchState `json:"state"`
	DataCenter string `json:"data_center,omitempty"`
	CritCount int `json:"crit_count"`
	WarnCount int `json:"warn_count"`
	InfoCount int        `json:"info_count"`
	ScanAt    time.Time  `json:"scan_at"`
	GradedAt  *time.Time `json:"graded_at,omitempty"`
}

// AssetDimView 资产维度偏离视图（API 汇总看板）

type AssetDimView struct {
	Asset      string     `json:"asset"`
	MaxGrade   DriftGrade `json:"max_grade"`
	BatchCount int        `json:"batch_count"`
	LastScanAt time.Time  `json:"last_scan_at"`
	DeltaHot string `json:"delta_hot"`
	DeltaCold string `json:"delta_cold"`
}

// AckRequest 运维确认封堵建议的请求体
type AckRequest struct {
	AdviceID string `json:"advice_id"`
	Actor    string `json:"actor"`
	Note string `json:"note,omitempty"`
}

// ForceReviewRequest 强制复核批次请求体
type ForceReviewRequest struct {
	BatchID string `json:"batch_id"`
	Actor string `json:"actor"`
	Note  string `json:"note,omitempty"`
}
