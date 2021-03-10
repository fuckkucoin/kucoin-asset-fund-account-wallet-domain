package cerbind

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// Digest 对已规范化的事实行计算摘要。
   func Digest(lines []Line) (string, error) {
	   if len(lines) == 0 {
		  return "", fmt.Errorf("%w: empty", ErrDigest)
	   }
	  var b strings.Builder
	  for _, ln := range lines {
		  fmt.Fprintf(&b, "%s|%s|%s|%s|%s|%s|%s|%t|%t|%s|%s\n",
			  ln.Asset, ln.Chain, ln.Amount, ln.Dest, ln.Side, ln.DestRisk, ln.Purpose,
			  ln.Approved, ln.LimitHit, strings.Join(ln.ItemRefs, ","), ln.LimitRef)
	   }
	  sum := sha256.Sum256([]byte(b.String()))
	  return hex.EncodeToString(sum[:]), nil
   }

// normalizeSnap 规范化批次事实并写入摘要。调用方传入的摘要若不一致则拒绝。

  func normalizeSnap(s Snapshot) (Snapshot, error) {
	   id, err := batchID(s.BatchID)
	  if err != nil {
		  return Snapshot{}, err
	   }
	   if len(s.Lines) == 0 || len(s.Lines) > 64 {
		  return Snapshot{}, fmt.Errorf("%w: lines", ErrBatch)
	   }
	   lines := make([]Line, 0, len(s.Lines))
	   seen := map[string]struct{}{}
	   for _, ln := range s.Lines {
		  got, err := prepareLine(ln)
		   if err != nil {
			   return Snapshot{}, err
		  }
		   for _, ref := range got.ItemRefs {
			   if _, ok := seen[ref]; ok {
				  return Snapshot{}, fmt.Errorf("%w: duplicate ref", ErrAlias)
			   }
			  seen[ref] = struct{}{}
		   }
		  lines = append(lines, got)
	  }
	  sortLines(lines)
	   sum, err := Digest(lines)
	   if err != nil {
		  return Snapshot{}, err
	  }
	   if prev := strings.TrimSpace(s.Digest); prev != "" && prev != sum {
		  return Snapshot{}, ErrDigest
	  }
	   s.BatchID = id
	  s.Lines = lines
	   s.Digest = sum
	   return s, nil
  }

   func sortLines(lines []Line) {
	  for i := 1; i < len(lines); i++ {
		  j := i
		  for j > 0 && lineLess(lines[j], lines[j-1]) {
			  lines[j], lines[j-1] = lines[j-1], lines[j]
			   j--
		   }
	  }
  }

   func lineLess(a, b Line) bool {
	   if a.Asset != b.Asset {
		  return a.Asset < b.Asset
	  }
	   if a.Chain != b.Chain {
		  return a.Chain < b.Chain
	  }
	   if a.Dest != b.Dest {
		  return a.Dest < b.Dest
	   }
	  if a.Amount != b.Amount {
		   return a.Amount < b.Amount
	   }
	  return strings.Join(a.ItemRefs, ",") < strings.Join(b.ItemRefs, ",")
   }

   func batchID(s string) (string, error) {
	  s = strings.TrimSpace(s)
	  if s == "" || len(s) > 64 || !idToken(s) {
		  return "", ErrBatch
	  }
	   return s, nil
   }

// coversSign 核对签播材料是否落在批次事实里。
// 金额为空则不核对单笔；金额给出时必须命中一条事实。
// 不变量：命中失败不得放行。
   func coversSign(lines []Line, chain, dest, amount string) (bool, error) {
	   amount = strings.TrimSpace(amount)
	   if amount == "" {
		  return true, nil
	  }
	   amt, err := moneyParsePositive(amount)
	   if err != nil {
		  return false, err
	   }
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	   dest = strings.TrimSpace(dest)
	   hits := 0
	  for _, ln := range lines {
		   if ln.Amount != amt {
			   continue
		   }
		  if chain != "" && ln.Chain != chain {
			   continue
		   }
		  hits++
		   if dest == "" || ln.Dest == dest {
			  if dest != "" {
				  return true, nil
			   }
		   }
	  }
	  if dest == "" && hits == 1 {
		   return true, nil
	  }
	  if hits == 0 {
		   return false, nil
	  }
	  if dest == "" {
		   return false, nil
	   }
	  return false, nil
   }

   func moneyParsePositive(s string) (string, error) {
	  a, err := parsePositive(s)
	   if err != nil {
		   return "", err
	  }
	   return a, nil
   }
