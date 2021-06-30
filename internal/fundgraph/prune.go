package fundgraph

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"
)

// PruneItem 是一条待标记的冷边或归档候选。标记不删除边。

type PruneItem struct {
	EdgeID     string    `json:"edge_id"`
	Tier       string    `json:"tier"`
	OccurredAt time.Time `json:"occurred_at"`
	Ref        string    `json:"ref"`
}

// PrunePlan 是一轮保留计划。Mode 为 mark 时只写 tier；delete 需要显式打开配置并带确认口令。
type PrunePlan struct {
	At   time.Time `json:"at"`
	Mode string    `json:"mode"`
	Mark    []PruneItem `json:"mark"`
	Delete  []string    `json:"delete,omitempty"`
	Confirm string      `json:"confirm,omitempty"`
}

// PruneResult 记录实际标记和删除的边数。

type PruneResult struct {
	Marked  int `json:"marked"`
	Deleted int `json:"deleted"`
}

// ConfirmToken 由待删除边主键算出。口令不匹配则拒绝删除。
  func ConfirmToken(ids []string) string {
	  cp := append([]string{}, ids...)
	   sort.Strings(cp)
	  sum := sha256.Sum256([]byte(strings.Join(cp, ",")))
	  return hex.EncodeToString(sum[:])
   }

// PlanPrune 按发生时间划分热边、冷边和归档候选。已是目标标记的边不再进入计划。

  func (s *Service) PlanPrune(ctx context.Context) (PrunePlan, error) {
	   if s == nil {
		   return PrunePlan{}, ErrInvalid
	   }
	   edges,
		   err := s.store.ListEdges(ctx, EdgeFilter{})
	  if err != nil {
		  return PrunePlan{}, err
	  }
	   now := s.clock()
	  plan := PrunePlan{At: now, Mode: "mark", Mark: []PruneItem{}}
	   for _, e := range edges {
		  if e.OccurredAt.IsZero() {
			  continue
		   }
		  age := now.Sub(e.OccurredAt)
		  tier := ""
		   switch {
		   case age >= s.cfg.ColdAge:
			  tier = TierArchive
		  case age >= s.cfg.HotAge:
			  tier = TierCold
		  default:
			  tier = TierHot
		   }
		  current := ""
		   if e.Attrs != nil {
			  current = e.Attrs["tier"]
		  }
		   if tier == TierHot || current == tier {
			   continue
		   }
		   plan.Mark = append(plan.Mark, PruneItem{
			  EdgeID: e.ID,
			   Tier:       tier,
			   OccurredAt: e.OccurredAt,
			  Ref:        e.Ref,
		  })
	  }
	  sort.Slice(plan.Mark, func(i, j int) bool { return plan.Mark[i].EdgeID < plan.Mark[j].EdgeID })
	   return plan, nil
  }

// ApplyPrune 执行标记。只有 Mode=delete、配置允许且口令一致时才物理删除。
// 删除条件不满足时整单不写，避免先改冷热标记再拒绝删除。
  func (s *Service) ApplyPrune(ctx context.Context, plan PrunePlan) (PruneResult, error) {
	  if s == nil {
		  return PruneResult{}, ErrInvalid
	  }
	   modeDelete := strings.EqualFold(strings.TrimSpace(plan.Mode), "delete")
	   if modeDelete {
		  if !s.cfg.AllowDelete {
			   return PruneResult{}, ErrDeleteDisabled
		   }
		   if len(plan.Delete) == 0 {
			   return PruneResult{}, ErrInvalid
		   }
		  if plan.Confirm != ConfirmToken(plan.Delete) {
			  return PruneResult{}, ErrConfirm
		  }
	  } else if len(plan.Delete) > 0 {
		   return PruneResult{}, ErrDeleteDisabled
	   }
	  for _, item := range plan.Mark {
		  if item.EdgeID == "" || (item.Tier != TierCold && item.Tier != TierArchive && item.Tier != TierHot) {
			  return PruneResult{}, ErrInvalid
		   }
	  }
	  var res PruneResult
	  now := s.clock()
	   for _, item := range plan.Mark {
		   if err := s.store.SetEdgeTier(ctx,
			   item.EdgeID, item.Tier, now); err != nil {
			  return res, err
		   }
		  res.Marked++
	  }
	   if modeDelete {
		   if err := s.store.DeleteEdges(ctx, plan.Delete); err != nil {
			  return res, err
		   }
		  res.Deleted = len(plan.Delete)
	  }
	   return res, nil
   }
