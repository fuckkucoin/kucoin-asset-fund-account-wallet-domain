package withdraw

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"account-assets.kucoin.net/funding/internal/chainad"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

// SubmitOnChain 冻单之后广播；memo/tag/destination_tag 透传入 Outbound。
// 备注来自 transfers.memo_hint/tag_hint/tag_name_hint；哈希仍由适配器产生。
// FamMEMO 链空备注在 SubmitForWithdraw 处 fail-closed。
  func (s *Service) SubmitOnChain(ctx context.Context, reg *chainad.Registry, trID, chain, from, to, asset, amount, nonce, memo, tag, tagName string) (Broadcast, error) {
	   if s == nil || s.bc == nil {
		   return Broadcast{}, fmt.Errorf("withdraw broadcaster unavailable")
	   }
	  if reg == nil {
		   return Broadcast{}, chainad.ErrUnavailable
	   }
	  ch, err := validate.Chain(chain)
	   if err != nil {
		  return Broadcast{}, err
	  }
	   if _, err := money.Parse(amount); err != nil {
		  return Broadcast{}, err
	   }
	   if err := s.admitReserve(ctx, PhaseSubmit, strings.ToUpper(strings.TrimSpace(asset)), amount, trID); err != nil {
		  return Broadcast{}, err
	   }
	   if err := s.checkTravelBroadcast(trID); err != nil {
		  return Broadcast{}, err
	  }
	  fam, _ := chainad.FamilyOf(ch)
	  var n uint64
	   hasNum := false
	   if fam == chainad.FamETH && strings.TrimSpace(nonce) != "" {
		   if v, err := strconv.ParseUint(strings.TrimSpace(nonce), 10, 64); err == nil {
			   n, hasNum = v, true
		   }
	  }
	   out := chainad.Outbound{
		   Chain: ch, From: from, To: to, Asset: asset, Amount: amount, Nonce: nonce,
		   Memo: memo, Tag: tag, TagName: tagName,
	   }
	   chainad.NormalizeOutboundMemo(&out)
	   rec, err := chainad.SubmitForWithdraw(ctx, mustSubmitter(reg, ch), out)
	  if err != nil {
		  return Broadcast{}, err
	  }
	  if hasNum && strings.TrimSpace(from) != "" {
		   if err := s.Nonces().Reserve(ch, from, n, rec.Hash); err != nil && !strings.Contains(err.Error(), "nonce reused") {
			   return Broadcast{}, err
		  }
	   }
	  return s.bc.Track(trID, ch, rec.Hash, asset, amount)
   }

  func mustSubmitter(reg *chainad.Registry, chain string) chainad.Submitter {
	   a, err := reg.Get(chain)
	   if err != nil {
		   return nil
	  }
	   return a
  }

// SubmitOnChainFromHints 用账本 memo_hint/tag_hint/tag_name_hint 广播。
  func (s *Service) SubmitOnChainFromHints(ctx context.Context, reg *chainad.Registry, trID, chain, from, to, asset, amount, nonce, memoHint, tagHint, tagNameHint string) (Broadcast, error) {
	   return s.SubmitOnChain(ctx, reg, trID, chain, from, to, asset, amount, nonce, memoHint, tagHint, tagNameHint)
  }
