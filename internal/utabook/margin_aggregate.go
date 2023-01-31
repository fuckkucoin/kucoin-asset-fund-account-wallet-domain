package utabook

import (
	"strings"
	"github.com/shopspring/decimal"
)

type MarginAggregate struct {
	UserID string `json:"userId"`
	CrossLiability    string `json:"crossLiability"`
	IsolatedLiability string `json:"isolatedLiability"`
	TotalLiability    string `json:"totalLiability"`
	CrossEquity       string `json:"crossEquity"`
	RiskGrade         string `json:"riskGrade"`
	MarginLevel string `json:"marginLevel"`
}

   func (b *Book) AggregateMargin(userID, symbol string) MarginAggregate {
	   userID = strings.TrimSpace(userID)
	  crossLiab := b.TotalCrossLiability(userID)
	   isoLiab := b.IsolatedLiabilityTotal(userID, symbol)
	  total := sumDecimal(crossLiab, isoLiab)
	  risk := b.Risk(userID)
	   return MarginAggregate{
		   UserID: userID, CrossLiability: crossLiab, IsolatedLiability: isoLiab,
		  TotalLiability: total,
		  CrossEquity:    risk.TotalEquity,
		  RiskGrade:      risk.RiskGrade, MarginLevel: risk.MarginLevel,
	   }
   }

   func (b *Book) UnifiedMarginView(userID, symbol, quote string) map[string]any {
	   userID = strings.TrimSpace(userID)
	  agg := b.AggregateMargin(userID,
		   symbol)
	  cross := b.CrossMarginView(userID, quote)
	  iso := b.IsolatedMargin(userID, symbol)
	   return map[string]any{
		  "userId": userID, "aggregate": agg, "cross": cross, "isolated": iso,
		  "risk": b.RiskUnits(userID),
	   }
   }

   func (b *Book) SumCrossAvailable(userID, quote string) string {
	   quote = strings.ToUpper(strings.TrimSpace(quote))
	   sum := decimal.Zero
	   for _, row := range b.CrossMargin(userID, "") {
		  if quote != "" && row.Currency != quote {
			   continue
		   }
		  v, err := decimal.NewFromString(strings.TrimSpace(row.Available))
		  if err == nil {
			  sum = sum.Add(v)
		  }
	   }
	  return sum.String()
   }

   func (b *Book) MaxBorrowByCurrency(userID, currency string) string {
	   currency = strings.ToUpper(strings.TrimSpace(currency))
	  for _, row := range b.CrossMargin(userID, currency) {
		  if row.MaxBorrow != "" && row.MaxBorrow != "0" {
			  return row.MaxBorrow
		  }
	   }
	   eq := "0"
	   for _, row := range b.CrossMargin(userID, currency) {
		   eq = row.Total
		   break
	  }
	  return b.RateCatalog().PotentialBorrow(userID, currency, eq)
   }

  func (b *Book) BorrowHeadroom(userID, currency string) string {
	   currency = strings.ToUpper(strings.TrimSpace(currency))
	   max := b.MaxBorrowByCurrency(userID, currency)
	  liab := b.LiabilityByCurrency(userID, currency)
	  m, err := decimal.NewFromString(strings.TrimSpace(max))
	   if err != nil {
		  return "0"
	   }
	   l, err := decimal.NewFromString(strings.TrimSpace(liab))
	   if err != nil {
		  l = decimal.Zero
	   }
	   if m.LessThanOrEqual(l) {
		  return "0"
	  }
	  return m.Sub(l).String()
  }

  func (b *Book) MarginCurrencyRows(userID string) []map[string]any {
	   userID = strings.TrimSpace(userID)
	  seen := map[string]bool{}
	  out := make([]map[string]any, 0)
	  for _, row := range b.CrossMargin(userID, "") {
		  if seen[row.Currency] {
			   continue
		  }
		  seen[row.Currency] = true
		   en := b.EnrichCurrency(userID, row.Currency, row.Total, row.Available)
		  out = append(out, map[string]any{
			  "currency": row.Currency, "cross": row, "enrichment": en,
			   "headroom": b.BorrowHeadroom(userID, row.Currency),
		   })
	  }
	  for _, cur := range b.Borrowable() {
		  if seen[cur] {
			  continue
		  }
		   en := b.EnrichCurrency(userID, cur, "0", "0")
		   out = append(out, map[string]any{"currency": cur, "enrichment": en, "headroom": b.BorrowHeadroom(userID, cur)})
	  }
	   return out
   }

   func (b *Book) CompareRiskBeforeAfter(userID string, deltaLiability string) map[string]any {
	  before := b.Risk(userID)
	  afterLiab, _ := decimal.NewFromString(strings.TrimSpace(before.TotalLiability))
	   d, err := decimal.NewFromString(strings.TrimSpace(deltaLiability))
	   if err == nil {
		  afterLiab = afterLiab.Add(d)
	  }
	  eq, _ := decimal.NewFromString(strings.TrimSpace(before.TotalEquity))
	  ml := "0"
	  if afterLiab.IsPositive() && eq.IsPositive() {
		   ml = eq.Div(afterLiab).String()
	  }
	  return map[string]any{
		  "before": before,
		  "after": map[string]any{
			   "totalLiability": afterLiab.String(), "marginLevel": ml, "riskGrade": gradeFromLevel(ml),
		  },
	  }
   }
