package recon

import (
	"strings"
)

// stampDrift 补类别、容忍和严重级别，并写回备注。
  func stampDrift(d *TripDrift) {
	  if d == nil {
		  return
	   }
	   class, tol := toleranceText(d.Asset)
	  if d.Asset == "*" {
		  class = "all"
		   tol = "0"
	   }
	   d.Class = class
	  d.Tolerance = tol
	  d.Severity = severityOf(d.Kind)
	  if d.Note != "" && !strings.Contains(d.Note, " class=") {
		   d.Note = d.Note + " class=" + class + " tol=" + tol + " sev=" + d.Severity
	   }
   }

  func severityOf(kind string) string {
	  switch kind {
	  case tripUnread,
		   tripHotBelowUser, tripTransitAged:
		  return "critical"
	   case tripBridgeInflate, tripUnexplained:
		   return "high"
	  default:
		   return "high"
	  }
  }

   func decorateDrifts(rep *TripReport) {
	  if rep == nil {
		   return
	  }
	   for i := range rep.Drifts {
		  stampDrift(&rep.Drifts[i])
	  }
	   for i := range rep.Legs {
		   for j := range rep.Legs[i].Drifts {
			   stampDrift(&rep.Legs[i].Drifts[j])
		  }
	  }
  }

// blockSummary 平台挡冻原因。取最严重的一条，避免备注被长列表淹没。

   func blockSummary(drifts []TripDrift) string {
	  if len(drifts) == 0 {
		   return "triparty drift"
	   }
	  best := drifts[0]
	  rank := severityRank(best.Severity)
	   for _, d := range drifts[1:] {
		   if severityRank(d.Severity) > rank {
			  best = d
			   rank = severityRank(d.Severity)
		  }
	  }
	   if best.Note != "" {
		  return best.Note
	   }
	  return "triparty:" + best.Kind
   }

  func severityRank(s string) int {
	   switch s {
	  case "critical":
		  return 3
	   case "high":
		   return 2
	  default:
		  return 1
	   }
   }
