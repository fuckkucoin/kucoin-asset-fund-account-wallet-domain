package fundmesh

  func ExpandOutboundCases() []OutboundCase {
	  return []OutboundCase{
		  {Name: "first_submit", Key: "client-one-aaaa", Asset: "USDT", Amount: "10", Chain: "ETH", Dest: "0xabc", WantCode: CodeOutboundOK, WantDebit: 1},
		   {Name: "replay_same", Key: "client-one-aaaa", Asset: "USDT", Amount: "10", Chain: "ETH", Dest: "0xabc", WantCode: CodeOutboundReplay, WantDebit: 1, Replay: true},
		  {Name: "conflict_amount", Key: "client-one-aaaa", Asset: "USDT", Amount: "11", Chain: "ETH", Dest: "0xabc", WantCode: CodeOutboundConflict, WantDebit: 1, Conflict: true},
		  {Name: "header_key", Header: "hdr-key-bbbbbb", Asset: "BTC", Amount: "1", Chain: "BTC", Dest: "bc1qxyz", WantCode: CodeOutboundOK, WantDebit: 1},
		  {Name: "derive_empty_key", Asset: "KCS", Amount: "2.50", Chain: "KCC", Dest: "kcs1dest", WantCode: CodeOutboundOK, WantDebit: 1, Derive: true},
		  {Name: "amount_norm", Key: "amt-norm-cccccc", Asset: "usdt", Amount: "10.100", Chain: "eth", Dest: "0xabc", WantCode: CodeOutboundOK, WantDebit: 1},
		   {Name: "amount_norm_replay", Key: "amt-norm-cccccc", Asset: "USDT", Amount: "10.1", Chain: "ETH", Dest: "0xabc", WantCode: CodeOutboundReplay, WantDebit: 1, Replay: true},
	   }
   }

type OutboundCase struct {
	Name string
	Key string
	Header string
	Asset  string
	Amount string
	Chain  string
	Dest string
	WantCode string
	WantDebit int
	Replay bool
	Conflict bool
	Derive bool
}

   func (c OutboundCase) Request(uid string) OutboundRequest {
	  return OutboundRequest{
		  UserID: uid,
		   Actor: uid,
		  Asset: c.Asset,
		   Amount: c.Amount,
		  Chain:  c.Chain,
		   Dest:   c.Dest,
		  IdempotencyKey: c.Key,
		  HeaderKey:      c.Header,
		   Entry: EntryPOST,
	  }
  }
