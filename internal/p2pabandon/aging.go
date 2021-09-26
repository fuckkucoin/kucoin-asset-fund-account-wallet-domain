package p2pabandon

import (
	"time"
)

// AgingPatrolResult 单次超龄巡检结果汇总。
type AgingPatrolResult struct {
	Inspected int // 检查的计划数
	NormalCnt int // 正常数量
	WarningCnt int // 预警数量
	EscCnt int // 新升级 escalated 数量
	SkippedCnt int // 已处于终态跳过数量
	Errors     []string
}

// PatrolAging 执行一次超龄巡检。
// holdBirthFn 注入：给定 plan_id，返回 hold 的诞生时间（调用方从 holdcat 只读查询）。
// 若 holdBirthFn 返回 zero time 或 error，跳过该计划并记录 error。
   func (s *Service) PatrolAging(holdBirthFn func(planID, holdID string) (time.Time, error)) AgingPatrolResult {
	  if s == nil || holdBirthFn == nil {
		   return AgingPatrolResult{}
	   }
	  now := s.now()
	   plans := s.ListAllPlans()
	  result := AgingPatrolResult{}
	  for _, plan := range plans {
		  result.Inspected++
		  // 已处于终态的计划跳过超龄检查。
		   if plan.Status.Terminal() {
			   result.SkippedCnt++
			   continue
		  }
		  birth,
			  err := holdBirthFn(plan.ID, plan.HoldID)
		  if err != nil || birth.IsZero() {
			   result.Errors = append(result.Errors, "plan "+plan.ID+": hold birth query failed: "+errString(err))
			  continue
		  }
		  holdAgeSecs := int64(now.Sub(birth).Seconds())
		   if holdAgeSecs < 0 {
			  holdAgeSecs = 0
		  }
		  slaSecs := plan.SLAEscalateSecs
		  if slaSecs <= 0 {
			   slaSecs = s.sla.SLAEscalateSecs
		  }
		  age, err := s.UpsertAge(plan.ID, holdAgeSecs, slaSecs)
		  if err != nil {
			   result.Errors = append(result.Errors, "plan "+plan.ID+": upsert age failed: "+errString(err))
			   continue
		  }
		  switch age.Stage {
		  case AgeStageNormal:
			   result.NormalCnt++
		  case AgeStageWarning:
			   result.WarningCnt++
		   case AgeStageEscalated:
			   // 若此次刚刚升级（EscalatedAt 为本轮 now 附近），计入升级计数。
			  if age.EscalatedAt == nil {
				   // 自动触发升级。
				   _, escalErr := s.EscalateAge(EscalateAgeReq{
					   AgeID:      age.ID,
					  EscalateTo: "p2p-abandon-desk",
					   EscalateNote: "hold age exceeded SLA; auto-escalated by aging patrol",
				  })
				if escalErr == nil {
					result.EscCnt++
				} else {
					result.Errors = append(result.Errors, "plan "+plan.ID+": escalate age failed: "+errString(escalErr))
				}
			} else {
				// 已升级过，无需重复计入。
				result.EscCnt++
			}
		case AgeStageResolved:
			result.SkippedCnt++
		}
	}
	return result
}

// HoldAgeSummary 计划维度超龄摘要（API 响应）。

type HoldAgeSummary struct {
	PlanID string `json:"plan_id"`
	OrderID string `json:"order_id"`
	UserID  string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	HoldID string `json:"hold_id"`
	Stage       AgeStage `json:"stage"`
	HoldAgeSecs int64    `json:"hold_age_secs"`
	SLASecs     int64    `json:"sla_secs"`
	AgeID string `json:"age_id,omitempty"`
}

// GetHoldAgeSummary 获取计划维度超龄摘要（API 用）。

   func (s *Service) GetHoldAgeSummary(planID string) (HoldAgeSummary, error) {
	  if s == nil {
		   return HoldAgeSummary{}, ErrUnavailable
	  }
	  plan, err := s.GetPlan(planID)
	  if err != nil {
		   return HoldAgeSummary{}, err
	   }
	  sum := HoldAgeSummary{
		  PlanID:  plan.ID,
		  OrderID: plan.OrderID,
		   UserID:  plan.UserID,
		  Asset:   plan.Asset,
		   Amount:  plan.Amount,
		   HoldID: plan.HoldID,
		   SLASecs: plan.SLAEscalateSecs,
	  }
	  age, err := s.GetAgeByPlan(planID)
	  if err != nil {
		   // 尚无超龄记录，返回 normal。
		   sum.Stage = AgeStageNormal
		  return sum, nil
	   }
	  sum.AgeID = age.ID
	  sum.Stage = age.Stage
	   sum.HoldAgeSecs = age.HoldAgeSecs
	   return sum, nil
  }

   func errString(err error) string {
	  if err == nil {
		   return "<nil>"
	  }
	  return err.Error()
  }
