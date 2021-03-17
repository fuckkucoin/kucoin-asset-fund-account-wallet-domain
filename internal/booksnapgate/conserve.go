package booksnapgate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/pkg/money"
)

// ConserveResult 单次守恒校验结果。
type ConserveResult struct {
	Conserved bool
	TotalDebit  money.Amount
	TotalCredit money.Amount
	Delta       money.Amount
	BookResults []BookConserveResult
	Err         error
}

type BookConserveResult struct {
	Book string
	Debit money.Amount
	Credit money.Amount
	Count  int
}

// CheckConserve 对给定账单行做多科目守恒校验。
  func CheckConserve(bills []BillEntry) ConserveResult {
	  type bookAcc struct {
		   debit  money.Amount
		  credit money.Amount
		  count int
	  }
	   books := map[string]*bookAcc{}
	  bookOrder := []string{}
	  for _, b := range bills {
		   bk := strings.ToLower(strings.TrimSpace(b.Book))
		   if bk == "" {
			  continue
		  }
		  acc, ok := books[bk]
		  if !ok {
			   acc = &bookAcc{
				   debit:  money.Zero(),
				  credit: money.Zero(),
			   }
			   books[bk] = acc
			  bookOrder = append(bookOrder, bk)
		   }
		  if d, err := money.Parse(orZero(b.Debit)); err == nil && d.IsPositive() {
			   acc.debit, _ = acc.debit.Add(d)
			   acc.count++
		  }
		   if c, err := money.Parse(orZero(b.Credit)); err == nil && c.IsPositive() {
			  acc.credit, _ = acc.credit.Add(c)
			  acc.count++
		   }
	   }
	  sort.Strings(bookOrder)
	  totalDebit := money.Zero()
	  totalCredit := money.Zero()
	   bookResults := make([]BookConserveResult, 0, len(bookOrder))
	   for _, bk := range bookOrder {
		   acc := books[bk]
		  totalDebit, _ = totalDebit.Add(acc.debit)
		  totalCredit, _ = totalCredit.Add(acc.credit)
		  bookResults = append(bookResults, BookConserveResult{
			  Book: bk,
			  Debit: acc.debit,
			   Credit: acc.credit,
			   Count: acc.count,
		  })
	   }
	   delta, err := totalDebit.Sub(totalCredit)
	  if err != nil {
		  return ConserveResult{Err: err}
	   }
	   return ConserveResult{
		   Conserved: delta.IsZero(),
		   TotalDebit:  totalDebit,
		  TotalCredit: totalCredit,
		   Delta:       delta,
		  BookResults: bookResults,
	  }
  }

// ConserveChecksum 守恒结果的校验和。
   func ConserveChecksum(result ConserveResult) string {
	  h := sha256.New()
	  h.Write([]byte(fmt.Sprintf("%v", result.Conserved)))
	  h.Write([]byte{0})
	  h.Write([]byte(result.TotalDebit.String()))
	  h.Write([]byte{0})
	   h.Write([]byte(result.TotalCredit.String()))
	  h.Write([]byte{0})
	  h.Write([]byte(result.Delta.String()))
	  h.Write([]byte{0})
	  for _, br := range result.BookResults {
		  h.Write([]byte(br.Book))
		  h.Write([]byte{0})
		   h.Write([]byte(br.Debit.String()))
		   h.Write([]byte{0})
		  h.Write([]byte(br.Credit.String()))
		   h.Write([]byte{0})
	   }
	  return hex.EncodeToString(h.Sum(nil))
  }

// BuildMultiConserveFromBills 从账单行构建 MultiConserve。

  func BuildMultiConserveFromBills(userID, asset, leaseID, snapWindow string, snapGen int64, bills []BillEntry, now time.Time) MultiConserve {
	  cr := CheckConserve(bills)
	  books := make([]BookTotals, 0, len(cr.BookResults))
	   for _, br := range cr.BookResults {
		  books = append(books, BookTotals{
			  Book:  br.Book,
			   Debit: br.Debit.String(),
			   Credit: br.Credit.String(),
			  Count: br.Count,
		  })
	   }
	   mc := MultiConserve{
		  UserID: userID,
		   Asset: asset,
		  SnapGen:     snapGen,
		  Books:       books,
		  TotalDebit:  cr.TotalDebit.String(),
		   TotalCredit: cr.TotalCredit.String(),
		   Conserved: cr.Conserved,
		   CheckedAt:  now,
		   SnapWindow: snapWindow,
		   LeaseID: leaseID,
	   }
	  if !cr.Conserved {
		   mc.Delta = cr.Delta.String()
	   }
	   return mc
  }

// ValidateMultiConserve 验证 MultiConserve 记录一致性。
   func ValidateMultiConserve(mc MultiConserve) error {
	   totalD, err := money.ParseBook(orZero(mc.TotalDebit))
	   if err != nil {
		  return fmt.Errorf("invalid total debit: %w", err)
	   }
	  totalC, err := money.ParseBook(orZero(mc.TotalCredit))
	  if err != nil {
		  return fmt.Errorf("invalid total credit: %w", err)
	   }
	  sumD := money.Zero()
	   sumC := money.Zero()
	  for _, bt := range mc.Books {
		  d,
			   err := money.ParseBook(orZero(bt.Debit))
		  if err != nil {
			   return fmt.Errorf("invalid book debit %s: %w", bt.Book, err)
		   }
		  c, err := money.ParseBook(orZero(bt.Credit))
		   if err != nil {
			  return fmt.Errorf("invalid book credit %s: %w", bt.Book, err)
		  }
		  sumD, _ = sumD.Add(d)
		   sumC, _ = sumC.Add(c)
	   }
	   if sumD.String() != totalD.String() {
		  return fmt.Errorf("book debit sum %s != total %s", sumD.String(), totalD.String())
	  }
	  if sumC.String() != totalC.String() {
		   return fmt.Errorf("book credit sum %s != total %s", sumC.String(), totalC.String())
	   }
	   delta, _ := totalD.Sub(totalC)
	   if mc.Conserved && !delta.IsZero() {
		  return fmt.Errorf("marked conserved but delta=%s", delta.String())
	  }
	  if !mc.Conserved && delta.IsZero() {
		  return fmt.Errorf("marked not conserved but delta=0")
	  }
	   return nil
   }
