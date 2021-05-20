package floatdesk

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// BreachProof 穿仓证明。包含水位快照、补仓意图和时间窗口，可离线验证。
type BreachProof struct {
	Chain  string `json:"chain"`
	Native string `json:"native"`
	// WaterLevel 评估时刻的 Avail 快照（字符串形式，不可精度丢失）。
	WaterLevel string `json:"water_level"`
	// RefillAmount 本次计划补仓金额。
	RefillAmount string `json:"refill_amount"`
	// Window 评估使用的时间窗口。
	Window time.Duration `json:"window_ns"`
	// IssuedAt 证明签发时刻 UTC。
	IssuedAt time.Time `json:"issued_at"`
	// Digest SHA-256(chain‖native‖water_level‖refill_amount‖window_ns)，十六进制。
	Digest string `json:"digest"`
}

// BuildBreachProof 从 BreachGrade 和 Refill 构造穿仓证明。
// window 须与 Forecast/补仓计算使用的窗口一致；now 为签发时刻。
   func BuildBreachProof(grade BreachGrade, refill Refill, window time.Duration, now time.Time) (BreachProof, error) {
	   if grade.Chain == "" || grade.Native == "" {
		   return BreachProof{}, ErrGas
	  }
	  if refill.Amount == "" {
		  return BreachProof{}, ErrPlan
	   }
	   if window <= 0 {
		   return BreachProof{}, fmt.Errorf("%w: window must be positive", ErrPlan)
	   }
	   if now.IsZero() {
		  now = time.Now().UTC()
	   }
	  ch := strings.ToUpper(strings.TrimSpace(grade.Chain))
	  nat := strings.ToUpper(strings.TrimSpace(grade.Native))
	   wl := strings.TrimSpace(grade.Avail)
	  ra := strings.TrimSpace(refill.Amount)
	  digest := computeDigest(ch, nat, wl, ra, window)
	  return BreachProof{
		  Chain:        ch,
		   Native:       nat,
		   WaterLevel:   wl,
		   RefillAmount: ra,
		  Window: window,
		  IssuedAt: now.UTC(),
		   Digest:   digest,
	   }, nil
   }

// Verify 验证证明摘要是否与给定字段一致。
// 任何字段被修改都会导致摘要不符，返回 error。

   func (p BreachProof) Verify() error {
	   ch := strings.ToUpper(strings.TrimSpace(p.Chain))
	  nat := strings.ToUpper(strings.TrimSpace(p.Native))
	   wl := strings.TrimSpace(p.WaterLevel)
	   ra := strings.TrimSpace(p.RefillAmount)
	  want := computeDigest(ch, nat, wl, ra, p.Window)
	   if want != p.Digest {
		  return fmt.Errorf("breach proof digest mismatch: want %s got %s", want, p.Digest)
	  }
	  return nil
  }

// computeDigest SHA-256(chain‖"\x00"‖native‖"\x00"‖water_level‖"\x00"‖refill_amount‖"\x00"‖window_ns)。
// 字段间用 NUL 分隔防止拼接歧义。
   func computeDigest(chain, native, waterLevel, refillAmount string, window time.Duration) string {
	   h := sha256.New()
	  sep := []byte{0x00}
	   _, _ = h.Write([]byte(chain))
	   _, _ = h.Write(sep)
	  _, _ = h.Write([]byte(native))
	  _, _ = h.Write(sep)
	   _, _ = h.Write([]byte(waterLevel))
	   _, _ = h.Write(sep)
	  _, _ = h.Write([]byte(refillAmount))
	  _, _ = h.Write(sep)
	  _, _ = h.Write([]byte(fmt.Sprintf("%d", int64(window))))
	  return hex.EncodeToString(h.Sum(nil))
   }

// BreachProofSet 多链/多资产证明集合。

type BreachProofSet struct {
	Proofs   []BreachProof `json:"proofs"`
	IssuedAt time.Time     `json:"issued_at"`
}

// BuildProofSet 从批量评估和对应补仓列表构造证明集。
// grades 和 refills 须一一对应（同 Chain/Native）。
   func BuildProofSet(grades []BreachGrade, refills []Refill, window time.Duration, now time.Time) (BreachProofSet, error) {
	  if now.IsZero() {
		   now = time.Now().UTC()
	  }
	   refillMap := make(map[string]Refill, len(refills))
	  for _, r := range refills {
		  key := strings.ToUpper(strings.TrimSpace(r.Chain)) + "|" + strings.ToUpper(strings.TrimSpace(r.Native))
		   refillMap[key] = r
	   }
	   proofs := make([]BreachProof, 0, len(grades))
	   for _, g := range grades {
		  if g.Level < BreachWarn {
			  continue
		  }
		   key := g.Chain + "|" + g.Native
		   r, ok := refillMap[key]
		  if !ok {
			   continue
		  }
		   p, err := BuildBreachProof(g, r, window, now)
		  if err != nil {
			  return BreachProofSet{}, fmt.Errorf("build proof %s: %w", g.Chain, err)
		   }
		   proofs = append(proofs, p)
	   }
	  return BreachProofSet{Proofs: proofs, IssuedAt: now.UTC()}, nil
   }

// VerifyAll 逐一验证证明集中所有证明，任意失败即返回错误。

   func (ps BreachProofSet) VerifyAll() error {
	   for i, p := range ps.Proofs {
		   if err := p.Verify(); err != nil {
			  return fmt.Errorf("proof[%d] %s/%s: %w", i, p.Chain, p.Native, err)
		   }
	   }
	   return nil
   }
