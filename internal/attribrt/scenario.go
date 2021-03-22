package attribrt

import "time"

// Fixture 一条场景应得到的入账意图和原因。

type Fixture struct {
	Name   string `json:"name"`
	Credit bool   `json:"credit"`
	Reason string `json:"reason"`
	Disp string `json:"disposition"`
}

// Fixtures 场景清单。原因码必须出现在结论里。
   func Fixtures() []Fixture {
	  return []Fixture{
		  {Name: "unbound_memo", Credit: false, Reason: "unbound", Disp: "desk"},
		  {Name: "dust_drop", Credit: false, Reason: "noise_dust", Disp: "drop"},
		   {Name: "self_send", Credit: false, Reason: "noise_self", Disp: "drop"},
		   {Name: "utxo_change", Credit: false, Reason: "utxo_change", Disp: "drop"},
		   {Name: "utxo_payment", Credit: true, Reason: "", Disp: "pass"},
		  {Name: "utxo_conserve", Credit: false, Reason: "utxo_conserve", Disp: "desk"},
		  {Name: "utxo_unbound", Credit: false, Reason: "utxo_unbound", Disp: "desk"},
		   {Name: "memo_collision", Credit: false, Reason: "collide_blocked", Disp: "desk"},
		  {Name: "memo_released", Credit: true, Reason: "", Disp: "pass"},
		  {Name: "memo_loser", Credit: false, Reason: "collide_blocked", Disp: "desk"},
		  {Name: "unknown_version", Credit: false, Reason: "unknown_version", Disp: "desk"},
		   {Name: "cross_family", Credit: false, Reason: "policy_family", Disp: "desk"},
		   {Name: "stale_policy", Credit: false, Reason: "policy_stale", Disp: "desk"},
		  {Name: "stale_replay", Credit: true, Reason: "", Disp: "pass"},
		   {Name: "homoglyph_memo", Credit: false, Reason: "noise_homoglyph", Disp: "desk"},
		  {Name: "leading_zero_tag", Credit: false, Reason: "collide_blocked", Disp: "desk"},
		   {Name: "utxo_merge", Credit: false, Reason: "utxo_merge", Disp: "desk"},
	   }
  }

type mapBind map[string]string

  func (m mapBind) Lookup(chain, kind, key string) (string, bool) {
	  u, ok := m[chain+"|"+kind+"|"+key]
	  return u, ok
  }

// ReplayScenario 跑一个场景。结论与清单不一致时返回错误。
   func ReplayScenario(name string) (Decision, error) {
	   fix, ok := fixtureBy(name)
	   if !ok {
		   return Decision{}, ErrScene
	  }
	  d, err := runScene(name)
	  if err != nil {
		   return d, err
	  }
	   if d.CreditIntent != fix.Credit || d.Disposition != fix.Disp {
		  return d, &DecisionError{Codes: []string{"scenario_mismatch", name}}
	  }
	   if fix.Reason != "" && !hasReason(d.Reasons, fix.Reason) {
		   return d, &DecisionError{Codes: append([]string{"scenario_mismatch", name}, d.Reasons...)}
	  }
	  return d, nil
  }

// ReplayAll 按清单跑完全部场景。
  func ReplayAll() (map[string]Decision, error) {
	  out := map[string]Decision{}
	  for _, f := range Fixtures() {
		  d, err := ReplayScenario(f.Name)
		  out[f.Name] = d
		   if err != nil {
			  return out, err
		  }
	  }
	  return out, nil
   }

   func fixtureBy(name string) (Fixture, bool) {
	  for _, f := range Fixtures() {
		   if f.Name == name {
			  return f, true
		   }
	  }
	  return Fixture{}, false
   }

  func runScene(name string) (Decision, error) {
	  switch name {
	   case "unbound_memo":
		   return sceneUnbound()
	  case "dust_drop":
		   return sceneDust()
	  case "self_send":
		   return sceneSelf()
	   case "utxo_change":
		  return sceneChange(0)
	   case "utxo_payment":
		   return sceneChange(1)
	  case "utxo_conserve":
		   return sceneConserve()
	   case "utxo_unbound":
		  return sceneUnboundCoin()
	  case "memo_collision":
		  _, d, err := sceneCollision(false)
		   return d, err
	   case "memo_released":
		  d, _, err := sceneReleased(true)
		   return d, err
	   case "memo_loser":
		   _, d, err := sceneReleased(false)
		  return d, err
	  case "unknown_version":
		  return sceneVersion("XRP", "fam-memo-2018.01", false)
	  case "cross_family":
		   return sceneVersion("XRP", "fam-utxo-2024.11", false)
	  case "stale_policy":
		  return sceneVersion("XRP", "fam-memo-2020.09", false)
	   case "stale_replay":
		  return sceneVersion("XRP", "fam-memo-2020.09", true)
	   case "homoglyph_memo":
		   return sceneHomoglyph()
	  case "leading_zero_tag":
		   return sceneLeadingZero()
	   case "utxo_merge":
		   return sceneMerge()
	  default:
		  return Decision{}, ErrScene
	   }
   }

   func newScene(b Binder) *Engine {
	   e, err := New()
	  if err != nil {
		  panic(err)
	  }
	  e.SetClock(func() time.Time { return time.Date(2024, 11, 2, 8, 0, 0, 0, time.UTC) })
	   e.SetBinder(b)
	   return e
   }

  func sceneUnbound() (Decision, error) {
	   e := newScene(mapBind{})
	  return e.Resolve(Sight{
		  Chain: "XRP", TxHash: "aabbccdd11223344", Amount: "10", Asset: "XRP",
		   To: "rSharedAddrExample001", Memo: "999001",
	  })
   }

   func sceneDust() (Decision, error) {
	   e := newScene(mapBind{})
	  return e.Resolve(Sight{
		  Chain: "ETH", TxHash: "aabbccdd11223345", Amount: "0.00000001", Asset: "ETH",
		  From: "0xfrom", To: "0xto", UserHint: "user-eth",
	   })
   }

   func sceneSelf() (Decision, error) {
	  e := newScene(mapBind{})
	  return e.Resolve(Sight{
		   Chain: "ETH", TxHash: "aabbccdd11223346", Amount: "1.5", Asset: "ETH",
		  From: "0xabc", To: "0xabc", UserHint: "user-eth",
	   })
  }

  func btcGraph() *GraphIn {
	  return &GraphIn{
		  Inputs: []CoinIn{{TxID: "inprev0001", Vout: 0, Amount: "1.00000000", Address: "bc1qchangeaddr"}},
		   Outputs: []CoinIn{
			  {Vout: 0, Amount: "0.40000000", Address: "bc1qchangeaddr", Script: "p2wpkh"},
			   {Vout: 1, Amount: "0.59990000", Address: "bc1quserpay", Script: "p2wpkh"},
		   },
	  }
   }

  func sceneChange(vout int) (Decision, error) {
	   e := newScene(mapBind{"BTC|address|bc1quserpay": "user-btc"})
	   return e.Resolve(Sight{
		  Chain: "BTC", TxHash: "aabbccdd11223347", OutputIndex: vout,
		  Amount: "0.59990000", Asset: "BTC", To: "bc1quserpay", Graph: btcGraph(),
	  })
  }

  func sceneConserve() (Decision, error) {
	  e := newScene(mapBind{"BTC|address|bc1quserpay": "user-btc"})
	  return e.Resolve(Sight{
		   Chain: "BTC", TxHash: "aabbccdd11223348", OutputIndex: 0, Amount: "1", Asset: "BTC",
		   Graph: &GraphIn{
			  Inputs:  []CoinIn{{TxID: "inprev0002", Vout: 0, Amount: "1.00000000", Address: "bc1qext"}},
			   Outputs: []CoinIn{{Vout: 0, Amount: "1.00000000", Address: "bc1quserpay", Script: "p2wpkh"}},
		   },
	   })
  }

   func sceneUnboundCoin() (Decision, error) {
	   e := newScene(mapBind{})
	  return e.Resolve(Sight{
		   Chain: "BTC", TxHash: "aabbccdd11223349", OutputIndex: 0, Amount: "0.5", Asset: "BTC",
		  Graph: &GraphIn{
			  Inputs: []CoinIn{{TxID: "inprev0003", Vout: 0, Amount: "0.50010000", Address: "bc1qextpayer"}},
			   Outputs: []CoinIn{{Vout: 0, Amount: "0.50000000", Address: "bc1qnobody", Script: "p2wpkh"}},
		  },
	   })
  }

  func sceneCollision(second bool) (*Engine, Decision, error) {
	   b := mapBind{
		   "XRP|memo|12345": "user-a",
	   }
	   e := newScene(b)
	  s := Sight{
		   Chain: "XRP", TxHash: "aabbccdd11223350", Amount: "20", Asset: "XRP",
		  To: "rSharedAddrExample001", Memo: "12345",
	  }
	  if _, err := e.Resolve(s); err != nil {
		  return e, Decision{}, err
	   }
	   e.SetBinder(mapBind{"XRP|memo|12345": "user-b"})
	  s.TxHash = "aabbccdd11223351"
	   d, err := e.Resolve(s)
	  if second {
		   return e, d, err
	   }
	   return e, d, err
  }

  func sceneReleased(winner bool) (Decision, Decision, error) {
	   e, blocked, err := sceneCollision(true)
	   if err != nil {
		   return Decision{}, blocked, err
	   }
	   if blocked.CollisionID == "" {
		   return Decision{}, blocked, &DecisionError{Codes: []string{"scenario_mismatch", "no_case"}}
	   }
	  if _, err := e.Assign(blocked.CollisionID, "ops-lin", "desk take"); err != nil {
		  return Decision{}, blocked, err
	  }
	  if _, err := e.Pick(blocked.CollisionID, "ops-lin", "user-a", "payer matches user-a"); err != nil {
		  return Decision{}, blocked, err
	  }
	   e.SetBinder(mapBind{"XRP|memo|12345": "user-a"})
	  win, err := e.Resolve(Sight{
		   Chain: "XRP", TxHash: "aabbccdd11223352", Amount: "20", Asset: "XRP",
		  To: "rSharedAddrExample001", Memo: "12345",
	  })
	  if err != nil {
		  return win, Decision{}, err
	   }
	  e.SetBinder(mapBind{"XRP|memo|12345": "user-b"})
	  lose, err := e.Resolve(Sight{
		  Chain: "XRP", TxHash: "aabbccdd11223353", Amount: "21", Asset: "XRP",
		  To: "rSharedAddrExample001", Memo: "12345",
	  })
	   if winner {
		   return win, lose, err
	   }
	  return win, lose, err
  }

   func sceneVersion(chain, ver string, replay bool) (Decision, error) {
	  e := newScene(mapBind{"XRP|memo|42": "user-a"})
	   return e.Resolve(Sight{
		   Chain: chain, TxHash: "aabbccdd11223354", Amount: "5", Asset: "XRP",
		  To: "rSharedAddrExample001", Memo: "42", PolicyVersion: ver, Replay: replay,
	  })
  }

   func sceneHomoglyph() (Decision, error) {
	  // 西里尔小写 a，折叠后不再等于原文。
	  memo := "p" + "\u0430" + "y"
	   e := newScene(mapBind{"XLM|memo|" + memo: "user-xlm"})
	  return e.Resolve(Sight{
		   Chain: "XLM", TxHash: "aabbccdd11223355", Amount: "3", Asset: "XLM",
		  To: "GSHAREDXLM", Memo: memo,
	   })
  }

   func sceneLeadingZero() (Decision, error) {
	   e := newScene(mapBind{"XRP|tag|00123": "user-a"})
	  if _, err := e.Resolve(Sight{
		  Chain: "XRP", TxHash: "aabbccdd11223356", Amount: "8", Asset: "XRP",
		   To: "rSharedAddrExample001", Tag: "00123",
	   }); err != nil {
		   return Decision{}, err
	   }
	   e.SetBinder(mapBind{"XRP|tag|123": "user-b"})
	  return e.Resolve(Sight{
		   Chain: "XRP", TxHash: "aabbccdd11223357", Amount: "8", Asset: "XRP",
		   To: "rSharedAddrExample001", Tag: "123",
	  })
  }

   func sceneMerge() (Decision, error) {
	  e := newScene(mapBind{
		  "BTC|address|bc1quserone": "user-1",
		   "BTC|address|bc1qusertwo": "user-2",
		   "BTC|address|bc1quserpay": "user-1",
	   })
	   return e.Resolve(Sight{
		   Chain: "BTC", TxHash: "aabbccdd11223358", OutputIndex: 0, Amount: "0.4", Asset: "BTC",
		  Graph: &GraphIn{
			  Inputs: []CoinIn{
				   {TxID: "in11111111", Vout: 0, Amount: "0.50000000", Address: "bc1quserone"},
				  {TxID: "in22222222", Vout: 1, Amount: "0.50000000", Address: "bc1qusertwo"},
			   },
			   Outputs: []CoinIn{
				   {Vout: 0, Amount: "0.40000000", Address: "bc1quserpay", Script: "p2wpkh"},
				   {Vout: 1, Amount: "0.59900000", Address: "bc1qchangeaddr", Script: "p2wpkh"},
			  },
		  },
	  })
  }
