package utabook

import (
	"strings"
)

type BorrowInfoResponse struct {
	Currency string     `json:"currency"`
	Tiers    []RateTier `json:"tiers"`
	Limits InterestLimit `json:"limits"`
}

  func (b *Book) BorrowInfoResponse(currency string) BorrowInfoResponse {
	  currency = strings.ToUpper(strings.TrimSpace(currency))
	  return BorrowInfoResponse{
		  Currency: currency,
		  Tiers:    b.ListRateTiers(currency),
		   Limits: b.InterestLimits(currency),
	   }
   }

  func (b *Book) LiabilityREST(userID, currency, marginMode, symbol string) []map[string]string {
	   rows := b.Liabilities(userID, currency, marginMode, symbol)
	   out := make([]map[string]string, 0, len(rows))
	  for _, r := range rows {
		   out = append(out, map[string]string{
			   "currency": r.Currency, "marginMode": r.MarginMode, "symbol": r.Symbol,
			   "principal": r.Principal, "interest": r.Interest, "accruedInterest": r.AccruedInterest,
			  "totalLiability": r.TotalLiability,
			   "borrowEnabled": boolStr(r.BorrowEnabled),
		   })
	   }
	   return out
   }

  func boolStr(v bool) string {
	  if v {
		  return "true"
	   }
	   return "false"
   }

  func (b *Book) RiskREST(userID string) map[string]string {
	   v := b.Risk(userID)
	   return map[string]string{
		   "userId": v.UserID, "marginLevel": v.MarginLevel, "totalEquity": v.TotalEquity,
		   "totalLiability": v.TotalLiability, "totalCollateral": v.TotalCollateral,
		  "riskGrade": v.RiskGrade, "crossDebtRatio": v.CrossDebtRatio,
	   }
  }

   func (b *Book) CrossREST(userID, currency string) []map[string]string {
	  rows := b.CrossMargin(userID, currency)
	   out := make([]map[string]string, 0, len(rows))
	  for _, r := range rows {
		  out = append(out, map[string]string{
			  "currency": r.Currency, "total": r.Total, "available": r.Available, "hold": r.Hold,
			   "liability": r.Liability, "maxBorrow": r.MaxBorrow,
			  "borrowEnabled": boolStr(r.BorrowEnabled), "transferEnabled": boolStr(r.TransferEnabled),
		  })
	   }
	   return out
  }

  func NormalizeMarginMode(mode string) string {
	  mode = strings.ToUpper(strings.TrimSpace(mode))
	  switch mode {
	  case "CROSS_MARGIN":
		   return "CROSS"
	   case "ISOLATED_MARGIN":
		   return "ISOLATED"
	  default:
		  return mode
	   }
   }

  func (b *Book) ParseRepayQuery(currency, marginMode, symbol, page, pageSize, startAt, endAt string) RepayQuery {
	  return RepayQuery{
		  Currency: strings.ToUpper(strings.TrimSpace(currency)), MarginMode: NormalizeMarginMode(marginMode), Symbol: strings.ToUpper(strings.TrimSpace(symbol)),
		   StartMs: parseMs(startAt), EndMs: parseMs(endAt),
		   Page: parseInt(page, 1), PageSize: parseInt(pageSize, 50),
	   }
   }

   func parseMs(raw string) int64 {
	  raw = strings.TrimSpace(raw)
	  if raw == "" {
		  return 0
	   }
	   var n int64
	   for _, ch := range raw {
		   if ch < '0' || ch > '9' {
			   return 0
		  }
		  n = n*10 + int64(ch-'0')
	  }
	  return n
  }

   func parseInt(raw string, def int) int {
	  raw = strings.TrimSpace(raw)
	  if raw == "" {
		  return def
	  }
	  n := 0
	   for _, ch := range raw {
		  if ch < '0' || ch > '9' {
			  return def
		   }
		   n = n*10 + int(ch-'0')
	  }
	   if n <= 0 {
		   return def
	  }
	   return n
  }
