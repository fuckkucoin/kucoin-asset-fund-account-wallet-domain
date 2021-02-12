package ingest

import "time"

// Fact 跨域摄取的一条结论。Kind 由审计仓目录校验。

type Fact struct {
	Source string
	IdemKey string
	Kind    string
	Domain  string
	Tenant string
	UserID string
	Asset      string
	Amount     string
	RefID      string
	Actor      string
	Body       string
	OccurredAt time.Time
}

// DepositDecision 归因结论。noise 为真时种类走噪声，金额可以为 0。

  func DepositDecision(user, chain, kind, key, asset, amount, ref, actor string, noise bool, at time.Time) Fact {
	   k := "deposit.attrib.address"
	  switch kind {
	  case "memo":
		   k = "deposit.attrib.memo"
	   case "tag":
		   k = "deposit.attrib.tag"
	   case "utxo":
		   k = "deposit.attrib.utxo"
	  case "address", "":
		   k = "deposit.attrib.address"
	  }
	  if noise {
		   k = "deposit.attrib.noise"
	  }
	   if actor == "" {
		  actor = "deposit-desk"
	   }
	   if amount == "" {
		  amount = "0"
	  }
	   return Fact{
		   Source: "deposit.attrib", IdemKey: refOr(ref, user+"|"+chain+"|"+kind+"|"+key), Kind: k, Domain: "deposit",
		   UserID: user, Asset: asset, Amount: amount, RefID: ref, Actor: actor, OccurredAt: at,
		  Body: bodyPairs("chain", chain, "kind", kind, "key", key, "noise", noise),
	   }
   }

// CreditFact 入金贷记结论。idem 用链上引用，重复确认不会再记一条。

   func CreditFact(user, asset, amount, ref, actor string, at time.Time) Fact {
	   if actor == "" {
		  actor = "deposit-desk"
	  }
	   return Fact{
		   Source: "deposit.credit", IdemKey: "credit|" + ref, Kind: "deposit.credit", Domain: "deposit",
		  UserID: user, Asset: asset, Amount: amount, RefID: ref, Actor: actor, OccurredAt: at,
		  Body: bodyPairs("ref", ref),
	  }
  }

// FreezeFact 出金冻结跃迁。只有进入 held 才冻可用；取消和广播前失败才解冻。
// 广播之后的失败不记解冻。筛查、签名和广播本身金额记 0，重放当标记，避免同一笔冻多次。

   func FreezeFact(user, asset, amount, ref, from, to, actor string, at time.Time) Fact {
	  kind := "withdraw.freeze"
	  move := to == "held" && (from == "" || from == "none")
	   switch to {
	   case "broadcast", "confirming", "succeeded":
		  kind = "withdraw.broadcast"
		   move = false
	   case "cancelled":
		  kind = "withdraw.unfreeze"
		   move = true
	  case "failed":
		   switch from {
		   case "broadcast", "confirming", "succeeded":
			   kind = "withdraw.broadcast"
			   move = false
		   default:
			   kind = "withdraw.unfreeze"
			  move = true
		  }
	   }
	   if !move {
		  amount = "0"
	  }
	  if actor == "" {
		   actor = "withdraw-desk"
	   }
	   if amount == "" {
		   amount = "0"
	   }
	   return Fact{
		   Source: "withdraw.freeze", IdemKey: ref + "|" + from + ">" + to, Kind: kind, Domain: "withdraw",
		  UserID: user,
		  Asset:  asset, Amount: amount, RefID: ref, Actor: actor, OccurredAt: at,
		   Body: bodyPairs("from", from, "to", to),
	  }
  }

// PartialUnfreeze 部分解冻。金额是解开的那一截，不是剩余冻结。

  func PartialUnfreeze(user, asset, amount, ref, actor string, at time.Time) Fact {
	   if actor == "" {
		  actor = "withdraw-desk"
	   }
	   return Fact{
		   Source: "withdraw.freeze", IdemKey: "partial|" + ref + "|" + amount, Kind: "withdraw.partial_unfreeze", Domain: "withdraw",
		   UserID: user,
		   Asset:  asset, Amount: amount, RefID: ref, Actor: actor, OccurredAt: at,
		  Body: bodyPairs("ref", ref),
	  }
  }

// ControlFact 用户控制位。不改可用余额。

  func ControlFact(user, kind, actor string, at time.Time) Fact {
	   if actor == "" {
		  actor = "ledger-ops"
	  }
	   return Fact{
		  Source: "ledger.control", IdemKey: kind + "|" + user + "|" + at.UTC().Format(time.RFC3339Nano), Kind: kind, Domain: "ledger",
		   UserID: user, Amount: "0", Actor: actor, OccurredAt: at, Body: bodyPairs("user", user),
	   }
   }

// CoverFact 补账清扫引用。
  func CoverFact(user, asset, amount, ref, kind, actor string, at time.Time) Fact {
	   if actor == "" {
		  actor = "cover-sweeper"
	  }
	   if amount == "" {
		  amount = "0"
	  }
	  return Fact{
		  Source: "ledger.cover", IdemKey: kind + "|" + ref, Kind: kind, Domain: "ledger",
		  UserID: user, Asset: asset, Amount: amount, RefID: ref, Actor: actor, OccurredAt: at,
		  Body: bodyPairs("ref", ref),
	   }
   }

// SweepProof 归集证明只记引用号，不复制归集证明表。

  func SweepProof(chain, asset, amount, proofID, actor string, at time.Time) Fact {
	  if actor == "" {
		   actor = "sweep-orch"
	  }
	   if amount == "" {
		   amount = "0"
	   }
	  return Fact{
		  Source: "sweep.proof", IdemKey: "proof|" + proofID, Kind: "sweep.proof.ref", Domain: "sweep",
		   Asset: asset, Amount: amount, RefID: proofID, Actor: actor, OccurredAt: at,
		  Body: bodyPairs("chain", chain, "proof_id", proofID),
	  }
   }

// HopFact 母子 hop 的一侧。out 为真记转出用户。

   func HopFact(user, other, asset, amount, hopID string, out bool, at time.Time) Fact {
	  kind := "xfer.hop.in"
	   if out {
		   kind = "xfer.hop.out"
	   }
	  return Fact{
		   Source: "xfer.hop", IdemKey: hopID + "|" + kind + "|" + user, Kind: kind, Domain: "xfer",
		  UserID: user, Asset: asset, Amount: amount, RefID: hopID, Actor: "hop-desk", OccurredAt: at,
		   Body: bodyPairs("other", other, "hop", hopID),
	   }
  }

// InnerFact 内部转一侧。
  func InnerFact(user, other, asset, amount, ref string, out bool, at time.Time) Fact {
	  kind := "xfer.inner.in"
	  if out {
		   kind = "xfer.inner.out"
	   }
	   return Fact{
		   Source: "xfer.inner", IdemKey: ref + "|" + kind + "|" + user, Kind: kind, Domain: "xfer",
		   UserID: user, Asset: asset, Amount: amount, RefID: ref, Actor: "xfer-desk", OccurredAt: at,
		  Body: bodyPairs("other", other),
	  }
  }

// ReconFact 三方对账单号。公式不在这里。
  func ReconFact(user, asset, ticketID, actor, note string, at time.Time) Fact {
	  if actor == "" {
		  actor = "recon-desk"
	  }
	   return Fact{
		   Source: "recon.statement", IdemKey: "ticket|" + ticketID, Kind: "recon.statement.ref", Domain: "recon",
		   UserID: user, Asset: asset, Amount: "0", RefID: ticketID, Actor: actor, OccurredAt: at,
		  Body: bodyPairs("note", note),
	  }
  }

  func refOr(ref, fallback string) string {
	  if ref != "" {
		   return ref
	   }
	  return fallback
  }
