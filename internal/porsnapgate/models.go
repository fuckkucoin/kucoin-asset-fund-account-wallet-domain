package porsnapgate

import "time"

// ─────────────────────────────────────────────────────────────────────────────
// SnapGenState — 快照世代状态机
// ─────────────────────────────────────────────────────────────────────────────
// SnapGenState 描述 por_snap_gen 在其生命周期内的状态节点。
type SnapGenState string

const (
	// SnapPending 世代已分配 ID，尚未完成 Build/Seal。内部中间态，外部不可见。
	SnapPending SnapGenState = "pending"
	// SnapSealed Build/Seal 完成，根与 totals 指纹已钉，可申请公证。
	SnapSealed SnapGenState = "sealed"
	// SnapAttested 已关联一条 published AttestationBundle，世代进入公证态。
	SnapAttested SnapGenState = "attested"
	// SnapExpired 超过 TTL 未完成公证，或主动归档。
	SnapExpired SnapGenState = "expired"
	// SnapRevoked 显式撤销；已发布公证须一并撤销。
	SnapRevoked SnapGenState = "revoked"
)

// ─────────────────────────────────────────────────────────────────────────────
// AttestState — 公证状态机
// ─────────────────────────────────────────────────────────────────────────────
// AttestState 描述 por_attestations 行的生命周期。

type AttestState string

const (
	// AttestPending 公证请求已提交，尚未对外发布。
	AttestPending AttestState = "pending"
	// AttestPublished 公证已对外发布，可供 ProveAttested 使用。
	AttestPublished AttestState = "published"
	// AttestRevoked 公证已撤销（世代过期或人工干预）。
	AttestRevoked AttestState = "revoked"
)

// ─────────────────────────────────────────────────────────────────────────────
// ProofKind — 漂移证明类型
// ─────────────────────────────────────────────────────────────────────────────
// ProofKind 区分漂移证明来源。
type ProofKind string

const (
	// DriftKindReplay 按 snap_gen 重放时发现 live 根已变。
	DriftKindReplay ProofKind = "replay"
	// DriftKindCompare CompareWithProof 与活账时发现漂移。
	DriftKindCompare ProofKind = "compare"
)

// ─────────────────────────────────────────────────────────────────────────────
// PorSnapshotGen — 快照世代（核心租约）
// ─────────────────────────────────────────────────────────────────────────────
// PorSnapshotGen 封存 BuildWithGen 完成时的 Merkle 根、totals 指纹、
// 时间戳与 store 水位，形成不可变世代记录。
// 任何 Attest/导出均须携带同一 SnapGenID；缺 gen 的导出请求必须被拒绝。
// live 模式读取时不得写入本表。

type PorSnapshotGen struct {
	// SnapGenID 全局唯一标识符（UUID v4）。
	SnapGenID string `json:"snap_gen_id" db:"snap_gen_id"`
	// SnapshotID 对应 por.Snapshot.ID；一对一关系。
	SnapshotID string `json:"snapshot_id" db:"snapshot_id"`
	// MerkleRoot 封印时 por.Tree.Root（hex SHA-256）。
	MerkleRoot string `json:"merkle_root" db:"merkle_root"`
	// LeavesDigest leaves 集合的 SHA-256 摘要根（hex）。
	// 由 hashLeafSet(sorted canonical strings) 计算，防止顺序攻击。
	LeavesDigest string `json:"leaves_digest" db:"leaves_digest"`
	// TotalsFingerprint 各资产合计的 SHA-256 指纹（hex）。
	// 形态：sha256(asset1:amount1|asset2:amount2|… 按 asset 升序)。
	TotalsFingerprint string `json:"totals_fingerprint" db:"totals_fingerprint"`
	// AsOf 快照取样时刻（UTC）。
	AsOf time.Time `json:"as_of" db:"as_of"`
	// StoreMark store 水位标记；BuildWithGen 完成时从 ledger.Store 读取最新转账序号。
	// 重放时若 store 已超过此水位，则 live 账已变。
	StoreMark int64 `json:"store_mark" db:"store_mark"`
	// UserCount 本次快照覆盖的用户数。
	UserCount int `json:"user_count" db:"user_count"`
	// LeafCount 叶节点数量。
	LeafCount int `json:"leaf_count" db:"leaf_count"`
	// State 当前状态（sealed / attested / expired / revoked）。
	State SnapGenState `json:"state" db:"state"`
	// TTLSeconds 世代有效期；超时后 worker 迁移为 expired。默认 86400（24 h）。
	TTLSeconds int64 `json:"ttl_seconds" db:"ttl_seconds"`
	// SealedAt 封印完成时刻（UTC）。
	SealedAt time.Time `json:"sealed_at" db:"sealed_at"`
	// AttestedAt 关联公证的发布时刻（UTC）；State=attested 时填充。
	AttestedAt *time.Time `json:"attested_at,omitempty" db:"attested_at"`
	// ExpiredAt 过期时刻（UTC）。
	ExpiredAt *time.Time `json:"expired_at,omitempty" db:"expired_at"`
	// RevokedAt 撤销时刻（UTC）。
	RevokedAt *time.Time `json:"revoked_at,omitempty" db:"revoked_at"`
	// CreatedAt 行创建时刻（UTC）。
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	// UpdatedAt 行最后更新时刻（UTC）。
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// IsActive 返回世代是否处于活跃状态（sealed 或 attested）。

   func (g *PorSnapshotGen) IsActive() bool {
	  return g.State == SnapSealed || g.State == SnapAttested
  }

// IsForFormalAttest 判断世代是否可用于发起公证。只有 sealed 态允许。
  func (g *PorSnapshotGen) IsForFormalAttest() bool {
	   return g.State == SnapSealed
  }

// ExpiresAt 返回世代绝对过期时刻。
   func (g *PorSnapshotGen) ExpiresAt() time.Time {
	  return g.SealedAt.Add(time.Duration(g.TTLSeconds) * time.Second)
   }

// ─────────────────────────────────────────────────────────────────────────────
// AttestationBundle — 公证包
// ─────────────────────────────────────────────────────────────────────────────
// AttestationBundle 代表一次的准备金公证导出。
// 必须携带关联的 PorSnapshotGen（SnapGenID）；缺失则被 Attest 拒绝。
// 发布后供 ProveAttested 使用；撤销后一切基于该公证的证明请求均需告警。

type AttestationBundle struct {
	// AttestID 全局唯一标识符（UUID v4）。
	AttestID string `json:"attest_id" db:"attest_id"`
	// SnapGenID 绑定的快照世代 ID；外键约束 por_snap_gens.snap_gen_id。
	SnapGenID string `json:"snap_gen_id" db:"snap_gen_id"`
	// SnapshotID 透传自 PorSnapshotGen.SnapshotID，方便快速关联。
	SnapshotID string `json:"snapshot_id" db:"snapshot_id"`
	// MerkleRoot 快照封印根（从 gen 复制，防止请求传参篡改）。
	MerkleRoot string `json:"merkle_root" db:"merkle_root"`
	// TotalsFingerprint 合计指纹（从 gen 复制）。
	TotalsFingerprint string `json:"totals_fingerprint" db:"totals_fingerprint"`
	// ExportPayload 公证导出内容（JSON 编码），供下游外部审计使用。
	ExportPayload []byte `json:"export_payload" db:"export_payload"`
	// ExportHash ExportPayload 的 SHA-256（hex）；防篡改校验。
	ExportHash string `json:"export_hash" db:"export_hash"`
	// State 当前公证状态（pending / published / revoked）。
	State AttestState `json:"state" db:"state"`
	// Requester 发起公证的操作人 ID（ops uid 或 service account）。
	Requester string `json:"requester" db:"requester"`
	// PublishedAt 公证发布时刻（UTC）。
	PublishedAt *time.Time `json:"published_at,omitempty" db:"published_at"`
	// RevokedAt 公证撤销时刻（UTC）。
	RevokedAt *time.Time `json:"revoked_at,omitempty" db:"revoked_at"`
	// RevokeReason 撤销原因。
	RevokeReason string `json:"revoke_reason,omitempty" db:"revoke_reason"`
	// CreatedAt 行创建时刻（UTC）。
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	// UpdatedAt 行最后更新时刻（UTC）。
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}

// IsUsable 返回公证是否可用于证明（仅 published 态）。

   func (a *AttestationBundle) IsUsable() bool {
	  return a.State == AttestPublished
  }

// ─────────────────────────────────────────────────────────────────────────────
// PorSnapDriftProof — 活账漂移证明
// ─────────────────────────────────────────────────────────────────────────────
// PorSnapDriftProof 记录一次「按世代重放或与活账」时发现的漂移事件。
// 语义保证：
// - Replay 场景：同一 snap_gen 重建 Merkle 树时，live 数据已变，根不一致。
// - Compare 场景：将封印快照与当前 live 账时，发现 total/leaf 差异。
// 本表只追加，不允许修改或删除。供下游对账与运营审计只读消费。
type PorSnapDriftProof struct {
	// ProofID 全局唯一标识符（UUID v4）。
	ProofID string `json:"proof_id" db:"proof_id"`
	// SnapGenID 触发漂移的世代 ID。
	SnapGenID string `json:"snap_gen_id" db:"snap_gen_id"`
	// SnapshotID 对应快照 ID，方便关联查询。
	SnapshotID string `json:"snapshot_id" db:"snapshot_id"`
	// Kind 漂移来源（replay / compare）。
	Kind ProofKind `json:"kind" db:"kind"`
	// SealedRoot 封印时记录的 Merkle 根（hex）。
	SealedRoot string `json:"sealed_root" db:"sealed_root"`
	// LiveRoot 本次重放/时实时计算的 Merkle 根（hex）；与 SealedRoot 不同。
	LiveRoot string `json:"live_root" db:"live_root"`
	// SealedTotals 封印时 totals 快照（JSON 数组）。
	SealedTotals []byte `json:"sealed_totals" db:"sealed_totals"`
	// LiveTotals 重放/时实时 totals（JSON 数组）。
	LiveTotals []byte `json:"live_totals" db:"live_totals"`
	// DriftAssets 漂移的资产列表（JSON 数组），形态：[{asset, sealed, live}]。
	DriftAssets []byte `json:"drift_assets" db:"drift_assets"`
	// SealedStoreMark 封印时记录的 store 水位。
	SealedStoreMark int64 `json:"sealed_store_mark" db:"sealed_store_mark"`
	// LiveStoreMark 重放/时的当前 store 水位。
	LiveStoreMark int64 `json:"live_store_mark" db:"live_store_mark"`
	// DetectedAt 漂移发现时刻（UTC）。
	DetectedAt time.Time `json:"detected_at" db:"detected_at"`
	// CreatedAt 行创建时刻（UTC）。
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// ─────────────────────────────────────────────────────────────────────────────
// ReadMode — live 读取模式标记
// ─────────────────────────────────────────────────────────────────────────────
// ReadMode 控制快照构建时的读取语义。
// Attestation 模式禁止使用 live 读取；live 读取必须显式标记为非公证。

type ReadMode int

const (
	// ReadModeLive 显式 live 模式，结果标记为非法医/非公证，禁止写入公证表。
	ReadModeLive ReadMode = iota
	// ReadModeSealed 从已封印世代重建，用于 Prove/ProveAttested。
	ReadModeSealed
)

// ─────────────────────────────────────────────────────────────────────────────
// DriftAsset — 漂移资产明细
// ─────────────────────────────────────────────────────────────────────────────
// DriftAsset 单个资产在封印快照与 live 账之间的差异。

type DriftAsset struct {
	Asset string `json:"asset"`
	Sealed string `json:"sealed"`
	Live   string `json:"live"`
}

// ─────────────────────────────────────────────────────────────────────────────
// LiveExportTag — live 导出标记
// ─────────────────────────────────────────────────────────────────────────────
// LiveExportTag 附加到非公证 live 导出结果，标记其不可用于法医重放或公证。
type LiveExportTag struct {
	Mode string `json:"mode"`
	NonForensic    bool      `json:"non_forensic"`
	NonAttestation bool      `json:"non_attestation"`
	GeneratedAt    time.Time `json:"generated_at"`
	Disclaimer     string    `json:"disclaimer"`
}

// NewLiveExportTag 构造口径 live 导出标记。

  func NewLiveExportTag(now time.Time) LiveExportTag {
	   return LiveExportTag{
		   Mode: "live",
		  NonForensic:    true,
		   NonAttestation: true,
		   GeneratedAt: now.UTC(),
		   Disclaimer:  "live read: not a sealed attestation; root may not be reproduced; must not be used as then-root",
	   }
   }

// ─────────────────────────────────────────────────────────────────────────────
// BuildWithGenResult — BuildWithGen 返回值
// ─────────────────────────────────────────────────────────────────────────────
// BuildWithGenResult 包含 BuildWithGen 返回的快照与已封印世代。

type BuildWithGenResult struct {
	// SnapshotID 对应 por.Snapshot.ID。
	SnapshotID string `json:"snapshot_id"`
	// SnapGenID 封印的世代 ID；调用方可用于后续 Attest / ProveAttested。
	SnapGenID string `json:"snap_gen_id"`
	// MerkleRoot 封印根。
	MerkleRoot string `json:"merkle_root"`
	// LeavesDigest leaves 摘要根。
	LeavesDigest string `json:"leaves_digest"`
	// TotalsFingerprint totals 指纹。
	TotalsFingerprint string `json:"totals_fingerprint"`
	// AsOf 快照时刻。
	AsOf time.Time `json:"as_of"`
	// StoreMark 封印时 store 水位。
	StoreMark int64 `json:"store_mark"`
	// UserCount 覆盖用户数。
	UserCount int `json:"user_count"`
	// LeafCount 叶节点数。
	LeafCount int `json:"leaf_count"`
}

// ─────────────────────────────────────────────────────────────────────────────
// CompareWithProofResult — CompareWithProof 返回值
// ─────────────────────────────────────────────────────────────────────────────
// CompareWithProofResult 封装结果；若发现漂移，DriftProof 非空。
type CompareWithProofResult struct {
	// SnapGenID 被的世代 ID。
	SnapGenID string `json:"snap_gen_id"`
	// Drifted 是否检测到漂移。
	Drifted bool `json:"drifted"`
	// DriftProof 漂移证明；Drifted=false 时为 nil。
	DriftProof *PorSnapDriftProof `json:"drift_proof,omitempty"`
	// DriftAssets 漂移的资产明细；Drifted=false 时为空。
	DriftAssets []DriftAsset `json:"drift_assets,omitempty"`
}

// ─────────────────────────────────────────────────────────────────────────────
// ProveAttestedResult — ProveAttested 返回值
// ─────────────────────────────────────────────────────────────────────────────
// ProveAttestedResult 按世代重建并生成 Merkle 包含证明。

type ProveAttestedResult struct {
	// AttestID 绑定的公证 ID。
	AttestID string `json:"attest_id"`
	// SnapGenID 世代 ID。
	SnapGenID string `json:"snap_gen_id"`
	// MerkleRoot 封印根（与世代存储的根一致）。
	MerkleRoot string `json:"merkle_root"`
	// ProofPath 叶节点到根的 Merkle 路径（与 por.Proof 一致）。
	ProofPath []interface{} `json:"proof_path"`
	// Leaf 目标叶节点。
	Leaf interface{} `json:"leaf"`
	// Verified 服务端已验证路径通过。
	Verified bool `json:"verified"`
}

// ─────────────────────────────────────────────────────────────────────────────
// AttestRequest — Attest API 请求参数
// ─────────────────────────────────────────────────────────────────────────────
// AttestRequest 发起公证时的输入参数。缺 SnapGenID 时服务端拒绝。
type AttestRequest struct {
	// SnapGenID 绑定世代，必填；缺失则 ErrNoGen。
	SnapGenID string `json:"snap_gen_id"`
	// Requester 操作人 ID。
	Requester string `json:"requester"`
	// ExportPayload 公证导出内容（JSON 编码）。
	ExportPayload []byte `json:"export_payload,omitempty"`
}
