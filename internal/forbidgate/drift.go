package forbidgate

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// DriftService 漂移案例生命周期管理。
type DriftService struct {
	repo *Repo
}

// NewDriftService 构造漂移服务。

  func NewDriftService(repo *Repo) *DriftService {
	   return &DriftService{repo: repo}
  }

// GetDriftCase 读取单个漂移案例。
   func (s *DriftService) GetDriftCase(ctx context.Context, caseID string) (ForbidDriftCase, error) {
	  if s.repo == nil {
		   return ForbidDriftCase{}, ErrProofMissing
	  }
	   return s.repo.GetDriftCase(ctx, caseID)
  }

// ListOpenDriftCases 列出用户的 open/proven 漂移案例（用于挡写检查）。
  func (s *DriftService) ListOpenDriftCases(ctx context.Context, userID string) ([]ForbidDriftCase, error) {
	   if s.repo == nil {
		  return nil, nil
	  }
	   return s.repo.ListDriftCasesByUserState(ctx, userID, DriftStateOpen)
  }

// IsWriteBlocked 检查用户是否因漂移案例被挡写。
   func (s *DriftService) IsWriteBlocked(ctx context.Context, userID string) (bool, string, error) {
	   cases, err := s.ListOpenDriftCases(ctx, userID)
	  if err != nil {
		   return false, "", err
	  }
	  for _, c := range cases {
		   if c.State == DriftStateOpen || c.State == DriftStateProven {
			  return true, c.ID, nil
		   }
	   }
	   return false, "", nil
  }

// SubmitProof 提交漂移证明（第一人）。状态转 proven，不得自确认。
   func (s *DriftService) SubmitProof(ctx context.Context, caseID, submitter, summary, evidence string) (ForbidDriftProof, error) {
	  caseID = strings.TrimSpace(caseID)
	   submitter = strings.TrimSpace(submitter)
	  if caseID == "" || submitter == "" {
		  return ForbidDriftProof{}, ErrInvalidActor
	  }
	   if strings.TrimSpace(summary) == "" || strings.TrimSpace(evidence) == "" {
		   return ForbidDriftProof{}, ErrProofMissing
	   }
	   dc, err := s.repo.GetDriftCase(ctx, caseID)
	  if err != nil {
		  return ForbidDriftProof{}, fmt.Errorf("forbidgate drift: get case: %w", err)
	  }
	   if dc.State == DriftStateClosed {
		  return ForbidDriftProof{}, fmt.Errorf("forbidgate drift: case already closed")
	  }
	   now := time.Now().UTC()
	   proof := ForbidDriftProof{
		  ID:          uuid.NewString(),
		   DriftCaseID: caseID,
		   SubmittedBy: submitter,
		   Summary:     summary,
		  Evidence:    evidence,
		  SubmittedAt: now,
	  }
	   if err := s.repo.InsertDriftProof(ctx, proof); err != nil {
		   return ForbidDriftProof{}, fmt.Errorf("forbidgate drift: insert proof: %w", err)
	   }
	   // 转 proven。
	  dc.State = DriftStateProven
	   dc.ProvenAt = &now
	  dc.ProofID = proof.ID
	   dc.ProofSummary = summary
	   if err := s.repo.UpdateDriftCaseState(ctx, dc); err != nil {
		   return ForbidDriftProof{}, fmt.Errorf("forbidgate drift: update case proven: %w", err)
	   }
	   return proof, nil
   }

// ConfirmProof 双人确认证明（第二人不同于提交人）→ 案例转 closed，解除挡写。

  func (s *DriftService) ConfirmProof(ctx context.Context, proofID, confirmer string) error {
	  confirmer = strings.TrimSpace(confirmer)
	   if proofID == "" || confirmer == "" {
		   return ErrInvalidActor
	   }
	   proof, err := s.repo.GetDriftProof(ctx, proofID)
	   if err != nil {
		   return fmt.Errorf("forbidgate drift: get proof: %w", err)
	  }
	   if strings.EqualFold(proof.SubmittedBy, confirmer) {
		  return ErrProofConfirmConflict
	   }
	   if proof.ConfirmedAt != nil {
		   return nil // 幂等
	  }
	  now := time.Now().UTC()
	  proof.ConfirmedBy = confirmer
	  proof.ConfirmedAt = &now
	  if err := s.repo.UpdateDriftProofConfirmed(ctx, proof); err != nil {
		   return fmt.Errorf("forbidgate drift: confirm proof: %w", err)
	  }
	   // 关闭案例。
	  dc,
		   err := s.repo.GetDriftCase(ctx, proof.DriftCaseID)
	   if err != nil {
		  return fmt.Errorf("forbidgate drift: get case for close: %w", err)
	  }
	  if dc.State == DriftStateClosed {
		  return nil // 幂等
	   }
	  dc.State = DriftStateClosed
	   dc.ClosedAt = &now
	  dc.ClosedBy = confirmer
	  return s.repo.UpdateDriftCaseState(ctx, dc)
   }

// ExportProof 导出禁提证明摘要（含 user/actor/reason/gen）。
  func (s *DriftService) ExportProof(ctx context.Context, caseID string) (map[string]any, error) {
	  dc,
		  err := s.repo.GetDriftCase(ctx, caseID)
	   if err != nil {
		   return nil, fmt.Errorf("forbidgate drift: get case for export: %w", err)
	  }
	   if dc.ProofID == "" {
		   return nil, ErrNoProofToExport
	  }
	  proof, err := s.repo.GetDriftProof(ctx, dc.ProofID)
	   if err != nil {
		  return nil, fmt.Errorf("forbidgate drift: get proof for export: %w", err)
	  }
	   return map[string]any{
		  "case_id": dc.ID,
		  "user_id": dc.UserID,
		  "withdraw_ref":    dc.WithdrawRef,
		   "kind":            dc.Kind,
		  "gen_at_precheck": dc.GenAtPrecheck,
		  "gen_at_detect": dc.GenAtDetect,
		   "state":         dc.State,
		   "detected_at":  dc.DetectedAt,
		   "proof_id":     proof.ID,
		  "submitted_by": proof.SubmittedBy,
		  "confirmed_by": proof.ConfirmedBy,
		  "summary":      proof.Summary,
		  "evidence":     proof.Evidence,
		  "submitted_at": proof.SubmittedAt,
		  "confirmed_at": proof.ConfirmedAt,
	  }, nil
  }
