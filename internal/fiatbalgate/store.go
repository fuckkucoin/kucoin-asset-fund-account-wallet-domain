package fiatbalgate

import (
	"context"
	"time"
)

// Store fiatbalgate 持久化接口。
type Store interface {
	// ── JournalGeneration ─────────────────────────────────────────────────────
	// SaveGeneration 写入（或幂等更新）世代快照。同 Seq 二次写忽略（INSERT IGNORE）。
	SaveGeneration(ctx context.Context, g *JournalGeneration) error
	// GetGenerationBySeq 按 Seq 查询；未找到返回 ErrGenNotFound。
	GetGenerationBySeq(ctx context.Context, seq uint64) (*JournalGeneration, error)
	// MaxGenSeq 返回已写入的最大世代序号；无记录时返回 0, nil。
	MaxGenSeq(ctx context.Context) (uint64,
		error)
	// ── FiatImbalanceCase ─────────────────────────────────────────────────────
	// SaveCase 新建案件（insert-only）。
	SaveCase(ctx context.Context, c *FiatImbalanceCase) error
	// GetCase 按 case_id 查询；未找到返回 ErrCaseNotFound。
	GetCase(ctx context.Context, caseID string) (*FiatImbalanceCase, error)
	// UpdateCaseState CAS 更新案件状态（from→to），并可选更新 patch 中的附加字段。
	// 当前状态不等于 from 时返回 ErrInvalidTransition。
	UpdateCaseState(ctx context.Context, caseID string, from, to CaseState, patch *FiatImbalanceCase) error
	// ListOpenOrBlockingCases 返回 open / blocking 状态的案件（按 opened_at 降序）。
	// asset 不为空时只返回该资产的案件；limit <= 0 时使用默认值 100。
	ListOpenOrBlockingCases(ctx context.Context, asset string, limit int) ([]*FiatImbalanceCase, error)
	// ListCasesByAsset 返回指定资产的全部案件（按 opened_at 降序）；limit <= 0 时默认 50。
	ListCasesByAsset(ctx context.Context, asset string, limit int) ([]*FiatImbalanceCase, error)
	// ── FiatImbalanceProof ────────────────────────────────────────────────────
	// SaveProof 写入证明记录（insert-only）；同 case_id 二次写返回 ErrAlreadyProven。
	SaveProof(ctx context.Context, p *FiatImbalanceProof) error
	// GetProofByCase 按 case_id 查询；未找到返回 ErrProofNotFound。
	GetProofByCase(ctx context.Context,
		caseID string) (*FiatImbalanceProof, error)
	// GetProof 按 proof_id 查询；未找到返回 ErrProofNotFound。
	GetProof(ctx context.Context, proofID string) (*FiatImbalanceProof, error)
	// UpdateProofState CAS 更新证明状态（from→to）；状态不符返回 ErrInvalidTransition。
	UpdateProofState(ctx context.Context, proofID string, from, to ProofState, patch *FiatImbalanceProof) error
	// ── ClearApproval ─────────────────────────────────────────────────────────
	// SaveApproval 写入四眼清障确认记录（insert-only）。
	SaveApproval(ctx context.Context, a *ClearApproval) error
	// ListApprovalsByCase 返回指定 case_id 的全部确认记录（按 created_at 升序）。
	ListApprovalsByCase(ctx context.Context,
		caseID string) ([]*ClearApproval, error)
	// ── 辅助 ──────────────────────────────────────────────────────────────────
	// HasActiveBlockingForAsset 快速判断指定资产是否存在未清（open/blocking）的案件。
	// 只需 true/false，由 Engine.CheckBlock 调用；禁止加载完整列表。
	HasActiveBlockingForAsset(ctx context.Context,
		asset string) (bool, error)
	// ListExpiredOpenCases 返回超过 staleThreshold 仍为 open（未推进到 blocking）的案件。
	// 供 Worker 推进遗漏的 open→blocking。
	ListExpiredOpenCases(ctx context.Context, staleThreshold time.Time, limit int) ([]*FiatImbalanceCase, error)
}
