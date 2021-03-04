package auditchain

import "sort"

const (
	EffNone = "none"
	EffCredit = "credit"
	EffDebit  = "debit"
	EffFreeze = "freeze"
	EffUnfreeze = "unfreeze"
	EffOut      = "xfer_out"
	EffIn       = "xfer_in"
)

const (
	KindAttribMemo = "deposit.attrib.memo"
	KindAttribTag  = "deposit.attrib.tag"
	KindAttribUTXO = "deposit.attrib.utxo"
	KindAttribAddr = "deposit.attrib.address"
	KindAttribNoise = "deposit.attrib.noise"
	KindDepositCredit = "deposit.credit"
	KindWdFreeze   = "withdraw.freeze"
	KindWdUnfreeze = "withdraw.unfreeze"
	KindWdPartial  = "withdraw.partial_unfreeze"
	KindWdBroadcast = "withdraw.broadcast"
	KindCtlFreeze  = "ledger.control.freeze"
	KindCtlRelease = "ledger.control.release"
	KindCoverUndo  = "ledger.cover.undo"
	KindCoverFail = "ledger.cover.fail"
	KindSweepProof = "sweep.proof.ref"
	KindHopOut = "xfer.hop.out"
	KindHopIn  = "xfer.hop.in"
	KindInnerOut = "xfer.inner.out"
	KindInnerIn = "xfer.inner.in"
	KindReconRef = "recon.statement.ref"
)

// KindSpec 一种信封。Moves 为真时金额必须为正，重放才会动余额。
type KindSpec struct {
	Kind   string `json:"kind"`
	Domain string `json:"domain"`
	Effect string `json:"effect"`
	Moves bool `json:"moves"`
	Note string `json:"note"`
}

  func catalog() []KindSpec {
	  return []KindSpec{
		  {KindAttribMemo, "deposit", EffNone, false, "memo 归因结论"},
		   {KindAttribTag, "deposit", EffNone, false, "tag 归因结论"},
		   {KindAttribUTXO, "deposit", EffNone, false, "utxo 归因结论"},
		  {KindAttribAddr, "deposit", EffNone, false, "地址归因结论"},
		  {KindAttribNoise, "deposit", EffNone, false, "未能归因的噪声"},
		  {KindDepositCredit, "deposit", EffCredit, true, "入金贷记结论"},
		  {KindWdFreeze, "withdraw", EffFreeze, true, "出金冻结事实"},
		  {KindWdUnfreeze, "withdraw", EffUnfreeze, true, "出金解冻事实"},
		   {KindWdPartial, "withdraw", EffUnfreeze, true, "部分解冻"},
		  {KindWdBroadcast, "withdraw", EffNone, false, "已广播事实，不改可用"},
		   {KindCtlFreeze, "ledger", EffNone, false, "用户控制位冻结"},
		  {KindCtlRelease, "ledger", EffNone, false, "用户控制位解开"},
		  {KindCoverUndo, "ledger", EffNone, false, "补账清扫回冲引用"},
		  {KindCoverFail, "ledger", EffNone, false, "补账清扫失败"},
		   {KindSweepProof, "sweep", EffNone, false, "归集证明引用，不占归集表"},
		  {KindHopOut, "xfer", EffOut, true, "母子 hop 转出"},
		   {KindHopIn, "xfer", EffIn, true, "母子 hop 转入"},
		   {KindInnerOut, "xfer", EffOut, true, "内部转转出"},
		   {KindInnerIn, "xfer", EffIn, true, "内部转转入"},
		  {KindReconRef, "recon", EffNone, false, "三方对账单号引用"},
	   }
   }

var kindIndex = indexKinds()

  func indexKinds() map[string]KindSpec {
	   m := map[string]KindSpec{}
	  for _, s := range catalog() {
		  m[s.Kind] = s
	   }
	   return m
  }

// LookupKind 未知种类拒。摄取不得临时造 kind。
   func LookupKind(kind string) (KindSpec, error) {
	  s, ok := kindIndex[kind]
	   if !ok {
		  return KindSpec{}, ErrKind
	  }
	  return s, nil
  }

// EffectOf 重放机只认目录里的效果位。

   func EffectOf(kind string) (string, error) {
	  s, err := LookupKind(kind)
	  if err != nil {
		  return "", err
	  }
	   return s.Effect, nil
  }

// Kinds 目录副本，给查询接口和文档。

  func Kinds() []KindSpec {
	   src := catalog()
	  out := make([]KindSpec, len(src))
	   copy(out, src)
	  sort.Slice(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	  return out
  }

// DomainOf 分区用的域。未知 kind 已在 Lookup 拒绝。
   func DomainOf(kind string) (string, error) {
	  s, err := LookupKind(kind)
	   if err != nil {
		   return "", err
	  }
	   return s.Domain, nil
  }

// MovesBalance 金额缺失时，不动余额的种类仍可入库。
  func MovesBalance(kind string) bool {
	  s, ok := kindIndex[kind]
	   return ok && s.Moves
  }
