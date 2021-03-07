package brmismatch

import (
	"strings"
	"time"
)

// MatchResult 是单次窗口匹配的结果。
type MatchResult struct {
	// Matched 成功配对的数量。
	Matched int `json:"matched"`
	// Cases 检测到的失配案件（未持久化，需调用方决定是否提交）。
	Cases []MismatchCase `json:"cases,omitempty"`
}

// Match 对给定时间窗口内的链上事件与账本条目进行双边匹配。
// windowKey 须由调用方传入，如 "2026-04-28T19:00Z/1h"。
// 返回失配案件列表（均处于 StateOpen，ID 已填充，ProofHash 已计算）。
  func Match(events []ChainEvent, entries []BookEntry, windowKey string, now time.Time) (MatchResult, error) {
	  if strings.TrimSpace(windowKey) == "" {
		   return MatchResult{}, ErrWindow
	  }
	  // 构建链上事件索引：asset+normalizeAmount → []ChainEvent
	   // 链上 mint 对应 book mint；链上 burn 对应 book redeem。
	   chainIndex := map[string][]ChainEvent{}
	   for _, ev := range events {
		   k := chainMatchKey(ev.Asset, ev.Kind, ev.Amount)
		  chainIndex[k] = append(chainIndex[k], ev)
	  }
	   // 构建账本条目索引：asset+kind+normalizeAmount → []BookEntry
	  bookIndex := map[string][]BookEntry{}
	  for _, en := range entries {
		   k := bookMatchKey(en.Asset, en.Kind, en.Amount)
		   bookIndex[k] = append(bookIndex[k],
			  en)
	   }
	   matched := 0
	   var cases []MismatchCase
	   // 检测链上有记录但账本无配对：MintOver / BurnOver
	  for _, ev := range events {
		   k := chainMatchKey(ev.Asset, ev.Kind, ev.Amount)
		   pool := bookIndex[k]
		  if len(pool) == 0 {
			  mismatchKind := KindMintOver
			  if strings.ToLower(ev.Kind) == "burn" {
				   mismatchKind = KindBurnOver
			  }
			   ceh := ChainEventHash(ev.TxHash, ev.LogIndex)
			  eh := EntryHash("", "") // 无对应 entry
			   proof, perr := ComputeProof(ceh, eh, windowKey)
			  if perr != nil {
				  return MatchResult{}, perr
			   }
			  cases = append(cases, MismatchCase{
				  Kind: mismatchKind,
				   State: StateOpen,
				   Asset: ev.Asset,
				   ChainAmount: normalizeAmt(ev.Amount),
				  ChainEventHash: ceh,
				   EntryHash:      eh,
				  ProofHash:      proof,
				  WindowKey:      windowKey,
				   DetectedAt: now,
				   UpdatedAt: now,
			  })
		  } else {
			   // 消费一条 entry 配对
			   bookIndex[k] = pool[1:]
			   matched++
		  }
	  }
	   // 检测账本有记录但链上无配对：MintMiss / BurnMiss
	   for _, pool := range bookIndex {
		  for _, en := range pool {
			   k := bookMatchKey(en.Asset,
				   en.Kind, en.Amount)
			  chainPool := chainIndex[k]
			   if len(chainPool) == 0 {
				   mismatchKind := KindMintMiss
				   if strings.ToLower(en.Kind) == "redeem" {
					  mismatchKind = KindBurnMiss
				   }
				   ceh := ChainEventHash("", 0) // 无对应链事件
				  eh := EntryHash(en.ID,
					   en.Amount)
				   proof, perr := ComputeProof(ceh, eh, windowKey)
				   if perr != nil {
					  return MatchResult{}, perr
				   }
				   cases = append(cases, MismatchCase{
					  Kind:   mismatchKind,
					  State:  StateOpen,
					   UserID: en.UserID,
					   Asset:      en.Asset,
					   BookAmount: normalizeAmt(en.Amount),
					   ChainEventHash: ceh,
					  EntryHash:      eh,
					  ProofHash:      proof,
					   WindowKey:      windowKey,
					   DetectedAt: now,
					  UpdatedAt:  now,
				   })
			   }
			  // 已被链上事件消费的不再重复报告
		   }
	  }
	  return MatchResult{Matched: matched, Cases: cases}, nil
  }

// chainMatchKey 构造链上事件的匹配键。
// 链上 mint → 对应 book mint；链上 burn → 对应 book redeem。

   func chainMatchKey(asset, kind, amount string) string {
	   bkind := "mint"
	   if strings.ToLower(kind) == "burn" {
		   bkind = "redeem"
	  }
	  return strings.ToUpper(strings.TrimSpace(asset)) + "|" + bkind + "|" + normalizeAmt(amount)
   }

// bookMatchKey 构造账本条目的匹配键。

   func bookMatchKey(asset, kind, amount string) string {
	  return strings.ToUpper(strings.TrimSpace(asset)) + "|" + strings.ToLower(strings.TrimSpace(kind)) + "|" + normalizeAmt(amount)
   }

// normalizeAmt 对金额字符串做轻量归一化：trim 空白，去除末尾冗余零。
// 不做数值解析，保留小数点前至少一位。
  func normalizeAmt(s string) string {
	   s = strings.TrimSpace(s)
	   if !strings.Contains(s, ".") {
		  return s
	   }
	   s = strings.TrimRight(s, "0")
	  s = strings.TrimRight(s, ".")
	  if s == "" {
		  return "0"
	   }
	  return s
  }
