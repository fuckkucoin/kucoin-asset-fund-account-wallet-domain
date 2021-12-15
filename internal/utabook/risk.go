package utabook

import (
	"context"
	"database/sql"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
	"github.com/shopspring/decimal"
)

type RiskView struct {
	UserID          string `json:"userId"`
	MarginLevel     string `json:"marginLevel"`
	TotalEquity     string `json:"totalEquity"`
	TotalLiability  string `json:"totalLiability"`
	TotalCollateral string `json:"totalCollateral"`
	RiskGrade string `json:"riskGrade"`
	CrossDebtRatio string `json:"crossDebtRatio"`
	UpdatedAt int64 `json:"updatedAt"`
}

   func (b *Book) UpsertRisk(userID string, view RiskView) (RiskView, error) {
	  if b == nil {
		  return view, nil
	  }
	  userID = strings.TrimSpace(userID)
	   view.UserID = userID
	  if view.MarginLevel == "" {
		   view.MarginLevel = "0"
	   }
	  if view.TotalEquity == "" {
		  view.TotalEquity = "0"
	  }
	   if view.TotalLiability == "" {
		   view.TotalLiability = "0"
	  }
	  if view.TotalCollateral == "" {
		  view.TotalCollateral = "0"
	  }
	   if view.RiskGrade == "" {
		  view.RiskGrade = gradeFromLevel(view.MarginLevel)
	   }
	  if view.CrossDebtRatio == "" {
		   view.CrossDebtRatio = ratioLiabilityEquity(view.TotalLiability, view.TotalEquity)
	   }
	  if view.UpdatedAt == 0 {
		  view.UpdatedAt = time.Now().UTC().UnixMilli()
	  }
	  if err := b.persistRisk(view); err != nil {
		  return RiskView{}, err
	  }
	  b.mu.Lock()
	  b.risk[userID] = view
	  b.mu.Unlock()
	   return view, nil
  }

  func gradeFromLevel(level string) string {
	  v, err := decimal.NewFromString(strings.TrimSpace(level))
	   if err != nil || !v.IsPositive() {
		  return "LOW"
	   }
	   if v.LessThan(decimal.NewFromFloat(1.1)) {
		  return "HIGH"
	  }
	  if v.LessThan(decimal.NewFromFloat(1.3)) {
		   return "MEDIUM"
	  }
	  return "LOW"
   }

  func ratioLiabilityEquity(liab, equity string) string {
	   l, err1 := decimal.NewFromString(strings.TrimSpace(liab))
	   e, err2 := decimal.NewFromString(strings.TrimSpace(equity))
	  if err1 != nil || err2 != nil || !e.IsPositive() {
		   return "0"
	   }
	   return l.Div(e).String()
   }

  func (b *Book) Risk(userID string) RiskView {
	  userID = strings.TrimSpace(userID)
	   if b == nil {
		  return RiskView{UserID: userID, MarginLevel: "0"}
	  }
	  if b.sql != nil && b.sql.DB != nil {
		   row, err := sqlrepo.NewFundingSubjects(b.sql).GetRisk(context.Background(), userID)
		  if err == nil {
			   return RiskView{
				   UserID: row.UserID, MarginLevel: row.MarginLevel, TotalEquity: row.TotalEquity,
				  TotalLiability: row.TotalLiability, TotalCollateral: row.TotalCollateral,
				  RiskGrade: row.RiskGrade, CrossDebtRatio: row.CrossDebtRatio, UpdatedAt: row.UpdatedAt,
			   }
		  }
		  if err != sql.ErrNoRows {
			  // 回落内存
		  }
	   }
	  b.mu.Lock()
	   v, ok := b.risk[userID]
	   b.mu.Unlock()
	  if ok {
		   return v
	   }
	   return RiskView{UserID: userID, MarginLevel: "0", TotalEquity: "0", TotalLiability: "0", TotalCollateral: "0", CrossDebtRatio: "0"}
   }

   func (b *Book) RecomputeRiskFromLiabilities(userID string, equityHint string) (RiskView, error) {
	  userID = strings.TrimSpace(userID)
	   liabTotal := b.LiabilityByCurrency(userID, "")
	  if liabTotal == "0" {
		   rows := b.Liabilities(userID, "", "", "")
		  sum := decimal.Zero
		  for _, r := range rows {
			  v, _ := decimal.NewFromString(strings.TrimSpace(r.TotalLiability))
			  sum = sum.Add(v)
		   }
		   liabTotal = sum.String()
	  }
	   eq := strings.TrimSpace(equityHint)
	  if eq == "" {
		   eq = "0"
	  }
	   coll := decimal.Zero
	  for _, rule := range b.ListCollateral() {
		   _ = rule
	  }
	   ml := "0"
	  e, errE := decimal.NewFromString(eq)
	  l, errL := decimal.NewFromString(liabTotal)
	  if errE == nil && errL == nil && l.IsPositive() && e.IsPositive() {
		   ml = e.Div(l).String()
	  } else if errE == nil && e.IsPositive() && (!l.IsPositive()) {
		   ml = "999"
	   }
	   return b.UpsertRisk(userID, RiskView{
		  UserID: userID, MarginLevel: ml, TotalEquity: eq, TotalLiability: liabTotal,
		  TotalCollateral: coll.String(), RiskGrade: gradeFromLevel(ml),
		  CrossDebtRatio: ratioLiabilityEquity(liabTotal, eq),
	   })
  }

   func (b *Book) RiskUnits(userID string) map[string]any {
	  view := b.Risk(userID)
	  return map[string]any{
		   "userId":      userID,
		   "marginLevel": view.MarginLevel,
		  "totalEquity": view.TotalEquity,
		   "totalLiability": view.TotalLiability,
		   "totalCollateral": view.TotalCollateral,
		  "riskGrade":       view.RiskGrade,
		  "crossDebtRatio":  view.CrossDebtRatio,
		   "updatedAt":       view.UpdatedAt,
		   "units": []map[string]string{
			   {"code": "MARGIN_LEVEL", "value": view.MarginLevel},
			  {"code": "CROSS_DEBT_RATIO", "value": view.CrossDebtRatio},
			  {"code": "RISK_GRADE", "value": view.RiskGrade},
		   },
	  }
  }
