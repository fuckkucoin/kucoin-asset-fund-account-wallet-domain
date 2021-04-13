package depoccup

import "time"

   func (s *Service) UseDefaults() {
	  if s == nil {
		   return
	  }
	  rows := BuildDefaults()
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   for i := range rows {
		   row := rows[i]
		  if err := validatePolicy(row); err != nil {
			   continue
		   }
		  cp := row
		   s.policies = append(s.policies, &cp)
		  s.byKey[cp.key()] = &cp
	   }
   }

   func BuildDefaults() []Policy {
	   kb := kycBase()
	  sb := sourceBase()
	  cw := chainWeights()
	  aw := assetWeights()
	  chains := make([]string, 0, len(cw)+1)
	  chains = append(chains, "*")
	  for c := range cw {
		  chains = append(chains, c)
	   }
	  assets := make([]string, 0, len(aw)+1)
	   assets = append(assets, "*")
	  for a := range aw {
		   assets = append(assets, a)
	   }
	   kycs := []string{KYCNone, KYCBasic, KYCAdvanced}
	   sources := []Source{SourceSelf, SourceVASP, SourceExchange, SourceContract, SourceUnknown, SourceMixer}
	  out := make([]Policy, 0, len(kycs)*len(sources)*len(chains)*len(assets))
	  for _, kyc := range kycs {
		  for _, src := range sources {
			  for _, chain := range chains {
				   for _, asset := range assets {
					   out = append(out, composePolicy(kyc, src, chain, asset, kb, sb, cw, aw))
				   }
			  }
		  }
	  }
	   out = append(out, overridePolicies()...)
	   return out
   }

   func composePolicy(kyc string, src Source, chain, asset string, kb map[string]struct {
	   BPS int
	  Win time.Duration
  }, sb map[Source]struct {
	  BPS int
	  Mul int
   }, cw map[string]chainWeight, aw map[string]assetWeight) Policy {
	  base := kb[kyc]
	  srcb := sb[src]
	   bps := base.BPS + srcb.BPS
	  win := base.Win * time.Duration(srcb.Mul)
	   if chain != "*" {
		   if w, ok := cw[chain]; ok {
			   bps += w.AddBPS
			   win += w.AddWin
		  }
	   }
	  klass := ""
	   if asset != "*" {
		   if w, ok := aw[asset]; ok {
			   bps += w.AddBPS
			  klass = w.Class
			  if w.Class == "stable" {
				   bps += 200
			   }
			   if w.Class == "meme" {
				   bps += 300
				  win += time.Hour
			   }
		  }
	  }
	   if stableOnHotChain(asset, chain) {
		  bps += 400
		  win += 30 * time.Minute
	   }
	  if kyc == KYCNone && src == SourceUnknown && bps < 8000 {
		   bps = 8000
	  }
	   if kyc == KYCAdvanced && src == SourceSelf && asset == "BTC" && (chain == "BTC" || chain == "*") && bps > 800 {
		   bps = 800
	   }
	  if src == SourceMixer {
		   bps = MaxRatioBPS
		  win = 72 * time.Hour
	   }
	   bps = clampBPS(bps)
	  win = clampWindow(win)
	   floor := TierT0
	   switch {
	  case src == SourceMixer:
		   floor = TierT3
	  case src == SourceUnknown && kyc == KYCNone:
		   floor = TierT2
	  case klass == "meme":
		  floor = TierT1
	   case src == SourceContract && kyc != KYCAdvanced:
		  floor = TierT1
	   }
	  p := Policy{
		   KYC: kyc, Asset: asset, Source: src, Chain: chain,
		  RatioBPS: bps, MaxWindow: win, MinWindow: 0,
		   MaxSec: int(win / time.Second), TierFloor: floor,
		   Version: 1, Priority: 0, Enabled: true, Note: "catalog",
	  }
	   p.ID = "cat-" + shortKey(p.key())
	   return p
   }

  func stableOnHotChain(asset, chain string) bool {
	  if asset != "USDT" && asset != "USDC" && asset != "DAI" {
		  return false
	   }
	   switch chain {
	  case "TRON", "BSC", "SOL", "TON":
		  return true
	   default:
		   return false
	   }
   }

  func overridePolicies() []Policy {
	  type row struct {
		   kyc, asset, chain string
		   src               Source
		   bps, maxSec       int
		   floor             Tier
		  note string
	  }
	  raw := []row{
		  {KYCAdvanced, "BTC", "BTC", SourceSelf, 300, 20 * 60, TierT0, "self-mined-btc"},
		  {KYCAdvanced, "BTC", "ETH", SourceSelf, 400, 30 * 60, TierT0, "wrapped-self"},
		  {KYCAdvanced, "ETH", "ETH", SourceVASP, 800, 45 * 60, TierT0, "vasp-eth"},
		   {KYCAdvanced, "USDT", "ETH", SourceVASP, 1200, 60 * 60, TierT0, "vasp-usdt-eth"},
		   {KYCAdvanced, "USDT", "TRON", SourceExchange, 3500, 3 * 60 * 60, TierT1, "tron-usdt-cex"},
		  {KYCAdvanced, "USDT", "TRON", SourceUnknown, 7000, 8 * 60 * 60, TierT2, "tron-usdt-unknown"},
		  {KYCBasic, "USDT", "TRON", SourceUnknown, 8500, 12 * 60 * 60, TierT2, "basic-tron-usdt"},
		  {KYCBasic, "USDT", "BSC", SourceUnknown, 8500, 12 * 60 * 60, TierT2, "basic-bsc-usdt"},
		  {KYCNone, "USDT", "TRON", SourceExchange, 9000, 18 * 60 * 60, TierT3, "none-tron-usdt"},
		   {KYCNone, "USDC", "SOL", SourceUnknown, 9000, 16 * 60 * 60, TierT3, "none-sol-usdc"},
		   {KYCAdvanced, "USDC", "ETH", SourceSelf, 600, 40 * 60, TierT0, "self-usdc"},
		  {KYCAdvanced, "USDC", "BASE", SourceVASP, 1000, 50 * 60, TierT0, "base-usdc-vasp"},
		  {KYCBasic, "ETH", "BASE", SourceContract, 4500, 6 * 60 * 60, TierT1, "base-contract"},
		  {KYCBasic, "ETH", "ARB", SourceContract, 4500, 6 * 60 * 60, TierT1, "arb-contract"},
		  {KYCNone, "ETH", "*", SourceContract, 9500, 24 * 60 * 60, TierT3, "none-contract-eth"},
		  {KYCAdvanced, "SOL", "SOL", SourceSelf, 700, 40 * 60, TierT0, "self-sol"},
		   {KYCBasic, "SOL", "SOL", SourceUnknown, 6500, 8 * 60 * 60, TierT2, "basic-sol-unknown"},
		  {KYCNone, "TON", "TON", SourceUnknown, 9000, 20 * 60 * 60, TierT3, "none-ton"},
		   {KYCBasic, "XRP", "XRP", SourceVASP, 3000, 4 * 60 * 60, TierT1, "xrp-vasp"},
		   {KYCBasic, "XLM", "XLM", SourceVASP, 3000, 4 * 60 * 60, TierT1, "xlm-vasp"},
		  {KYCNone, "DOGE", "BTC", SourceExchange, 8000, 10 * 60 * 60, TierT2, "doge-btc-cex"},
		  {KYCAdvanced, "DAI", "ETH", SourceVASP, 1500, 90 * 60, TierT0, "dai-vasp"},
		  {KYCBasic, "SHIB", "ETH", SourceUnknown, 9000, 12 * 60 * 60, TierT2, "shib-unknown"},
		   {KYCBasic, "PEPE", "ETH", SourceUnknown, 9000, 12 * 60 * 60, TierT2, "pepe-unknown"},
		   {KYCNone, "SHIB", "*", SourceExchange, 9500, 24 * 60 * 60, TierT3, "meme-none"},
		  {KYCAdvanced, "BTC", "*", SourceMixer, 10000, 72 * 60 * 60, TierT4, "mixer-floor-btc"},
		   {KYCAdvanced, "ETH", "*", SourceMixer, 10000, 72 * 60 * 60, TierT4, "mixer-floor-eth"},
		   {KYCBasic, "*", "*", SourceMixer, 10000, 72 * 60 * 60, TierT4, "mixer-floor-basic"},
		   {KYCNone, "*", "*", SourceMixer, 10000, 72 * 60 * 60, TierT4, "mixer-floor-none"},
		   {KYCAdvanced, "USDT", "ETH", SourceExchange, 1800, 2 * 60 * 60, TierT0, "eth-usdt-cex"},
		   {KYCAdvanced, "USDT", "BSC", SourceExchange, 3200, 3 * 60 * 60, TierT1, "bsc-usdt-cex"},
		  {KYCBasic, "BTC", "BTC", SourceSelf, 1500, 90 * 60, TierT0, "basic-self-btc"},
		   {KYCBasic, "BTC", "BTC", SourceVASP, 1800, 2 * 60 * 60, TierT0, "basic-vasp-btc"},
		   {KYCNone, "BTC", "BTC", SourceSelf, 7000, 12 * 60 * 60, TierT2, "none-self-btc"},
		  {KYCNone, "BTC", "ETH", SourceExchange, 7500, 14 * 60 * 60, TierT2, "none-wbtc"},
		  {KYCAdvanced, "ATOM", "ATOM", SourceVASP, 1600, 2 * 60 * 60, TierT0, "atom-vasp"},
		   {KYCBasic, "ATOM", "ATOM", SourceUnknown, 7000, 10 * 60 * 60, TierT2, "atom-unknown"},
		  {KYCAdvanced, "BNB", "BSC", SourceExchange, 2000, 2 * 60 * 60, TierT0, "bnb-cex"},
		  {KYCBasic, "BNB", "BSC", SourceContract, 5500, 8 * 60 * 60, TierT1, "bnb-contract"},
		  {KYCNone, "TRX", "TRON", SourceUnknown, 9200, 20 * 60 * 60, TierT3, "trx-unknown"},
		   {KYCAdvanced, "TRX", "TRON", SourceVASP, 2200, 3 * 60 * 60, TierT1, "trx-vasp"},
		   {KYCBasic, "AVAX", "AVAX", SourceExchange, 2800, 4 * 60 * 60, TierT1, "avax-cex"},
		  {KYCNone, "AVAX", "AVAX", SourceContract, 9000, 18 * 60 * 60, TierT3, "avax-contract"},
		   {KYCAdvanced, "MATIC", "MATIC", SourceVASP, 1800, 2 * 60 * 60, TierT0, "matic-vasp"},
		  {KYCBasic, "MATIC", "ETH", SourceContract, 5000, 6 * 60 * 60, TierT1, "matic-eth-contract"},
		  {KYCNone, "*", "EOS", SourceUnknown, 9000, 18 * 60 * 60, TierT3, "eos-unknown"},
		   {KYCBasic, "*", "EOS", SourceVASP, 4000, 6 * 60 * 60, TierT1, "eos-vasp"},
		   {KYCAdvanced, "USDT", "*", SourceSelf, 1000, 60 * 60, TierT0, "self-usdt-any"},
		   {KYCNone, "USDT", "*", SourceSelf, 8000, 16 * 60 * 60, TierT2, "none-self-usdt"},
		  {KYCAdvanced, "USDC", "ARB", SourceVASP, 1100, 70 * 60, TierT0, "arb-usdc"},
		   {KYCAdvanced, "USDC", "OP", SourceVASP, 1100, 70 * 60, TierT0, "op-usdc"},
		  {KYCBasic, "USDC", "OP", SourceUnknown, 7200, 10 * 60 * 60, TierT2, "op-usdc-unknown"},
		   {KYCNone, "ETH", "OP", SourceUnknown, 8800, 16 * 60 * 60, TierT3, "op-eth-unknown"},
		  {KYCAdvanced, "ETH", "OP", SourceSelf, 900, 50 * 60, TierT0, "op-self-eth"},
	   }
	  out := make([]Policy, 0, len(raw))
	  for _, r := range raw {
		   win := time.Duration(r.maxSec) * time.Second
		  win = clampWindow(win)
		  p := Policy{
			   KYC: r.kyc, Asset: r.asset, Source: r.src, Chain: r.chain,
			   RatioBPS: clampBPS(r.bps), MaxWindow: win, MinWindow: 0,
			  MaxSec: int(win / time.Second), TierFloor: r.floor,
			   Version: 1, Priority: 80, Enabled: true, Note: r.note,
		   }
		  if r.src == SourceMixer {
			   p.RatioBPS = MaxRatioBPS
			  p.TierFloor = TierT4
		  }
		  p.ID = "ovr-" + shortKey(p.key())
		   out = append(out, p)
	   }
	  return out
   }
