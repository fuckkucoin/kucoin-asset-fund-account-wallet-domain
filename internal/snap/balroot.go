package snap

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// BalanceRoot 计算一组快照科目的余额根 checksum。
// 形态：对 userID|book|asset → available|frozen 的映射，按 key 排序后 SHA-256。
// 与 restoregate.BalanceRoot 协议相同，用于 SplitBrainProof 的前后核对。
// 若 snap 为零值（无科目），返回空串（零根）。

   func (s Snapshot) BalanceRoot() string {
	   if len(s.Positions) == 0 {
		  return ""
	   }
	   entries := make(map[string]string,
		   len(s.Positions))
	   uid := strings.TrimSpace(s.UserID)
	  for _, p := range s.Positions {
		   key := uid + "|" + strings.ToUpper(strings.TrimSpace(p.Book)) + "|" + strings.ToUpper(strings.TrimSpace(p.Asset))
		  entries[key] = strings.TrimSpace(p.Available) + "|" + strings.TrimSpace(p.Frozen)
	   }
	   return balanceRootFromMap(entries)
  }

// BalanceRootMulti 计算多用户快照集合的余额根（灾备对账用）。
// snaps 可来自不同用户；所有科目合并后统一排序再 SHA-256。
  func BalanceRootMulti(snaps []Snapshot) string {
	   entries := make(map[string]string)
	  for _, s := range snaps {
		   uid := strings.TrimSpace(s.UserID)
		  for _, p := range s.Positions {
			  key := uid + "|" + strings.ToUpper(strings.TrimSpace(p.Book)) + "|" + strings.ToUpper(strings.TrimSpace(p.Asset))
			   entries[key] = strings.TrimSpace(p.Available) + "|" + strings.TrimSpace(p.Frozen)
		   }
	   }
	  if len(entries) == 0 {
		   return ""
	   }
	   return balanceRootFromMap(entries)
  }

// BalanceRootDiff 比较两个余额根，返回是否一致及差异描述。
// before / after 均为 BalanceRoot / BalanceRootMulti 的输出。
  func BalanceRootDiff(before, after string) (match bool, diffSummary string) {
	  if before == after {
		  return true, ""
	  }
	   return false, "balance_root_before=" + before + " balance_root_after=" + after
  }

  func balanceRootFromMap(entries map[string]string) string {
	  keys := make([]string,
		   0, len(entries))
	  for k := range entries {
		  keys = append(keys, k)
	   }
	   sort.Strings(keys)
	  h := sha256.New()
	  for _, k := range keys {
		   _, _ = h.Write([]byte(k))
		  _, _ = h.Write([]byte{0})
		  _, _ = h.Write([]byte(entries[k]))
		  _, _ = h.Write([]byte{0})
	  }
	  return hex.EncodeToString(h.Sum(nil))
   }

// SnapshotBalanceRootPair 封装回灌前后两个快照的余额根核对结果。
type SnapshotBalanceRootPair struct {
	PreSnap  Snapshot
	PostSnap Snapshot
	PreRoot string
	PostRoot string
	Match    bool
	DiffSummary string
}

// CompareBalanceRoots 核对两快照的余额根，返回核对结果。

  func CompareBalanceRoots(pre, post Snapshot) SnapshotBalanceRootPair {
	  preRoot := pre.BalanceRoot()
	   postRoot := post.BalanceRoot()
	   match,
		   diff := BalanceRootDiff(preRoot, postRoot)
	   return SnapshotBalanceRootPair{
		  PreSnap:  pre,
		  PostSnap: post,
		  PreRoot: preRoot,
		  PostRoot:    postRoot,
		  Match:       match,
		  DiffSummary: diff,
	   }
   }
