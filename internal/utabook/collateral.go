package utabook

import (
	"strings"
	"time"
	"github.com/shopspring/decimal"
)

type CollateralRule struct {
	Currency string `json:"currency"`
	Haircut           string `json:"haircut"`
	CollateralEnabled bool   `json:"collateralEnabled"`
	MaxLTV string `json:"maxLtv"`
	UpdatedAt int64 `json:"updatedAt"`
}

  func (b *Book) PutCollateral(rule CollateralRule) (CollateralRule, error) {
	   if b == nil {
		   return rule, nil
	  }
	   rule.Currency = strings.ToUpper(strings.TrimSpace(rule.Currency))
	  if rule.Currency == "" {
		  return rule, nil
	   }
	   if rule.Haircut == "" {
		   rule.Haircut = "1"
	   }
	  if rule.MaxLTV == "" {
		   rule.MaxLTV = "0"
	   }
	   if rule.UpdatedAt == 0 {
		  rule.UpdatedAt = time.Now().UTC().UnixMilli()
	  }
	  if err := b.persistCollateral(rule); err != nil {
		  return CollateralRule{}, err
	  }
	  b.mu.Lock()
	  b.collateral[rule.Currency] = rule
	   b.mu.Unlock()
	   return rule, nil
   }

   func (b *Book) CollateralRule(currency string) CollateralRule {
	  currency = strings.ToUpper(strings.TrimSpace(currency))
	   if b == nil {
		  return CollateralRule{Currency: currency, Haircut: "1", CollateralEnabled: false, MaxLTV: "0"}
	   }
	   b.mu.Lock()
	   r, ok := b.collateral[currency]
	  b.mu.Unlock()
	  if !ok {
		  return CollateralRule{Currency: currency, Haircut: "1", CollateralEnabled: false, MaxLTV: "0"}
	   }
	  return r
   }

  func (b *Book) ListCollateral() []CollateralRule {
	  if b == nil {
		   return []CollateralRule{}
	   }
	  b.mu.Lock()
	   out := make([]CollateralRule, 0, len(b.collateral))
	  for _, r := range b.collateral {
		  out = append(out, r)
	   }
	   b.mu.Unlock()
	   return out
  }

// CollateralStatus 返回公开 REST 用的抵押状态码：1=可抵押，0=不可抵押。
  func (b *Book) CollateralStatus(currency string) string {
	   r := b.CollateralRule(currency)
	  if r.CollateralEnabled {
		   return "1"
	   }
	   return "0"
   }

   func (b *Book) AdjustedCollateralValue(currency, balance string) string {
	   r := b.CollateralRule(currency)
	  bal,
		   err := decimal.NewFromString(strings.TrimSpace(balance))
	  if err != nil || !bal.IsPositive() {
		   return "0"
	   }
	  if !r.CollateralEnabled {
		   return "0"
	   }
	   h, err := decimal.NewFromString(strings.TrimSpace(r.Haircut))
	  if err != nil || !h.IsPositive() {
		   return "0"
	  }
	  if h.GreaterThan(decimal.NewFromInt(1)) {
		   return bal.String()
	  }
	  return bal.Mul(h).String()
  }

   func (b *Book) CollateralCatalogView() []map[string]any {
	   rules := b.ListCollateral()
	  out := make([]map[string]any,
		  0, len(rules))
	  for _, r := range rules {
		  out = append(out, map[string]any{
			   "currency": r.Currency, "haircut": r.Haircut,
			  "collateralEnabled": r.CollateralEnabled, "maxLtv": r.MaxLTV,
		   })
	   }
	  return out
   }
