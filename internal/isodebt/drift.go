package isodebt

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/isobook"
)

// DriftDetector 负责从 isobook + PairGeneration 发现漂移并开立 RatioDriftCase。
type DriftDetector struct {
	store Store
	genMgr *GenerationManager
	book  *isobook.Book
	cfg   Config
	clock func() time.Time
}

// NewDriftDetector 构造漂移检测器。
  func NewDriftDetector(store Store, genMgr *GenerationManager, book *isobook.Book, cfg Config) *DriftDetector {
	  return &DriftDetector{
		  store: store,
		   genMgr: genMgr,
		   book: book,
		  cfg: cfg,
		  clock: func() time.Time { return time.Now().UTC() },
	  }
  }

// WithClock 注入时钟。
  func (d *DriftDetector) WithClock(c func() time.Time) *DriftDetector {
	   if d != nil && c != nil {
		   d.clock = c
	   }
	   return d
  }

   // DetectResult 单次漂移检测结果。

   type DetectResult struct {
	  UserID string `json:"user_id"`
	  Symbol string `json:"symbol"`
	  Drifted        bool            `json:"drifted"`
	   DisplayedRatio decimal.Decimal `json:"displayed_ratio"`
	   ActualRatio    decimal.Decimal `json:"actual_ratio"`
	   Delta          decimal.Decimal `json:"delta"`
	  CaseID string `json:"case_id,omitempty"`
   }

   // Detect 对指定 Pair 执行一次漂移检测。
  // 若发现漂移且当前无 open 案例，自动开立 RatioDriftCase。

  func (d *DriftDetector) Detect(ctx context.Context, userID, symbol string) (DetectResult, error) {
	  userID = strings.TrimSpace(userID)
	   symbol = strings.ToUpper(strings.TrimSpace(symbol))
	  if userID == "" || symbol == "" {
		  return DetectResult{}, fmt.Errorf("%w: userID and symbol required", ErrInvalidInput)
	   }
	  // 读展示比率（来自 isobook）
	   pairs := d.book.List(userID,
		   symbol)
	   displayedRatio := decimal.Zero
	  for _, p := range pairs {
		  if p.Symbol == symbol {
			   r, _ := decimal.NewFromString(p.DebtRatio)
			  displayedRatio = r
			  break
		  }
	  }
	  // 读实际比率（来自最新世代）
	  gen, err := d.store.GetGeneration(ctx, userID, symbol)
	  if err != nil {
		  return DetectResult{}, fmt.Errorf("isodebt: get generation: %w", err)
	   }
	  actualRatio := gen.DebtRatio
	   delta := actualRatio.Sub(displayedRatio).Abs()
	  res := DetectResult{
		  UserID: userID,
		   Symbol: symbol,
		   DisplayedRatio: displayedRatio,
		  ActualRatio: actualRatio,
		   Delta:       delta,
	  }
	  if delta.LessThan(d.cfg.DriftThreshold) {
		   return res, nil
	   }
	  // 发现漂移：检查是否已有 open 案例
	   openCases, err := d.store.ListOpenDriftCases(ctx, userID, symbol)
	  if err != nil {
		  return res, fmt.Errorf("isodebt: list drift cases: %w", err)
	  }
	  if len(openCases) > 0 {
		   res.Drifted = true
		   res.CaseID = openCases[0].ID
		   return res, nil
	   }
	   // 开立新案例
	   now := d.clock()
	   dc := RatioDriftCase{
		   ID:     uuid.NewString(),
		  UserID: userID,
		   Symbol: symbol,
		  DisplayedRatio: displayedRatio,
		   ActualDebt: gen.TotalDebt,
		  ActualEquity: gen.TotalEquity,
		  ActualRatio:  actualRatio,
		  Generation: gen.Generation,
		   State:      DriftStateOpen,
		  DetectedAt: now,
	   }
	   if err := d.store.SaveDriftCase(ctx, dc); err != nil {
		  return res, fmt.Errorf("isodebt: save drift case: %w", err)
	  }
	   res.Drifted = true
	  res.CaseID = dc.ID
	   return res, nil
   }

// ─────────────────────────────────────────────
// DriftProofManager
// ─────────────────────────────────────────────
// DriftProofManager 管理漂移证明的提交与四眼确认。

type DriftProofManager struct {
	store Store
	clock func() time.Time
}

// NewDriftProofManager 构造证明管理器。

  func NewDriftProofManager(store Store) *DriftProofManager {
	  return &DriftProofManager{
		   store: store,
		   clock: func() time.Time { return time.Now().UTC() },
	   }
   }

// WithClock 注入时钟。

  func (pm *DriftProofManager) WithClock(c func() time.Time) *DriftProofManager {
	   if pm != nil && c != nil {
		  pm.clock = c
	   }
	   return pm
  }

  // SubmitProof 提交漂移证明（open → proven）。

   func (pm *DriftProofManager) SubmitProof(ctx context.Context, driftCaseID, submittedBy, summary, evidence string) (RatioDriftProof, error) {
	   dc, err := pm.store.GetDriftCase(ctx, driftCaseID)
	  if err != nil {
		  return RatioDriftProof{}, fmt.Errorf("isodebt: get drift case: %w", err)
	   }
	  if dc.State != DriftStateOpen {
		  return RatioDriftProof{}, fmt.Errorf("isodebt: drift case %s state=%s, expect open", driftCaseID, dc.State)
	  }
	   now := pm.clock()
	   proof := RatioDriftProof{
		  ID:          uuid.NewString(),
		  DriftCaseID: driftCaseID,
		  SubmittedBy: submittedBy,
		   Summary:     summary,
		   Evidence:    evidence,
		   SubmittedAt: now,
	   }
	   if err := pm.store.SaveProof(ctx, proof); err != nil {
		  return RatioDriftProof{}, fmt.Errorf("isodebt: save proof: %w", err)
	  }
	   // 推进案例状态
	   dc.State = DriftStateProven
	  dc.ProvenAt = &now
	  dc.ProofID = proof.ID
	  dc.ProofSummary = summary
	  if err := pm.store.SaveDriftCase(ctx, dc); err != nil {
		   return proof, fmt.Errorf("isodebt: update drift case state: %w", err)
	  }
	  return proof, nil
  }

// ConfirmClose 四眼确认关闭漂移案例（proven → closed）。
// 关闭后 TransferInEnabled 可恢复。
   func (pm *DriftProofManager) ConfirmClose(ctx context.Context, driftCaseID, confirmedBy string) (RatioDriftCase, error) {
	  dc,
		  err := pm.store.GetDriftCase(ctx, driftCaseID)
	  if err != nil {
		  return RatioDriftCase{}, fmt.Errorf("isodebt: get drift case: %w", err)
	  }
	  if dc.State != DriftStateProven {
		  return dc, fmt.Errorf("isodebt: drift case %s state=%s, expect proven for close", driftCaseID, dc.State)
	  }
	   now := pm.clock()
	   dc.State = DriftStateClosed
	  dc.ClosedAt = &now
	  dc.ClosedBy = confirmedBy
	  // 更新证明的 ConfirmedBy
	   if dc.ProofID != "" {
		   proof, err2 := pm.store.GetProof(ctx, dc.ProofID)
		   if err2 == nil {
			  proof.ConfirmedBy = confirmedBy
			  proof.ConfirmedAt = &now
			   _ = pm.store.SaveProof(ctx, proof)
		   }
	   }
	  if err := pm.store.SaveDriftCase(ctx, dc); err != nil {
		   return dc, fmt.Errorf("isodebt: update drift case close: %w", err)
	   }
	  return dc, nil
   }

// ExportProof 导出漂移证明摘要（供只读消费）。

  func (pm *DriftProofManager) ExportProof(ctx context.Context, proofID string) (RatioDriftProof, error) {
	  return pm.store.GetProof(ctx,
		  proofID)
  }

// IsDriftBlocking 返回指定 Pair 是否因漂移案例阻断 TransferIn。

  func IsDriftBlocking(ctx context.Context, store Store, userID, symbol string) (bool, string, error) {
	  cases, err := store.ListOpenDriftCases(ctx, userID, symbol)
	  if err != nil {
		   return false, "", err
	  }
	   if len(cases) == 0 {
		  return false, "", nil
	  }
	   return true, cases[0].ID, nil
   }
