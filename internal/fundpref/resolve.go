package fundpref

import (
	"context"
	"strings"
)

const (
	AccountFunding = "funding"
	AccountTrading = "trading"
	AccountMargin   = "margin"
	AccountIsolated = "isolated"
	AccountFutures  = "futures"
	AccountEarn = "earn"
)

// ParseDefaultAccount 与 account.ParseBook 同名表，避免 fundpref 反向依赖 account/app。

  func ParseDefaultAccount(s string) (string, error) {
	   switch strings.ToLower(strings.TrimSpace(s)) {
	  case AccountFunding, "spot", "wallet", "main":
		   return AccountFunding, nil
	   case AccountTrading, "trade":
		  return AccountTrading, nil
	  case AccountMargin:
		  return AccountMargin, nil
	  case AccountIsolated, "isolated_margin":
		   return AccountIsolated, nil
	   case AccountFutures, "contract":
		  return AccountFutures, nil
	   case AccountEarn:
		  return AccountEarn, nil
	  default:
		  return "", ErrAccount
	   }
   }

// ResolveDefault 解析用户默认资金科目。无行回 funding。
  func (b *Book) ResolveDefault(ctx context.Context, userID string) (string, error) {
	   p, err := b.Get(ctx, userID)
	  if err != nil {
		   return "", err
	   }
	   book, err := ParseDefaultAccount(p.DefaultAccount)
	   if err != nil {
		   return "", ErrAccount
	  }
	  return book, nil
   }

// ResolveDefaultView 读面挂钩用的默认科目视图。

  func (b *Book) ResolveDefaultView(ctx context.Context, userID string) (DefaultView, error) {
	   book, err := b.ResolveDefault(ctx, userID)
	   if err != nil {
		   return DefaultView{}, err
	  }
	   return DefaultView{
		  UserID:         strings.TrimSpace(userID),
		  DefaultAccount: book,
		   AccountType:    KCAccountType(book),
		   Book:           book,
	   }, nil
  }

// KCAccountType 科目到公开账户类型。MAIN 对应 funding。

  func KCAccountType(book string) string {
	   switch book {
	  case AccountTrading:
		   return "TRADE"
	   case AccountMargin:
		   return "MARGIN"
	   case AccountIsolated:
		   return "ISOLATED"
	  case AccountFutures:
		   return "CONTRACT"
	  case AccountEarn:
		   return "EARN"
	   default:
		  return "MAIN"
	  }
  }

// ApplyOrder 按偏好序重排资产行；未点名的保持原相对序接在后面。
  func ApplyOrder[T any](rows []T, order []string, assetOf func(T) string) []T {
	  if len(rows) == 0 || len(order) == 0 || assetOf == nil {
		  return rows
	   }
	  idx := make(map[string]int, len(order))
	   for i, a := range order {
		   idx[strings.ToUpper(strings.TrimSpace(a))] = i
	  }
	   head := make([]T, len(order))
	  used := make([]bool, len(order))
	  tail := make([]T, 0, len(rows))
	  for _, row := range rows {
		  key := strings.ToUpper(strings.TrimSpace(assetOf(row)))
		   i, ok := idx[key]
		  if !ok || used[i] {
			   tail = append(tail, row)
			  continue
		  }
		  head[i] = row
		   used[i] = true
	  }
	   out := make([]T, 0, len(rows))
	   for i, ok := range used {
		  if ok {
			  out = append(out, head[i])
		   }
	  }
	  return append(out, tail...)
   }

  // OverlayRead 查询串为空时用偏好覆盖展示币与隐藏零余额。

   func OverlayRead(p Preference, quote, hideRaw string) (string, bool, bool) {
	   q := strings.ToUpper(strings.TrimSpace(quote))
	   fromPref := false
	  if q == "" {
		   q = p.DisplayCurrency
		  fromPref = true
	  }
	   hideSet := strings.TrimSpace(hideRaw) != ""
	  hide := p.HideZero
	   if hideSet {
		  switch strings.ToLower(strings.TrimSpace(hideRaw)) {
		  case "1", "true", "yes":
			  hide = true
		  default:
			   hide = false
		   }
	  }
	  return q, hide, fromPref
  }
