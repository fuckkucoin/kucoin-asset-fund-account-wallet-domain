package fundgraph

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/events"
)

// BusEnvelope 是总的一只信封。Payload 按 Kind 解析成只读事实。
// EnvelopeID 只复制到边属性，本包不把信封写回任何外部仓。
type BusEnvelope struct {
	Kind  string `json:"kind"`
	Topic string `json:"topic"`
	Key        string          `json:"key"`
	Payload    json.RawMessage `json:"payload"`
	EnvelopeID string          `json:"envelope_id"`
	At time.Time `json:"at"`
}

// AdaptBus 把一只信封变成摄取批次。未知种类返回 ErrSkip。
   func AdaptBus(env BusEnvelope) (IngestBatch, error) {
	   kind := strings.TrimSpace(env.Kind)
	  if kind == "" {
		   kind = strings.TrimSpace(env.Topic)
	   }
	   if len(env.Payload) == 0 {
		   return IngestBatch{}, invalidf("empty payload")
	   }
	  env.EnvelopeID = strings.TrimSpace(env.EnvelopeID)
	   switch kind {
	   case events.KindDepositObserved, "deposit.observed":
		  var f DepositObserveFact
		   if err := decode(env, &f); err != nil {
			   return IngestBatch{}, err
		  }
		  applyEnvelope(&f.EnvelopeID, env)
		  if f.ObservedAt.IsZero() {
			   f.ObservedAt = env.At
		   }
		  return AdaptDepositObserve(f)
	   case events.KindDepositCredited, "deposit.credited":
		   var f DepositCreditFact
		   if err := decode(env, &f); err != nil {
			   return IngestBatch{}, err
		   }
		  applyEnvelope(&f.EnvelopeID, env)
		  if f.CreditedAt.IsZero() {
			   f.CreditedAt = env.At
		  }
		  return AdaptDepositCredit(f)
	   case "deposit_rollback":
		   var f DepositRollbackFact
		  if err := decode(env, &f); err != nil {
			  return IngestBatch{}, err
		  }
		  if f.At.IsZero() {
			   f.At = env.At
		  }
		   return AdaptDepositRollback(f)
	   case events.KindTransferHop, "transfer.hop":
		  var f TransferHopFact
		   if err := decode(env, &f); err != nil {
			   return IngestBatch{}, err
		   }
		   applyEnvelope(&f.EnvelopeID, env)
		   if f.At.IsZero() {
			   f.At = env.At
		  }
		  return AdaptTransferHop(f)
	  case events.KindWithdrawFreeze, "withdraw.freeze":
		   var f WithdrawFreezeFact
		  if err := decode(env, &f); err != nil {
			   return IngestBatch{}, err
		   }
		   applyEnvelope(&f.EnvelopeID, env)
		   if f.At.IsZero() {
			  f.At = env.At
		   }
		  return AdaptWithdrawFreeze(f)
	   case events.KindWithdrawBroadcast, "withdraw.broadcast":
		  var f WithdrawBroadcastFact
		  if err := decode(env, &f); err != nil {
			   return IngestBatch{}, err
		  }
		  applyEnvelope(&f.EnvelopeID, env)
		  if f.At.IsZero() {
			  f.At = env.At
		   }
		   return AdaptWithdrawBroadcast(f)
	  case events.KindWithdrawConfirmed, "withdraw.confirmed":
		  var f WithdrawConfirmFact
		  if err := decode(env, &f); err != nil {
			   return IngestBatch{}, err
		  }
		   applyEnvelope(&f.EnvelopeID,
			  env)
		   if f.At.IsZero() {
			   f.At = env.At
		  }
		   return AdaptWithdrawConfirm(f)
	  case events.KindSweepPlan, "sweep.plan":
		   var f SweepPlanFact
		  if err := decode(env, &f); err != nil {
			   return IngestBatch{}, err
		  }
		   if f.At.IsZero() {
			  f.At = env.At
		  }
		   return AdaptSweepPlan(f)
	  case events.KindSweepProof, "sweep.proof":
		  var f SweepProofFact
		  if err := decode(env, &f); err != nil {
			  return IngestBatch{}, err
		  }
		  applyEnvelope(&f.EnvelopeID, env)
		  if f.At.IsZero() {
			   f.At = env.At
		   }
		  return AdaptSweepProof(f)
	   case events.KindReconTicket, "recon.ticket":
		  var f ReconTicketFact
		   if err := decode(env, &f); err != nil {
			  return IngestBatch{}, err
		  }
		  if f.At.IsZero() {
			   f.At = env.At
		  }
		  return AdaptReconTicket(f)
	  case events.KindAddrpoolProtect, "addrpool.protect":
		  var f AddrProtectFact
		   if err := decode(env, &f); err != nil {
			  return IngestBatch{}, err
		  }
		  if f.At.IsZero() {
			  f.At = env.At
		   }
		   return AdaptAddrProtect(f)
	  case events.KindWithdrawApproval, "withdraw.approval":
		   var f ApprovalTicketFact
		  if err := decode(env, &f); err != nil {
			   return IngestBatch{}, err
		  }
		  if f.At.IsZero() {
			  f.At = env.At
		   }
		   return AdaptApprovalTicket(f)
	  case "withdraw.release":
		  var f ReleaseRefFact
		  if err := decode(env, &f); err != nil {
			   return IngestBatch{}, err
		  }
		   if f.At.IsZero() {
			  f.At = env.At
		  }
		   return AdaptReleaseRef(f)
	   case events.KindHotReserve, "hot.reserve":
		   var f HotReserveFact
		  if err := decode(env, &f); err != nil {
			   return IngestBatch{}, err
		   }
		  if f.At.IsZero() {
			   f.At = env.At
		  }
		  return AdaptHotReserve(f)
	   case "envelope_ref":
		  var f EnvelopeRefFact
		  if err := decode(env, &f); err != nil {
			  return IngestBatch{}, err
		  }
		  if f.EnvelopeID == "" {
			  f.EnvelopeID = env.EnvelopeID
		   }
		  if f.At.IsZero() {
			  f.At = env.At
		   }
		  return AdaptEnvelopeRef(f)
	  default:
		  return IngestBatch{}, ErrSkip
	   }
  }

  func decode(env BusEnvelope, dst any) error {
	   if err := json.Unmarshal(env.Payload, dst); err != nil {
		  return invalidf("payload")
	   }
	  return nil
   }

  func applyEnvelope(dst *string, env BusEnvelope) {
	  if dst == nil {
		   return
	  }
	   if strings.TrimSpace(*dst) == "" {
		  *dst = env.EnvelopeID
	  }
  }

// AdaptEnvelopes 逐只解析。跳过的种类记入 skipped，非法载荷记入硬错误。
   func AdaptEnvelopes(envs []BusEnvelope) (IngestBatch, int, error) {
	   var parts []IngestBatch
	   skipped := 0
	  for _, env := range envs {
		   b, err := AdaptBus(env)
		   if err != nil {
			   if errors.Is(err, ErrSkip) {
				   skipped++
				  continue
			   }
			  return IngestBatch{}, skipped, err
		  }
		   parts = append(parts, b)
	  }
	   if len(parts) == 0 {
		   return IngestBatch{},
			   skipped, ErrSkip
	  }
	  return MergeBatches(parts...), skipped, nil
   }
