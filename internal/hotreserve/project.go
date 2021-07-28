package hotreserve

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"github.com/shopspring/decimal"
)

// ProjectInput 预测输入。空串拒绝，避免把缺失当成 0 放行。

type ProjectInput struct {
	HotAvail string
	Inflow   string
	Outflow   string
	SweepHold string
	InTransit string
	Haircut string
	Stress  string
	Buffer  string
}

// ProjectResult 点估、下界，以及下界里已经折过的入金与加压后的出金。
type ProjectResult struct {
	Point           string
	Lower           string
	InflowKept      string
	OutflowStressed string
}

// Project 复算窗口期末热可用。
// 做什么：点估 = 热可用 + 入金期望 − 出金期望 − 归集占用 − 广播在途。
// 下界对入金打折、对出金加压并再减缓冲；结果允许为负。
// 不变量：同一输入重复调用得到同一字符串。
  func Project(in ProjectInput) (ProjectResult, error) {
	   hot, err := parseNonNeg(in.HotAvail)
	   if err != nil {
		   return ProjectResult{}, err
	  }
	  inflow,
		  err := parseNonNeg(in.Inflow)
	  if err != nil {
		   return ProjectResult{}, err
	  }
	  outflow, err := parseNonNeg(in.Outflow)
	  if err != nil {
		   return ProjectResult{}, err
	  }
	  sweep, err := parseNonNeg(in.SweepHold)
	   if err != nil {
		   return ProjectResult{}, err
	   }
	  transit, err := parseNonNeg(in.InTransit)
	   if err != nil {
		  return ProjectResult{}, err
	   }
	  hair, err := parseNonNeg(in.Haircut)
	  if err != nil {
		   return ProjectResult{}, err
	  }
	  if hair.GreaterThanOrEqual(decimal.NewFromInt(1)) {
		   return ProjectResult{}, ErrAmount
	  }
	  stress, err := parseNonNeg(in.Stress)
	  if err != nil {
		  return ProjectResult{}, err
	  }
	   buf, err := parseNonNeg(in.Buffer)
	  if err != nil {
		   return ProjectResult{}, err
	   }
	  one := decimal.NewFromInt(1)
	  kept := inflow.Mul(one.Sub(hair))
	  stressed := outflow.Mul(one.Add(stress))
	  point := hot.Add(inflow).Sub(outflow).Sub(sweep).Sub(transit)
	   lower := hot.Add(kept).Sub(stressed).Sub(sweep).Sub(transit).Sub(buf)
	   return ProjectResult{
		  Point:           point.String(),
		  Lower:           lower.String(),
		  InflowKept:      kept.String(),
		   OutflowStressed: stressed.String(),
	   }, nil
   }

// Digest 输入摘要。归集计划编号排序后拼入，避免 map 迭代顺序把代次打乱。
   func Digest(asset string, in ProjectInput, windowSec int64, sampleN int, sweepIDs []string) string {
	  ids := append([]string(nil), sweepIDs...)
	  sort.Strings(ids)
	   h := sha256.New()
	  fmt.Fprintf(h, "%s|%s|%s|%s|%s|%s|%s|%s|%s|%d|%d|%s",
		  strings.ToUpper(strings.TrimSpace(asset)),
		   in.HotAvail, in.Inflow, in.Outflow, in.SweepHold, in.InTransit,
		  in.Haircut, in.Stress, in.Buffer, windowSec, sampleN, strings.Join(ids, ","))
	  return hex.EncodeToString(h.Sum(nil))
   }

// Short 判断保守下界扣除已预留与新增意图后是否为负。等于 0 放行。
// 不变量：只认小于 0；调用方不得把负下界改写成 0 再比较。
  func Short(lower, reserved, extra, intent decimal.Decimal) bool {
	   return lower.Sub(reserved).Sub(extra).Sub(intent).IsNegative()
   }
