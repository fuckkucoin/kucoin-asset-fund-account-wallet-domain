package batchresiduefence

type ScenarioKind string

const (
	ScenConcurrentDup ScenarioKind = "concurrent_dup"
	ScenSubmitted ScenarioKind = "submitted_reject"
	ScenSkipped    ScenarioKind = "skipped_reject"
	ScenHalfResume ScenarioKind = "half_resume"
	ScenNoFence    ScenarioKind = "no_fence"
	ScenExpired    ScenarioKind = "expired"
	ScenReleased ScenarioKind = "released"
	ScenGenDrift ScenarioKind = "gen_drift"
	ScenFPDrift ScenarioKind = "fp_drift"
	ScenMutex ScenarioKind = "mutex_submit"
)

type Scenario struct {
	Kind ScenarioKind
	BatchID  string
	ItemIdem string
	Phase    ItemPhase
	UserID string
	Asset  string
	Amount  string
	WantErr error
}

   func CoreScenarios() []Scenario {
	  return []Scenario{
		   {Kind: ScenConcurrentDup, BatchID: "sc-dup", ItemIdem: "it-dup", Phase: PhaseFailed, UserID: "u-s1", Asset: "USDT", Amount: "10", WantErr: ErrDupFreeze},
		   {Kind: ScenSubmitted, BatchID: "sc-sub", ItemIdem: "it-sub", Phase: PhaseSubmitted, UserID: "u-s2", Asset: "BTC", Amount: "1", WantErr: ErrItemSubmitted},
		  {Kind: ScenSkipped, BatchID: "sc-sk", ItemIdem: "it-sk", Phase: PhaseSkipped, UserID: "u-s3", Asset: "ETH", Amount: "2", WantErr: ErrItemSkipped},
		  {Kind: ScenHalfResume, BatchID: "sc-hf", ItemIdem: "it-hf", Phase: PhaseFailed, UserID: "u-s4", Asset: "ETH", Amount: "2.25"},
		  {Kind: ScenNoFence, BatchID: "sc-nf", ItemIdem: "it-nf", Phase: PhaseFailed, UserID: "u-s5", Asset: "USDT", Amount: "3", WantErr: ErrFailClosed},
		   {Kind: ScenExpired, BatchID: "sc-ex", ItemIdem: "it-ex", Phase: PhaseFailed, UserID: "u-s6", Asset: "USDT", Amount: "4", WantErr: ErrFenceExpired},
		  {Kind: ScenReleased, BatchID: "sc-rl", ItemIdem: "it-rl", Phase: PhaseFailed, UserID: "u-s7", Asset: "USDT", Amount: "5", WantErr: ErrFenceReleased},
		  {Kind: ScenGenDrift, BatchID: "sc-gn", ItemIdem: "it-gn", Phase: PhaseFailed, UserID: "u-s8", Asset: "USDT", Amount: "6", WantErr: ErrGeneration},
		   {Kind: ScenFPDrift, BatchID: "sc-fp", ItemIdem: "it-fp", Phase: PhaseFailed, UserID: "u-s9", Asset: "USDT", Amount: "7", WantErr: ErrFingerprint},
		  {Kind: ScenMutex, BatchID: "sc-mx", ItemIdem: "it-mx", Phase: PhaseFailed, UserID: "u-s10", Asset: "USDT", Amount: "8", WantErr: ErrMutexHeld},
	   }
  }

   func ExpandAssetGrid() []Scenario {
	   assets := []string{"USDT", "BTC", "ETH", "KCS", "USDC", "SOL", "TRX", "XRP", "ADA", "DOT"}
	  amts := []string{"1", "2", "3.5", "10", "0.25", "100", "8", "9", "11", "12"}
	   out := make([]Scenario, 0, len(assets)*len(amts))
	  for i, a := range assets {
		   for j, m := range amts {
			   out = append(out, Scenario{
				   Kind: ScenNoFence, BatchID: "grid-" + a, ItemIdem: "g-" + a + "-" + m,
				  Phase: PhaseFailed, UserID: "u-g", Asset: a, Amount: m,
				   WantErr: ErrFailClosed,
			  })
			   _ = i
			  _ = j
		  }
	   }
	  return out
   }

   func ExpandPhaseRejects() []Scenario {
	  out := make([]Scenario, 0)
	  for _, ph := range []ItemPhase{PhaseSubmitted, PhaseSkipped, PhasePending} {
		   for i := 0; i < 20; i++ {
			  want := ErrItemNotFailed
			   if ph == PhaseSubmitted {
				   want = ErrItemSubmitted
			  }
			   if ph == PhaseSkipped {
				  want = ErrItemSkipped
			  }
			   out = append(out, Scenario{
				  Kind: ScenSubmitted, BatchID: "ph-" + string(ph), ItemIdem: string(ph) + "-n",
				  Phase: ph, UserID: "u-p", Asset: "USDT", Amount: "1", WantErr: want,
			  })
		   }
	   }
	  return out
   }
