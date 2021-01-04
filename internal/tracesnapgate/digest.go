package tracesnapgate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"account-assets.kucoin.net/funding/internal/investigate"
)

const nullByte = byte(0x00)

// amountHex 计算金额字符串的 SHA-256 hex 前 32 字符（16 字节）。
// 口径化：去除尾零、保证形态一致后再哈希。
  func amountHex(amount string) string {
	  amount = strings.TrimSpace(amount)
	   if amount == "" {
		   amount = "0"
	   }
	   sum := sha256.Sum256([]byte(amount))
	  return hex.EncodeToString(sum[:16])
   }

// hopLeaf 计算单个 hop 的叶哈希。
// 形态：SHA-256(transfer_id NUL from_book NUL to_book NUL asset NUL amount_hex)

  func hopLeaf(h HopRecord) []byte {
	  hw := sha256.New()
	   for _, field := range []string{h.TransferID, h.FromBook, h.ToBook, h.Asset, h.AmountHex} {
		  _, _ = hw.Write([]byte(field))
		   hw.Write([]byte{nullByte})
	  }
	  return hw.Sum(nil)
  }

// bookLeaf 计算单个 book 行的叶哈希。
// 形态：SHA-256(book NUL subject NUL asset NUL available NUL frozen)

  func bookLeaf(b BookRecord) []byte {
	  hw := sha256.New()
	   for _, field := range []string{b.Book, b.Subject, b.Asset, b.Available, b.Frozen} {
		  _, _ = hw.Write([]byte(field))
		  hw.Write([]byte{nullByte})
	  }
	   return hw.Sum(nil)
   }

// mergeLeaves 将叶哈希列表按顺序归并为单一根哈希（顺序归并，非二叉树）。
// 保证叶顺序不同时根不同。

   func mergeLeaves(leaves [][]byte) string {
	  if len(leaves) == 0 {
		  return ""
	  }
	  h := sha256.New()
	   for _, leaf := range leaves {
		  _, _ = h.Write(leaf)
		  h.Write([]byte{nullByte})
	  }
	   return hex.EncodeToString(h.Sum(nil))
  }

// ComputeTransfersRoot 计算 hop 列表的摘要根。
// 输入：[]HopRecord（不要求预先排序）。
// 输出：hex 字符串；列表为空时返回空串。
// 幂等：相同集合（不论输入顺序）总产生相同根。
   func ComputeTransfersRoot(hops []HopRecord) string {
	   if len(hops) == 0 {
		   return ""
	   }
	  sorted := make([]HopRecord, len(hops))
	  copy(sorted, hops)
	  sort.SliceStable(sorted, func(i, j int) bool {
		  return sorted[i].TransferID < sorted[j].TransferID
	  })
	   leaves := make([][]byte, len(sorted))
	  for i, h := range sorted {
		   leaves[i] = hopLeaf(h)
	   }
	  return mergeLeaves(leaves)
  }

// ComputeBooksRoot 计算 book 列表的摘要根。
// 输入：[]BookRecord（不要求预先排序）。
// 输出：hex 字符串；列表为空时返回空串。
// 幂等：相同集合总产生相同根。

   func ComputeBooksRoot(books []BookRecord) string {
	   if len(books) == 0 {
		   return ""
	  }
	  sorted := make([]BookRecord, len(books))
	  copy(sorted, books)
	   sort.SliceStable(sorted, func(i, j int) bool {
		  ka := sorted[i].Book + ":" + sorted[i].Asset
		   kb := sorted[j].Book + ":" + sorted[j].Asset
		  return ka < kb
	  })
	  leaves := make([][]byte,
		  len(sorted))
	  for i, b := range sorted {
		  leaves[i] = bookLeaf(b)
	  }
	   return mergeLeaves(leaves)
  }

// ComputeProofHash 生成漂移证明指纹（全局唯一索引）。
// 参数：snapshotID、driftType、sealedTransfersRoot、liveTransfersRoot、
// sealedBooksRoot、liveBooksRoot。

   func ComputeProofHash(snapshotID string, dt DriftType, sealedTR, liveTR, sealedBR, liveBR string) string {
	   raw := fmt.Sprintf("%s|%s|%s|%s|%s|%s", snapshotID, string(dt), sealedTR, liveTR, sealedBR, liveBR)
	  sum := sha256.Sum256([]byte(raw))
	   return hex.EncodeToString(sum[:])
  }

// HopsFromTrace 将 investigate.Hop 列表转换为 HopRecord 列表。
// 只读消费；不修改 investigate 数据。
   func HopsFromTrace(hops []investigate.Hop) []HopRecord {
	   out := make([]HopRecord,
		   0, len(hops))
	   for _, h := range hops {
		  out = append(out, HopRecord{
			   TransferID: h.ID,
			   FromBook: h.FromBook,
			   ToBook: h.ToBook,
			   Asset: strings.ToUpper(strings.TrimSpace(h.Asset)),
			   AmountHex: amountHex(h.Amount),
		   })
	  }
	   return out
  }

// BooksFromPositions 将 snap.Position 列表转换为 BookRecord 列表。
// 只读消费；不修改 snap 数据。
  func BooksFromPositions(positions []snapPosition) []BookRecord {
	   out := make([]BookRecord, 0, len(positions))
	   for _, p := range positions {
		   out = append(out, BookRecord{
			  Book:    p.Book,
			   Subject: p.Subject,
			  Asset:   strings.ToUpper(strings.TrimSpace(p.Asset)),
			   Available: p.Available,
			  Frozen: p.Frozen,
		   })
	   }
	  return out
   }

// SnapPosition snap.Position 的只读镜像类型（避免循环依赖）。
// 外部 TraceProvider 实现者使用此类型传递科目余额视图。
type SnapPosition struct {
	Book    string
	Subject string
	Asset   string
	Available string
	Frozen    string
}

// snapPosition 内部别名，兼容旧引用。

type snapPosition = SnapPosition

// DiffHops 核对两组 HopRecord 列表，返回变化条目。
// 用于漂移证明的人类可读差异字段。

   func DiffHops(sealed, live []HopRecord) []DriftEntry {
	  sealedMap := make(map[string]HopRecord, len(sealed))
	  for _, h := range sealed {
		   sealedMap[h.TransferID] = h
	  }
	  liveMap := make(map[string]HopRecord, len(live))
	  for _, h := range live {
		   liveMap[h.TransferID] = h
	   }
	   var entries []DriftEntry
	   for id, sh := range sealedMap {
		  if lh, ok := liveMap[id]; !ok {
			   entries = append(entries, DriftEntry{Kind: "hop", Ref: id, Note: "removed"})
		  } else if sh.AmountHex != lh.AmountHex {
			   entries = append(entries, DriftEntry{Kind: "hop", Ref: id, Note: "changed"})
		   }
	   }
	   for id := range liveMap {
		  if _, ok := sealedMap[id]; !ok {
			  entries = append(entries, DriftEntry{Kind: "hop", Ref: id, Note: "added"})
		   }
	   }
	   sort.Slice(entries, func(i, j int) bool { return entries[i].Ref < entries[j].Ref })
	   return entries
   }

// DiffBooks 核对两组 BookRecord 列表，返回变化条目。

   func DiffBooks(sealed, live []BookRecord) []DriftEntry {
	  bookKey := func(b BookRecord) string { return b.Book + ":" + b.Asset }
	   sealedMap := make(map[string]BookRecord, len(sealed))
	   for _, b := range sealed {
		   sealedMap[bookKey(b)] = b
	   }
	  liveMap := make(map[string]BookRecord,
		  len(live))
	   for _, b := range live {
		   liveMap[bookKey(b)] = b
	   }
	   var entries []DriftEntry
	  for k, sb := range sealedMap {
		  if lb, ok := liveMap[k]; !ok {
			   entries = append(entries, DriftEntry{Kind: "book", Ref: k, Note: "removed"})
		   } else if sb.Available != lb.Available || sb.Frozen != lb.Frozen {
			   entries = append(entries, DriftEntry{Kind: "book", Ref: k, Note: "changed"})
		  }
	  }
	   for k := range liveMap {
		  if _, ok := sealedMap[k]; !ok {
			   entries = append(entries, DriftEntry{Kind: "book", Ref: k, Note: "added"})
		  }
	  }
	  sort.Slice(entries, func(i, j int) bool { return entries[i].Ref < entries[j].Ref })
	   return entries
   }
