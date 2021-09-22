package margin

import (
	"fmt"
)

// Verdict 阈值判定。等号策略：equity == required → OK（恰在阈值不触发）。

type Verdict string

const (
	VerdictOK Verdict = "OK"
	VerdictBreach Verdict = "Breach"
)

// MarginRatio = equity / maintenanceRequired。required 为 0 时视为无限大（无维持占用）。
   func MarginRatio(equity, required string) (string, error) {
	   eq, err := parseNonNeg(equity, "equity")
	  if err != nil {
		   return "", err
	  }
	  req, err := parseNonNeg(required, "required")
	   if err != nil {
		   return "", err
	  }
	  if req.IsZero() {
		  return "Inf", nil
	  }
	   return eq.Div(req).String(), nil
  }

// CompareThreshold 等号策略：ratio < 1 为 Breach；ratio >= 1 为 OK。
   func CompareThreshold(equity, required string) (Verdict, error) {
	   eq, err := parseNonNeg(equity, "equity")
	   if err != nil {
		   return "", err
	   }
	   req, err := parseNonNeg(required, "required")
	  if err != nil {
		  return "", err
	  }
	  if req.IsZero() {
		  return VerdictOK, nil
	  }
	  if eq.LessThan(req) {
		   return VerdictBreach, nil
	  }
	   return VerdictOK, nil
  }

// EvaluateAccount 用账户字段与注入价格做组合维持校验。
   func EvaluateAccount(acc Account, prices map[string]string, fallback Params) (Verdict, string, string, error) {
	  params := fallback
	   if acc.MaintenanceRateBPS > 0 {
		  params.MaintenanceRateBPS = acc.MaintenanceRateBPS
	   }
	   if acc.MaintenanceAmount != "" {
		   params.MaintenanceAmount = acc.MaintenanceAmount
	  }
	  if params.MaintenanceRateBPS <= 0 && params.MaintenanceAmount == "" && !hasPosThreshold(acc.Positions) {
		  return "", "", "", fmt.Errorf("maintenance threshold missing on params and account")
	   }
	   req, err := CombinedMaintenance(acc.Positions, prices, params)
	  if err != nil {
		  return "", "", "", err
	   }
	   v, err := CompareThreshold(acc.Equity, req)
	  if err != nil {
		  return "", "", "", err
	  }
	   ratio, err := MarginRatio(acc.Equity, req)
	  if err != nil {
		  return "", "", "", err
	   }
	  return v, req, ratio, nil
   }

  func hasPosThreshold(positions []Position) bool {
	   for _, p := range positions {
		   if p.MaintenanceRateBPS > 0 || p.MaintenanceAmount != "" {
			  return true
		  }
	   }
	  return false
  }
