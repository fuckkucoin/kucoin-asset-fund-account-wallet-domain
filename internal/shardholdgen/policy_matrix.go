package shardholdgen

import "account-assets.kucoin.net/funding/internal/wdshard"

// Decision 闸位决策面，供运维与对表。

type Decision string

const (
	DecisionAllow Decision = "allow"
	DecisionDeny  Decision = "deny"
	DecisionConflict Decision = "conflict"
	DecisionProof    Decision = "proof"
)

// GateCase 策略矩阵行。

type GateCase struct {
	Name string
	Leg  wdshard.LegStatus
	Gen    GenState
	HasGen bool
	Amount int64
	HoldAmt int64
	Op      string
	Want      Decision
	Err       error
	ProofKind ProofKind
}

// PolicyMatrix Advance/Release 失败语义表。
   func PolicyMatrix() []GateCase {
	   return []GateCase{
		  {Name: "advance_pending_issued", Leg: wdshard.HoldLegPending, Gen: GenIssued, HasGen: true, Op: "advance", Want: DecisionAllow},
		   {Name: "advance_failed_held", Leg: wdshard.HoldLegFailed, Gen: GenHeld, HasGen: true, Op: "advance", Want: DecisionAllow},
		  {Name: "advance_broadcasting", Leg: wdshard.HoldLegBroadcasting, Gen: GenHeld, HasGen: true, Op: "advance", Want: DecisionDeny, Err: ErrBroadcastLive, ProofKind: ProofBroadcastRel},
		   {Name: "advance_no_gen", Leg: wdshard.HoldLegPending, HasGen: false, Op: "advance", Want: DecisionDeny, Err: ErrNoGeneration, ProofKind: ProofNoGeneration},
		  {Name: "advance_released", Leg: wdshard.HoldLegReleased, Gen: GenReleased, HasGen: true, Op: "advance", Want: DecisionDeny, Err: ErrGenReleased},
		   {Name: "advance_filled", Leg: wdshard.HoldLegFilled, Gen: GenHeld, HasGen: true, Op: "advance", Want: DecisionDeny, Err: ErrAdvanceDenied},
		  {Name: "release_failed_held", Leg: wdshard.HoldLegFailed, Gen: GenHeld, HasGen: true, Amount: 100, HoldAmt: 100, Op: "release", Want: DecisionAllow},
		   {Name: "release_broadcasting", Leg: wdshard.HoldLegBroadcasting, Gen: GenHeld, HasGen: true, Amount: 100, HoldAmt: 100, Op: "release", Want: DecisionDeny, Err: ErrBroadcastLive, ProofKind: ProofBroadcastRel},
		  {Name: "release_over", Leg: wdshard.HoldLegFailed, Gen: GenHeld, HasGen: true, Amount: 200, HoldAmt: 100, Op: "release", Want: DecisionDeny, Err: ErrOverRelease, ProofKind: ProofOverRelease},
		  {Name: "release_no_gen", Leg: wdshard.HoldLegFailed, HasGen: false, Amount: 100, Op: "release", Want: DecisionDeny, Err: ErrNoGeneration, ProofKind: ProofNoGeneration},
		  {Name: "release_expired", Leg: wdshard.HoldLegFailed, Gen: GenExpired, HasGen: true, Amount: 100, HoldAmt: 100, Op: "release", Want: DecisionDeny, Err: ErrGenExpired},
		   {Name: "release_already", Leg: wdshard.HoldLegReleased, Gen: GenReleased, HasGen: true, Amount: 100, HoldAmt: 0, Op: "release", Want: DecisionDeny, Err: ErrGenReleased},
		  {Name: "mutex_conflict", Leg: wdshard.HoldLegFailed, Gen: GenHeld, HasGen: true, Op: "mutex", Want: DecisionConflict, Err: ErrConflict, ProofKind: ProofConflict},
	  }
  }

// EvaluateGate 纯函数复验矩阵。

  func EvaluateGate(c GateCase) error {
	  var g *LegHoldGen
	   if c.HasGen {
		   g = &LegHoldGen{GenID: "lhg-eval", Generation: 1, State: c.Gen, HoldAmt: c.HoldAmt, Amount: c.HoldAmt, IssuedAt: defaultIssued()}
	   }
	  leg := wdshard.Leg{Status: c.Leg, Amount: c.HoldAmt, HoldAmt: c.HoldAmt}
	   switch c.Op {
	   case "advance":
		   return MayAdvance(leg, g, defaultIssued())
	  case "release":
		  return MayRelease(leg, g, c.Amount, defaultIssued())
	  default:
		  return nil
	   }
  }
