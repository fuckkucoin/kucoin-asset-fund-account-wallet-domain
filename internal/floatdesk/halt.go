package floatdesk

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"sync"
	"account-assets.kucoin.net/funding/internal/validate"
)

// Flags 紧急停浮。全站、按链、按资产任一打开即拒，不得接着补仓或出金预估。

type Flags struct {
	mu sync.Mutex
	global bool
	chain map[string]bool
	asset map[string]bool
	// gen 停仓翻转世代。仅 HaltAll/HaltChain/HaltAsset 递增；Resume 不回退。
	// 出金瞬时 Check 本身不因翻转吊销已放行结果；世代供 egress_lease 复验。
	gen uint64
}

   func NewFlags() *Flags {
	   return &Flags{chain: map[string]bool{}, asset: map[string]bool{}}
   }

  func (f *Flags) HaltAll() {
	   if f == nil {
		   return
	   }
	  f.mu.Lock()
	   if !f.global {
		  f.gen++
	  }
	   f.global = true
	  f.mu.Unlock()
  }

   func (f *Flags) ResumeAll() {
	   if f == nil {
		   return
	   }
	   f.mu.Lock()
	   f.global = false
	   f.mu.Unlock()
  }

  func (f *Flags) HaltChain(chain string) error {
	  if f == nil {
		   return ErrUnavailable
	  }
	  ch, err := validate.Chain(chain)
	  if err != nil {
		   return ErrChain
	  }
	  f.mu.Lock()
	  if !f.chain[ch] {
		   f.gen++
	   }
	   f.chain[ch] = true
	   f.mu.Unlock()
	  return nil
  }

   func (f *Flags) ResumeChain(chain string) {
	   if f == nil {
		   return
	  }
	   f.mu.Lock()
	   delete(f.chain, strings.ToUpper(strings.TrimSpace(chain)))
	  f.mu.Unlock()
  }

  func (f *Flags) HaltAsset(asset string) error {
	   if f == nil {
		   return ErrUnavailable
	   }
	   as, err := validate.Asset(asset)
	   if err != nil {
		  return ErrAsset
	   }
	  f.mu.Lock()
	   if !f.asset[as] {
		  f.gen++
	  }
	  f.asset[as] = true
	   f.mu.Unlock()
	   return nil
  }

  func (f *Flags) ResumeAsset(asset string) {
	  if f == nil {
		  return
	  }
	   f.mu.Lock()
	   delete(f.asset, strings.ToUpper(strings.TrimSpace(asset)))
	   f.mu.Unlock()
   }

  func (f *Flags) Halted() bool {
	  if f == nil {
		   return false
	  }
	  f.mu.Lock()
	   defer f.mu.Unlock()
	   return f.global
  }

// Check 全站或该链/资产停了就失败即关。未登记的链也要过 Chain 校验。
   func (f *Flags) Check(chain, asset string) error {
	   if f == nil {
		   return ErrUnavailable
	  }
	   var ch, as string
	   var err error
	  if chain != "" {
		  ch, err = validate.Chain(chain)
		   if err != nil {
			  return ErrChain
		  }
	  }
	  if asset != "" {
		  as, err = validate.Asset(asset)
		  if err != nil {
			  return ErrAsset
		  }
	  }
	   f.mu.Lock()
	   defer f.mu.Unlock()
	   if f.global {
		  return ErrHalt
	   }
	   if ch != "" && f.chain[ch] {
		  return ErrHalt
	   }
	  if as != "" && f.asset[as] {
		  return ErrHalt
	  }
	  return nil
   }

// HaltView 某一链与资产上的停仓位。指纹只覆盖这三位，其它链的停仓不改变本范围指纹。
type HaltView struct {
	Global    bool
	Chain     bool
	Asset     bool
	ChainName string
	AssetName string
}

// HaltedAny 全站、链或资产任一停仓。

   func (v HaltView) HaltedAny() bool {
	   return v.Global || v.Chain || v.Asset
   }

// Fingerprint 停仓位指纹。放行租约固化该值，广播前再算一次。
   func (v HaltView) Fingerprint() string {
	   raw := "g=0|c=0|a=0"
	  g, c, a := "0", "0", "0"
	  if v.Global {
		   g = "1"
	   }
	   if v.Chain {
		  c = "1"
	   }
	   if v.Asset {
		  a = "1"
	   }
	  raw = "g=" + g + "|c=" + c + "|a=" + a
	   sum := sha256.Sum256([]byte(raw))
	   return hex.EncodeToString(sum[:])
   }

// View 读取指定链与资产的停仓位。链、资产为空时只反映全站位。

   func (f *Flags) View(chain, asset string) (HaltView, error) {
	  if f == nil {
		  return HaltView{}, ErrUnavailable
	  }
	   var ch, as string
	  var err error
	   if chain != "" {
		   ch, err = validate.Chain(chain)
		   if err != nil {
			   return HaltView{}, ErrChain
		   }
	  }
	   if asset != "" {
		  as, err = validate.Asset(asset)
		  if err != nil {
			  return HaltView{}, ErrAsset
		   }
	   }
	   f.mu.Lock()
	  defer f.mu.Unlock()
	  v := HaltView{Global: f.global, ChainName: ch, AssetName: as}
	   if ch != "" {
		  v.Chain = f.chain[ch]
	   }
	   if as != "" {
		  v.Asset = f.asset[as]
	  }
	  return v, nil
  }

// HaltedChains 当前被停的链名，按字典序返回。

  func (f *Flags) HaltedChains() []string {
	  if f == nil {
		  return nil
	  }
	  f.mu.Lock()
	  defer f.mu.Unlock()
	   out := make([]string, 0, len(f.chain))
	  for k, on := range f.chain {
		  if on {
			   out = append(out, k)
		   }
	   }
	   sort.Strings(out)
	  return out
  }

// HaltedAssets 当前被停的资产代码，按字典序返回。

   func (f *Flags) HaltedAssets() []string {
	   if f == nil {
		  return nil
	   }
	  f.mu.Lock()
	   defer f.mu.Unlock()
	   out := make([]string, 0, len(f.asset))
	   for k, on := range f.asset {
		  if on {
			   out = append(out, k)
		   }
	  }
	  sort.Strings(out)
	  return out
  }

   func (f *Flags) Snapshot() map[string]bool {
	   if f == nil {
		   return nil
	  }
	  f.mu.Lock()
	  defer f.mu.Unlock()
	  out := map[string]bool{"*": f.global}
	   for k, v := range f.chain {
		  out["chain:"+k] = v
	   }
	  for k, v := range f.asset {
		   out["asset:"+k] = v
	  }
	  return out
  }

// Generation 停仓翻转世代。未发生 Halt 时为 0。
  func (f *Flags) Generation() uint64 {
	   if f == nil {
		  return 0
	  }
	  f.mu.Lock()
	   defer f.mu.Unlock()
	   return f.gen
   }

// Fingerprint 停仓 Flags 稳定指纹。键序固定，供 egress_lease 固化与复验。
   func (f *Flags) Fingerprint() string {
	  if f == nil {
		   return flagsFingerprint(nil)
	   }
	   return flagsFingerprint(f.Snapshot())
   }
