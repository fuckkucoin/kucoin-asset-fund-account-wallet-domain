package liqplan

import (
	"fmt"
	"time"
)

// ─────────────────────────────────────────────
// SeizeExecutor：外部扣押动作接口（不含撮合）
// ─────────────────────────────────────────────
// SeizeExecutor 执行一次实际扣押动作，返回已成功扣押金额。
// 实现者负责原子性：部分失败须返回已扣押金额（可为 "0"）与错误。
// 本接口不实施撮合盘口；仅操作 Hold/Borrow 保证金科目。

type SeizeExecutor interface {
	Execute(leg SeizeLeg) (seizedAmount string,
		err error)
}

// ─────────────────────────────────────────────
// LTVScanReport
// ─────────────────────────────────────────────
// LTVScanReport 一轮 LTV 扫描的汇报。

type LTVScanReport struct {
	Checked int `json:"checked"`
	Breached int `json:"breached"`
	Planned  int `json:"planned"`
	Skipped  int `json:"skipped"` // 已有活跃计划
	Errs int `json:"errors"`
}

// ─────────────────────────────────────────────
// LTVScanWorker
// ─────────────────────────────────────────────
// LTVScanWorker 周期扫描 LTV，穿仓时触发 TriggerPlan。
// BorrowLister 为迭代所有开仓借款的只读接口，由 marginhook 实现。

type LTVScanWorker struct {
	svc          *Service
	ltvReader    LTVReader
	borrowLister BorrowLister
	clock func() time.Time
}

// BorrowLister 迭代所有开仓借款（只读）。

type BorrowLister interface {
	ListOpenBorrows() []BorrowSnapshot
}

// BorrowSnapshot 借款只读快照。
type BorrowSnapshot struct {
	BorrowID    string
	UserID      string
	BorrowAsset string
	BorrowAmount string
	CollateralID     string
	CollateralAsset  string
	CollateralAmount string
	LTVBPS int
	CapBPS int
}

   func NewLTVScanWorker(svc *Service, ltvReader LTVReader, bl BorrowLister) *LTVScanWorker {
	   return &LTVScanWorker{
		   svc:       svc,
		   ltvReader: ltvReader,
		   borrowLister: bl,
		  clock: func() time.Time { return time.Now().UTC() },
	  }
   }

// RunOnce 执行一轮 LTV 扫描。

   func (w *LTVScanWorker) RunOnce() LTVScanReport {
	  var rpt LTVScanReport
	   if w == nil || w.borrowLister == nil {
		   return rpt
	   }
	  borrows := w.borrowLister.ListOpenBorrows()
	   for _, b := range borrows {
		  rpt.Checked++
		  ltvBPS := b.LTVBPS
		  capBPS := b.CapBPS
		   if w.ltvReader != nil {
			  live, cap, err := w.ltvReader.CurrentLTV(b.BorrowID)
			  if err != nil {
				  rpt.Errs++
				  continue
			   }
			   ltvBPS = live
			   capBPS = cap
		   }
		  if ltvBPS <= capBPS {
			  continue
		   }
		  rpt.Breached++
		   idemKey := "ltv-scan:" + b.BorrowID
		   _, err := w.svc.TriggerPlan(TriggerReq{
			  BorrowID: b.BorrowID,
			   IdempotencyKey: idemKey,
			   UserID:       b.UserID,
			   BorrowAsset:  b.BorrowAsset,
			   BorrowAmount: b.BorrowAmount,
			   CollateralID: b.CollateralID,
			   CollateralAsset:  b.CollateralAsset,
			   CollateralAmount: b.CollateralAmount,
			  TriggerLTVBPS: ltvBPS,
			   CapBPS:        capBPS,
		  })
		   if err == ErrAlreadyExists {
			  rpt.Skipped++
			   continue
		  }
		   if err != nil {
			   rpt.Errs++
			  continue
		   }
		  rpt.Planned++
	  }
	  return rpt
  }

// ─────────────────────────────────────────────
// SeizeExecuteWorker
// ─────────────────────────────────────────────
// SeizeExecuteReport 一轮扣押执行汇报。
type SeizeExecuteReport struct {
	Processed int `json:"processed"`
	Succeeded int `json:"succeeded"`
	Failed    int `json:"failed"`
	Errs      int `json:"errors"`
}

// SeizeExecuteWorker 从 seizing 计划中取 pending/failed 腿执行扣押。
type SeizeExecuteWorker struct {
	svc *Service
	executor SeizeExecutor
}

   func NewSeizeExecuteWorker(svc *Service, executor SeizeExecutor) *SeizeExecuteWorker {
	  return &SeizeExecuteWorker{svc: svc, executor: executor}
  }

// RunOnce 执行一轮扣押（pending 腿先，然后 failed 且未超重试上限）。

   func (w *SeizeExecuteWorker) RunOnce() SeizeExecuteReport {
	  var rpt SeizeExecuteReport
	  if w == nil || w.executor == nil {
		  return rpt
	  }
	   plans := w.svc.ListPlans(string(PlanSeizing))
	   plans = append(plans, w.svc.ListPlans(string(PlanTriggered))...)
	  for _, plan := range plans {
		  legs := w.svc.ListLegs(plan.ID)
		  for _, leg := range legs {
			   if leg.State != LegPending && leg.State != LegFailed {
				   continue
			   }
			  if leg.State == LegFailed && leg.RetryCount >= leg.MaxRetry {
				   continue
			  }
			  rpt.Processed++
			   if leg.State == LegPending {
				  started,
					   err := w.svc.StartLeg(leg.ID)
				  if err != nil {
					   rpt.Errs++
					  continue
				   }
				  leg = started
			  } else {
				  resumed, err := w.svc.ResumeLeg(leg.ID)
				   if err != nil {
					  rpt.Errs++
					  continue
				   }
				  leg = resumed
			  }
			   seizedAmount, execErr := w.executor.Execute(leg)
			   if execErr != nil {
				  partialAmt := seizedAmount
				   if partialAmt == "" {
					  partialAmt = "0"
				  }
				  _, _, err := w.svc.FailLeg(leg.ID, partialAmt, execErr.Error())
				   if err != nil {
					   rpt.Errs++
				  }
				  rpt.Failed++
				  continue
			   }
			  _,
				   _, err := w.svc.CompleteLeg(leg.ID, seizedAmount)
			   if err != nil {
				   rpt.Errs++
				  continue
			  }
			   rpt.Succeeded++
		  }
	   }
	   return rpt
   }

// ─────────────────────────────────────────────
// ResiduePatrolWorker
// ─────────────────────────────────────────────
// ResiduePatrolReport 一轮残留巡检汇报。
type ResiduePatrolReport struct {
	PlansScanned int `json:"plans_scanned"`
	LegsWithResidue int `json:"legs_with_residue"`
	AutoResumed     int `json:"auto_resumed"`
	Suspended       int `json:"suspended"`
	Alerted         int `json:"alerted"`
	Errs int `json:"errors"`
}

// ResidueAlerter 残留告警接口（可接入告警系统）。
type ResidueAlerter interface {
	Alert(planID, legID, reason string) error
}

// ResiduePatrolWorker 巡检 residued 计划，对 failed 腿自动续跑或告警。

type ResiduePatrolWorker struct {
	svc      *Service
	executor SeizeExecutor
	alerter      ResidueAlerter
	maxAutoRetry int
}

   func NewResiduePatrolWorker(svc *Service, executor SeizeExecutor, alerter ResidueAlerter) *ResiduePatrolWorker {
	   return &ResiduePatrolWorker{
		  svc:          svc,
		   executor:     executor,
		  alerter:      alerter,
		  maxAutoRetry: 3,
	  }
  }

// RunOnce 执行一轮残留巡检。

  func (w *ResiduePatrolWorker) RunOnce() ResiduePatrolReport {
	  var rpt ResiduePatrolReport
	  if w == nil {
		  return rpt
	   }
	   plans := w.svc.ListPlans(string(PlanResidued))
	  rpt.PlansScanned = len(plans)
	   for _, plan := range plans {
		  legs := w.svc.ListLegs(plan.ID)
		   for _, leg := range legs {
			  if !leg.HasResidue() {
				  continue
			  }
			   rpt.LegsWithResidue++
			  if leg.RetryCount < w.maxAutoRetry && w.executor != nil {
				   resumed, err := w.svc.ResumeLeg(leg.ID)
				   if err != nil {
					  rpt.Errs++
					  continue
				   }
				  seizedAmount, execErr := w.executor.Execute(resumed)
				   if execErr != nil {
					   partialAmt := seizedAmount
					   if partialAmt == "" {
						  partialAmt = "0"
					   }
					   _, _, err2 := w.svc.FailLeg(resumed.ID, partialAmt, execErr.Error())
					   if err2 != nil {
						  rpt.Errs++
					   }
					   if w.alerter != nil {
						   _ = w.alerter.Alert(plan.ID, resumed.ID, fmt.Sprintf("auto-resume failed: %v", execErr))
						   rpt.Alerted++
					  }
					  continue
				   }
				  _,
					  _, err2 := w.svc.CompleteLeg(resumed.ID, seizedAmount)
				  if err2 != nil {
					   rpt.Errs++
					  continue
				  }
				  rpt.AutoResumed++
				  continue
			  }
			   // 超重试上限：挂账并告警
			  if leg.State == LegFailed {
				   _, err := w.svc.SuspendLeg(leg.ID, "residue-patrol: max-retry exceeded")
				   if err != nil {
					   rpt.Errs++
				   } else {
					   rpt.Suspended++
				  }
			  }
			  if w.alerter != nil {
				   _ = w.alerter.Alert(plan.ID, leg.ID, "residue requires manual intervention")
				  rpt.Alerted++
			   }
		  }
	   }
	   return rpt
   }

// ─────────────────────────────────────────────
// MarkGenValidateWorker
// ─────────────────────────────────────────────
// MarkGenValidateReport 一轮标价世代校验汇报。

type MarkGenValidateReport struct {
	PlansChecked int `json:"plans_checked"`
	StaleFound int `json:"stale_found"`
	Blocked    int `json:"blocked"`
	Errs       int `json:"errors"`
}

// MarkGenValidateWorker 检查 seizing/settling 计划绑定的标价世代是否仍有效。
// 若标价过期/中毒，计划不得再新增腿（fail-closed）；现有进行中的腿不强制终止。
type MarkGenValidateWorker struct {
	svc   *Service
	clock func() time.Time
}

  func NewMarkGenValidateWorker(svc *Service) *MarkGenValidateWorker {
	  return &MarkGenValidateWorker{
		  svc:   svc,
		  clock: func() time.Time { return time.Now().UTC() },
	  }
  }

// RunOnce 执行一轮标价世代校验。
  func (w *MarkGenValidateWorker) RunOnce() MarkGenValidateReport {
	  var rpt MarkGenValidateReport
	  if w == nil {
		  return rpt
	  }
	  states := []PlanState{PlanTriggered, PlanSeizing, PlanSettling}
	   for _, st := range states {
		   plans := w.svc.ListPlans(string(st))
		   for _, plan := range plans {
			  rpt.PlansChecked++
			  if plan.MarkGateID == "" {
				  continue
			   }
			   g, ok := w.svc.marks.Get(plan.MarkGateID)
			   if !ok {
				  rpt.StaleFound++
				  rpt.Blocked++
				   continue
			  }
			   if g.IsStale(w.clock()) {
				  rpt.StaleFound++
				  rpt.Blocked++
			  }
		  }
	   }
	  return rpt
   }

// CanAddLeg 检查计划的标价世代是否仍有效，用于在 AddSeizeLeg 前调用（fail-closed 闸）。

  func (w *MarkGenValidateWorker) CanAddLeg(planID string) error {
	  plan, err := w.svc.GetPlan(planID)
	  if err != nil {
		  return err
	   }
	   if plan.MarkGateID == "" {
		   return nil
	   }
	  g, ok := w.svc.marks.Get(plan.MarkGateID)
	   if !ok {
		  return ErrMarkNotFound
	  }
	   if g.IsStale(w.clock()) {
		  return ErrMarkStale
	   }
	  return nil
   }
