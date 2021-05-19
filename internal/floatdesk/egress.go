package floatdesk

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

// BreachLevel 水位击穿等级。
// ok 可用 ≥ 目标
// warn 下限 ≤ 可用 < 目标
// halt 可用 < 下限，或 Flags 已停仓

type TankLevel string

const (
	TankOK TankLevel = "ok"
	TankWarn TankLevel = "warn"
	TankHalt TankLevel = "halt"
)

// EgressDecision 出金瞬时门控结论。

type InstantDecision string

const (
	InstantAllow InstantDecision = "allow"
	InstantWarn InstantDecision = "warn"
	InstantHalt InstantDecision = "halt"
)

// EgressGateEvent 出金瞬时门控事件。供 egressleasegate 只读固化。

type InstantEgressEvent struct {
	Chain     string          `json:"chain"`
	Asset     string          `json:"asset"`
	Decision  InstantDecision `json:"decision"`
	TankLevel TankLevel       `json:"breach_level"`
	FlagsFingerprint string `json:"flags_fingerprint"`
	TankDigest       string `json:"tank_digest"`
	HotAvail         string `json:"hot_available"`
	Min              string `json:"min"`
	Target           string `json:"target"`
	HaltGeneration uint64 `json:"halt_generation"`
	Reason string `json:"reason"`
}

// GradeAvail 按可用/下限/目标计算水位等级。金额一律走 money.ParseBook。

  func GradeAvail(avail, min, target string) (TankLevel, error) {
	   av,
		   err := money.ParseBook(orZ(avail))
	   if err != nil {
		   return "", err
	   }
	  lo, err := money.Parse(min)
	  if err != nil {
		   return "", ErrTarget
	   }
	  tgt, err := money.Parse(target)
	   if err != nil {
		   return "", ErrTarget
	   }
	   if av.Cmp(lo) < 0 {
		   return TankHalt, nil
	   }
	  if av.Cmp(tgt) < 0 {
		  return TankWarn, nil
	   }
	   return TankOK, nil
   }

// TankDigest 单仓水位摘要。不含补仓代次、不含热库 cap。

   func TankDigest(t Tank) string {
	  raw := strings.ToUpper(strings.TrimSpace(t.Chain)) + "|" +
		   strings.ToUpper(strings.TrimSpace(t.Native)) + "|" +
		  orZ(t.Avail) + "|" + orZ(t.Min) + "|" + orZ(t.Target)
	  sum := sha256.Sum256([]byte(raw))
	  return fmt.Sprintf("%x", sum[:])
   }

// TanksDigest 多仓稳定摘要。按 chain 排序后拼接。
  func TanksDigest(rows []Tank) string {
	  cp := append([]Tank(nil), rows...)
	   sort.Slice(cp, func(i, j int) bool {
		  if cp[i].Chain == cp[j].Chain {
			  return cp[i].Native < cp[j].Native
		  }
		   return cp[i].Chain < cp[j].Chain
	  })
	   parts := make([]string,
		  0, len(cp))
	  for _, t := range cp {
		  parts = append(parts, TankDigest(t))
	   }
	  sum := sha256.Sum256([]byte(strings.Join(parts, ",")))
	  return fmt.Sprintf("%x", sum[:])
  }

   func flagsFingerprint(snap map[string]bool) string {
	  if snap == nil {
		  sum := sha256.Sum256([]byte(""))
		   return fmt.Sprintf("%x", sum[:])
	  }
	   keys := make([]string, 0, len(snap))
	  for k := range snap {
		   keys = append(keys, k)
	  }
	   sort.Strings(keys)
	  var b strings.Builder
	  for _, k := range keys {
		   if snap[k] {
			  b.WriteString(k)
			   b.WriteByte('=')
			   b.WriteString("1")
			   b.WriteByte(';')
		   }
	   }
	   sum := sha256.Sum256([]byte(b.String()))
	  return fmt.Sprintf("%x", sum[:])
  }

// CheckEgress 按目录目标与热账可用瞬时门控。
// 前置：chain/asset 须过校验；hotAvail 走账面解析。
// 失败：Flags 停仓或水位 halt → Decision=Halt；不签发租约。
  func (s *Service) CheckEgress(chain, asset, hotAvail string) (InstantEgressEvent, error) {
	  if s == nil {
		   return InstantEgressEvent{}, ErrUnavailable
	   }
	   ch, err := validate.Chain(chain)
	   if err != nil {
		  return InstantEgressEvent{}, ErrChain
	   }
	  as,
		  err := validate.Asset(asset)
	  if err != nil {
		  return InstantEgressEvent{}, ErrAsset
	   }
	  tgt, err := s.cat.Get(ch, as)
	   if err != nil {
		  return InstantEgressEvent{}, err
	  }
	   ev := InstantEgressEvent{
		  Chain: ch,
		   Asset: as,
		  HotAvail: orZ(hotAvail),
		  Min:      tgt.Min,
		  Target: tgt.Target,
		  FlagsFingerprint: s.halt.Fingerprint(),
		  HaltGeneration:   s.halt.Generation(),
		   TankDigest:       TankDigest(Tank{Chain: ch, Native: as, Avail: orZ(hotAvail), Min: tgt.Min, Target: tgt.Target}),
	  }
	  if err := s.halt.Check(ch, as); err != nil {
		   ev.Decision = InstantHalt
		   ev.TankLevel = TankHalt
		   ev.Reason = "halt_flag"
		   return ev, nil
	   }
	  lv, err := GradeAvail(hotAvail, tgt.Min, tgt.Target)
	  if err != nil {
		   return InstantEgressEvent{}, err
	  }
	   ev.TankLevel = lv
	   switch lv {
	  case TankHalt:
		   ev.Decision = InstantHalt
		  ev.Reason = "level_halt"
	  case TankWarn:
		  ev.Decision = InstantWarn
		   ev.Reason = "level_warn"
	   default:
		  ev.Decision = InstantAllow
		   ev.Reason = "ok"
	   }
	  return ev, nil
  }

// CheckEgressFromTanks 按 gas 仓水位瞬时门控。资产取仓位 Native。
  func (s *Service) CheckEgressFromTanks(chain, asset string) (InstantEgressEvent, error) {
	   if s == nil {
		  return InstantEgressEvent{}, ErrUnavailable
	   }
	   ch, err := validate.Chain(chain)
	  if err != nil {
		   return InstantEgressEvent{}, ErrChain
	   }
	   as := strings.ToUpper(strings.TrimSpace(asset))
	  if as != "" {
		  if _,
			  err := validate.Asset(as); err != nil {
			   return InstantEgressEvent{}, ErrAsset
		  }
	  }
	   tank, err := s.tanks.Get(ch)
	   if err != nil {
		   return InstantEgressEvent{}, err
	   }
	  if as == "" {
		  as = tank.Native
	  }
	   ev := InstantEgressEvent{
		   Chain:    ch,
		   Asset:    as,
		  HotAvail: orZ(tank.Avail),
		   Min:      tank.Min,
		   Target:           tank.Target,
		  FlagsFingerprint: s.halt.Fingerprint(),
		  HaltGeneration:   s.halt.Generation(),
		   TankDigest:       TankDigest(tank),
	   }
	   if err := s.halt.Check(ch, as); err != nil {
		   ev.Decision = InstantHalt
		  ev.TankLevel = TankHalt
		   ev.Reason = "halt_flag"
		  return ev, nil
	   }
	  lv, err := GradeAvail(tank.Avail, tank.Min, tank.Target)
	   if err != nil {
		   return InstantEgressEvent{}, err
	   }
	   ev.TankLevel = lv
	  switch lv {
	   case TankHalt:
		   ev.Decision = InstantHalt
		  ev.Reason = "tank_halt"
	   case TankWarn:
		   ev.Decision = InstantWarn
		   ev.Reason = "tank_warn"
	  default:
		   ev.Decision = InstantAllow
		   ev.Reason = "ok"
	  }
	   return ev, nil
  }
