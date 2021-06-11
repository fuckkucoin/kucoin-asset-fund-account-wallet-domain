package fundmat

import (
	"fmt"
	"strings"
	"github.com/shopspring/decimal"
)

// Compare 可用与冻结。账本没有的科目若投影这两栏非零，记为漂移。
// 在途与占用不在账本余额里，不参与本。
  func Compare(rows []MatRow, lines []BookLine, tol string) ([]Drift, error) {
	  tol = strings.TrimSpace(tol)
	   if tol == "" {
		  tol = "0"
	   }
	   type key struct{ asset, subject string }
	   books := make(map[key]BookLine, len(lines))
	   for _, ln := range lines {
		  k := key{NormAsset(ln.Asset), NormSubject(ln.Subject)}
		  if k.asset == "" || k.subject == "" {
			  continue
		  }
		   ln.Asset = k.asset
		   ln.Subject = k.subject
		   books[k] = ln
	   }
	  seen := map[key]struct{}{}
	   var out []Drift
	   for _, row := range rows {
		  k := key{row.Asset, row.Subject}
		  seen[k] = struct{}{}
		   ln, ok := books[k]
		   avail, frozen := "0", "0"
		   if ok {
			  var err error
			  avail,
				   err = Canon(ln.Available)
			   if err != nil {
				  return nil, err
			  }
			   frozen, err = Canon(ln.Frozen)
			  if err != nil {
				   return nil, err
			   }
		  }
		  d, err := diffField(row, "available", avail, row.Available, tol)
		  if err != nil {
			   return nil, err
		  }
		  out = append(out, d...)
		  d, err = diffField(row, "frozen", frozen, row.Frozen, tol)
		  if err != nil {
			  return nil, err
		  }
		  out = append(out, d...)
	   }
	   for k, ln := range books {
		  if _, ok := seen[k]; ok {
			   continue
		  }
		   avail, err := Canon(ln.Available)
		   if err != nil {
			   return nil, err
		   }
		   frozen, err := Canon(ln.Frozen)
		  if err != nil {
			  return nil, err
		   }
		   blank := MatRow{UserID: NormUser(ln.UserID), Asset: k.asset, Subject: k.subject, Available: "0", Frozen: "0"}
		   if ln.UserID == "" && len(rows) > 0 {
			   blank.UserID = rows[0].UserID
		  }
		   d, err := diffField(blank, "available", avail, "0", tol)
		  if err != nil {
			  return nil, err
		   }
		  out = append(out, d...)
		   d, err = diffField(blank, "frozen", frozen, "0", tol)
		  if err != nil {
			   return nil, err
		   }
		  out = append(out, d...)
	  }
	   return out, nil
  }

  func diffField(row MatRow, field, ledger, proj, tol string) ([]Drift, error) {
	  ok, err := WithinTol(proj, ledger, tol)
	   if err != nil {
		   return nil, err
	  }
	  if ok {
		  return nil, nil
	  }
	  return []Drift{{
		   UserID: row.UserID, Asset: row.Asset, Subject: row.Subject,
		  Field: field, Ledger: ledger, Projection: proj,
	   }}, nil
  }

// Canon 把已落库金额收成十进制字符串。空串视为 0，负数拒绝。
  func Canon(s string) (string, error) {
	  s = strings.TrimSpace(s)
	   if s == "" {
		  return "0", nil
	  }
	   for _, r := range s {
		   if r == 'e' || r == 'E' {
			   return "", ErrBadEvent
		   }
	   }
	  d, err := decimal.NewFromString(s)
	  if err != nil || d.IsNegative() {
		   return "", ErrBadEvent
	   }
	   if d.Exponent() < -18 {
		   return "", ErrBadEvent
	  }
	  return d.String(), nil
  }

   func formatDrifts(ds []Drift) string {
	  if len(ds) == 0 {
		  return ""
	   }
	   d := ds[0]
	   s := fmt.Sprintf("%s %s %s %s ledger=%s proj=%s", d.UserID, d.Asset, d.Subject, d.Field, d.Ledger, d.Projection)
	   if len(ds) > 1 {
		   s = fmt.Sprintf("%s (+%d)", s, len(ds)-1)
	   }
	   if len(s) > 240 {
		  s = s[:240]
	  }
	   return s
   }
