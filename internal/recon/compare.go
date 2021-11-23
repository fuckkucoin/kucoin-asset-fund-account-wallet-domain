package recon

import (
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/stmt"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Source string

const (
	SrcLedger    Source = "ledger"
	SrcSnapshot  Source = "snapshot"
	SrcStatement Source = "statement"
)

type SourceBal struct {
	Source Source `json:"source"`
	Book   string `json:"book"`
	Asset  string `json:"asset"`
	Available string `json:"available"`
	Frozen string `json:"frozen"`
}

type Delta struct {
	Book   string `json:"book"`
	Asset  string `json:"asset"`
	Left   string `json:"left"`
	Right  string `json:"right"`
	Source string `json:"source"`
	Note   string `json:"note"`
}

type CompareReport struct {
	UserID string  `json:"user_id"`
	Deltas []Delta `json:"deltas"`
	OK     bool    `json:"ok"`
	Checked int         `json:"checked"`
	Sources []SourceBal `json:"sources,omitempty"`
}

type ShotPos struct {
	Book string
	Asset string
	Available string
	Frozen string
	Total  string
}

type Shot struct {
	UserID string
	Positions []ShotPos
}

// CompareSources 三方对余额：账本科目、快照、对账单 running。
// 金额只认 ParseBook。对不上落 Delta，不得四舍五入抹平。
  func CompareSources(userID string, views []ledger.BookView, snapShot Shot, report stmt.Report) (CompareReport, error) {
	  out := CompareReport{UserID: userID, OK: true}
	  byView := map[string]ledger.BookView{}
	   for _, v := range views {
		   byView[srcKey(v.Book, v.Asset)] = v
		  out.Sources = append(out.Sources,
			   SourceBal{Source: SrcLedger, Book: v.Book, Asset: v.Asset, Available: v.Available, Frozen: v.Frozen})
		  out.Checked++
	   }
	   if snapShot.UserID != "" && snapShot.UserID != userID {
		  return CompareReport{}, fmt.Errorf("snapshot user mismatch")
	  }
	   for _, p := range snapShot.Positions {
		  out.Sources = append(out.Sources, SourceBal{Source: SrcSnapshot, Book: p.Book, Asset: p.Asset, Available: p.Available, Frozen: p.Frozen})
		   v, ok := byView[srcKey(p.Book, p.Asset)]
		   if !ok {
			   out.Deltas = append(out.Deltas, Delta{Book: p.Book, Asset: p.Asset, Left: "0", Right: p.Total, Source: string(SrcSnapshot), Note: "snapshot has book missing on ledger"})
			   out.OK = false
			  continue
		  }
		  if err := sameAmt(v.Available, p.Available); err != nil {
			   out.Deltas = append(out.Deltas, Delta{Book: p.Book, Asset: p.Asset, Left: v.Available, Right: p.Available, Source: string(SrcSnapshot), Note: "available drifted vs snapshot"})
			  out.OK = false
		  }
		   if err := sameAmt(v.Frozen, p.Frozen); err != nil {
			   out.Deltas = append(out.Deltas, Delta{Book: p.Book, Asset: p.Asset, Left: v.Frozen, Right: p.Frozen, Source: string(SrcSnapshot), Note: "frozen drifted vs snapshot"})
			   out.OK = false
		   }
	   }
	   if len(snapShot.Positions) > 0 {
		  seen := map[string]struct{}{}
		  for _, p := range snapShot.Positions {
			  seen[srcKey(p.Book, p.Asset)] = struct{}{}
		   }
		  for _, v := range views {
			   if _, ok := seen[srcKey(v.Book, v.Asset)]; ok {
				  continue
			   }
			   out.Deltas = append(out.Deltas, Delta{Book: v.Book, Asset: v.Asset, Left: v.Available, Right: "0", Source: string(SrcSnapshot), Note: "ledger has book missing on snapshot"})
			  out.OK = false
		  }
	   }
	  run := lastRunning(report)
	   for key, running := range run {
		   book, asset, _ := strings.Cut(key, "|")
		   v, ok := byView[key]
		   if !ok {
			   out.Deltas = append(out.Deltas, Delta{Book: book, Asset: asset, Left: "0", Right: running, Source: string(SrcStatement), Note: "statement has book missing on ledger"})
			  out.OK = false
			  continue
		   }
		   total, err := addBook(v.Available, v.Frozen)
		  if err != nil {
			  return CompareReport{}, err
		   }
		   if err := sameAmt(total, running); err != nil {
			   out.Deltas = append(out.Deltas, Delta{Book: book, Asset: asset, Left: total, Right: running, Source: string(SrcStatement), Note: "book total != statement running"})
			  out.OK = false
		   }
	   }
	   return out, nil
   }

   func srcKey(book, asset string) string {
	  return strings.ToLower(strings.TrimSpace(book)) + "|" + strings.ToUpper(strings.TrimSpace(asset))
   }

   func sameAmt(a, b string) error {
	  left,
		  err := money.ParseBook(orZ(a))
	  if err != nil {
		   return err
	  }
	   right, err := money.ParseBook(orZ(b))
	   if err != nil {
		  return err
	  }
	  return money.Conserve(left, right)
   }

  func addBook(a, b string) (string, error) {
	   x, err := money.ParseBook(orZ(a))
	   if err != nil {
		  return "0", err
	   }
	   y, err := money.ParseBook(orZ(b))
	   if err != nil {
		  return "0", err
	   }
	   s, err := x.Add(y)
	  if err != nil {
		   return "0", err
	   }
	   return s.String(), nil
  }

   func lastRunning(rep stmt.Report) map[string]string {
	   out := map[string]string{}
	  for _, ln := range rep.Lines {
		  out[srcKey(ln.Book, ln.Asset)] = ln.Running
	   }
	   return out
  }

  func orZ(s string) string {
	  if strings.TrimSpace(s) == "" {
		  return "0"
	   }
	   return s
   }
// temporary guard removed after the production check
