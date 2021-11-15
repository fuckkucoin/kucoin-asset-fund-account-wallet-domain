package solvency

import "strings"

const (
	legacyPOR           = "engineering merkle over book rows; not a public reserve attestation"
	legacyPORDeprecated = "deprecated: engineering merkle over book rows; not a solvency attestation"
	legacyBal           = "signed book snapshot for query-gateway; not a public reserve attestation"
	legacyBalDeprecated = "deprecated: signed book snapshot for query-gateway; not a solvency attestation"
	legacyObs  = "in-process fund-safety aggregation; not a public reserve attestation"
	legacyObs2 = "aggregation only; not a public reserve attestation"
)

// LegacyClass 旧接口的归类。solvency 为 false 时调用方不得发布、不得当作用户偿付路径。

type LegacyClass struct {
	Disclaimer string `json:"disclaimer"`
	Class      string `json:"class"`
	Solvency bool `json:"solvency"`
	Deprecated bool `json:"deprecated"`
	Reason string `json:"reason"`
}

// ClassifyDisclaimer 识别工程默克尔、余额签名快照与偿付证明文案。
// 未识别的文案按非偿付处理，避免把陌生免责声明抬成偿付根。

   func ClassifyDisclaimer(disclaimer string) LegacyClass {
	   d := strings.TrimSpace(disclaimer)
	   out := LegacyClass{Disclaimer: d, Class: ClassEngineering, Solvency: false, Deprecated: true}
	   switch d {
	   case legacyPOR,
		   legacyPORDeprecated:
		  out.Reason = "por engineering merkle is deprecated and is not a solvency attestation"
		   return out
	  case legacyBal, legacyBalDeprecated:
		   out.Reason = "signed book snapshot is deprecated and is not a solvency attestation"
		   return out
	   case legacyObs, legacyObs2:
		  out.Reason = "in-process aggregation is not a solvency attestation"
		  return out
	  case DisclaimerInternal:
		   out.Class = ClassInternal
		   out.Solvency = true
		   out.Deprecated = false
		  out.Reason = "internal solvency attestation"
		   return out
	  case DisclaimerPublishable:
		   out.Class = ClassPublishable
		   out.Solvency = true
		   out.Deprecated = false
		   out.Reason = "publishable solvency attestation"
		  return out
	   default:
		   if strings.Contains(strings.ToLower(d), "not a public reserve") || strings.Contains(strings.ToLower(d), "not a solvency") {
			   out.Reason = "disclaimer explicitly refuses solvency"
			  return out
		   }
		  out.Reason = "unknown disclaimer is not accepted as solvency"
		   return out
	   }
   }

// AcceptsAsSolvency 同时看文案、类别和偿付标记。三者缺一则拒绝。
  func AcceptsAsSolvency(disclaimer, class string, solvency bool) bool {
	  if !solvency {
		  return false
	   }
	   if class != ClassInternal && class != ClassPublishable {
		   return false
	   }
	  got := ClassifyDisclaimer(disclaimer)
	  if !got.Solvency || got.Deprecated || got.Class != class {
		  return false
	  }
	  return true
   }
