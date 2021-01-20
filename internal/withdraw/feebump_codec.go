package withdraw

import (
	"context"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/chainad"
	"account-assets.kucoin.net/funding/internal/validate"
)

// BumpHints 抬价 HTTP/重提透传 destination_tag 的入参面。

type BumpHints struct {
	Memo string
	Tag  string
	TagName        string
	DestinationTag string
}

  func (h BumpHints) toChain() chainad.FeeBumpHints {
	   return chainad.FeeBumpHints{
		   Memo: h.Memo,
		  Tag: h.Tag, TagName: h.TagName, DestinationTag: h.DestinationTag,
	   }
  }

// BumpWithHints 登记抬价并落 destination_tag（独立编解码面材料）。
// 备注归一后写入 FeeBump；与主广播同形，供重签/重提。
// 空备注不臆造；FamMEMO 链重提前仍须 CheckMemo。

  func (b *Bumper) BumpWithHints(trID, chain, oldHash, oldFee, newFee, newHash string, hints BumpHints) (FeeBump, error) {
	  row,
		  err := b.Bump(trID, chain, oldHash, oldFee, newFee, newHash)
	  if err != nil {
		  return row, err
	   }
	   chHints := chainad.ResolveFeeBumpHints(hints.toChain())
	   if chHints.TagName == "" {
		   chHints.TagName = tagNameForChain(chain)
	   }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  if cur, ok := b.rows[trID]; ok {
		  cur.Memo = chHints.Memo
		   cur.Tag = chHints.Tag
		   cur.TagName = chHints.TagName
		  cur.DestinationTag = chHints.DestinationTag
		   row = *cur
		   if err := b.persistLocked(row); err != nil {
			  return row, err
		   }
	   }
	  return row, nil
  }

   func tagNameForChain(chain string) string {
	   ch, err := validate.Chain(chain)
	   if err != nil {
		  return ""
	   }
	  // 复用 chainad ResolveDestinationTag 的空链推导面。
	   _, name := chainad.ResolveDestinationTag(ch, "", "", "")
	  return name
  }

// BuildResubmitOutbound 抬价/RBF 独立编解码——从已登记抬价行 + 地址金额重建 Outbound。
// destination_tag 取自抬价行；缺则用入参 hints。
// 不得用空 hints 覆盖已落库非空 destination_tag。

  func (b *Bumper) BuildResubmitOutbound(trID, from, to, asset, amount string, hints BumpHints) (chainad.Outbound, FeeBump, error) {
	   if b == nil {
		   return chainad.Outbound{}, FeeBump{}, fmt.Errorf("fee bumper unavailable")
	   }
	  row, ok := b.Get(trID)
	   if !ok {
		   return chainad.Outbound{}, FeeBump{}, fmt.Errorf("fee bump not tracked")
	  }
	   merged := BumpHints{
		   Memo: firstNonEmpty(hints.Memo, row.Memo),
		   Tag:            firstNonEmpty(hints.Tag, row.Tag),
		   TagName:        firstNonEmpty(hints.TagName, row.TagName),
		  DestinationTag: firstNonEmpty(hints.DestinationTag, row.DestinationTag),
	   }
	  base := chainad.Outbound{
		   Chain: row.Chain, From: from, To: to, Asset: asset, Amount: amount,
	   }
	  out, err := chainad.BuildFeeBumpOutbound(base, row.NewFee, merged.toChain())
	   if err != nil {
		  return chainad.Outbound{}, row, err
	   }
	  return out, row, nil
  }

// SubmitFeeBump 抬价独立编解码面广播；Outbound 透传 destination_tag。
// 先 BuildResubmitOutbound，再 SubmitForWithdraw；成功后更新 new_hash。
// FamMEMO 空备注 fail-closed；新 hash 不得等于 old_hash。
  func (s *Service) SubmitFeeBump(ctx context.Context, reg *chainad.Registry, trID, from, to, asset, amount string, hints BumpHints) (FeeBump, chainad.Receipt, error) {
	   if s == nil {
		   return FeeBump{}, chainad.Receipt{}, fmt.Errorf("withdraw service unavailable")
	  }
	  out, row, err := s.Bumper().BuildResubmitOutbound(trID, from, to, asset, amount, hints)
	  if err != nil {
		   return row, chainad.Receipt{}, err
	  }
	   if reg == nil {
		  return row, chainad.Receipt{}, chainad.ErrUnavailable
	  }
	   rec, err := chainad.SubmitForWithdraw(ctx, mustSubmitter(reg, out.Chain), out)
	   if err != nil {
		   return row, chainad.Receipt{}, err
	   }
	   if strings.TrimSpace(rec.Hash) == "" {
		   return row, rec, chainad.ErrSubmit
	  }
	  if strings.EqualFold(strings.TrimSpace(rec.Hash), strings.TrimSpace(row.OldHash)) {
		  return row, rec, fmt.Errorf("fee bump refused: replacement hash matches old")
	  }
	  updated, err := s.Bumper().BumpWithHints(trID, row.Chain, row.OldHash, row.OldFee, row.NewFee, rec.Hash, BumpHints{
		   Memo: out.Memo, Tag: out.Tag, TagName: out.TagName, DestinationTag: chainad.FeeBumpDestinationTag(out),
	   })
	  if err != nil {
		   return updated, rec, err
	  }
	   if s.bc != nil {
		  _, _ = s.bc.Track(trID, out.Chain, rec.Hash, asset, amount)
	   }
	   return updated, rec, nil
  }
