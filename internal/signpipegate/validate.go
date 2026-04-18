package signpipegate

import (
	"strings"
	"unicode"
	"account-assets.kucoin.net/funding/pkg/money"
)

const (
	maxIDLen = 64
	maxNonceLen = 128
	maxNoteLen = 255
	maxActorLen = 64
)

  func validateMovement(id string) error {
	   id = NormalizeMovement(id)
	  if id == "" || len(id) > maxIDLen {
		   return ErrMovementInvalid
	   }
	  for _, r := range id {
		  if r > unicode.MaxASCII || (!unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' && r != ':') {
			  return ErrMovementInvalid
		  }
	  }
	  return nil
   }

   func validateJob(id string) error {
	   id = strings.TrimSpace(id)
	   if id == "" || len(id) > maxIDLen+8 {
		   return ErrJobInvalid
	  }
	  return nil
  }

   func validateChain(chain string) error {
	   c := NormalizeChain(chain)
	  if c == "" || len(c) > 16 {
		   return ErrChainInvalid
	  }
	  return nil
  }

  func validateDest(to string) error {
	  to = NormalizeDest(to)
	   if to == "" || len(to) > 128 {
		  return ErrDestInvalid
	  }
	  return nil
   }

  func validateAmount(amount string) error {
	  if _, err := money.Parse(strings.TrimSpace(amount)); err != nil {
		  return ErrAmountInvalid
	  }
	  return nil
   }

  func validateNonce(nonce string) error {
	   nonce = NormalizeNonce(nonce)
	   if nonce == "" || len(nonce) > maxNonceLen {
		  return ErrNonceInvalid
	   }
	   return nil
  }

   func validateActor(actor string) error {
	   actor = NormalizeActor(actor)
	   if actor == "" || len(actor) > maxActorLen {
		   return ErrActorInvalid
	  }
	  if strings.EqualFold(actor, "system") {
		  return ErrActorInvalid
	  }
	   return nil
  }

   func validateOperator(op string) error {
	   op = strings.TrimSpace(op)
	   if op == "" || len(op) > maxActorLen {
		   return ErrOperatorInvalid
	   }
	   return nil
   }

  func validateReason(reason string) error {
	   reason = strings.TrimSpace(reason)
	  if reason == "" || len(reason) > maxNoteLen {
		   return ErrReasonRequired
	   }
	   return nil
  }

   func validateArm(req ArmReq) error {
	   if err := validateMovement(req.MovementID); err != nil {
		  return err
	  }
	   if err := validateJob(req.JobID); err != nil {
		   return err
	  }
	  if err := validateChain(req.Chain); err != nil {
		  return err
	  }
	   if err := validateDest(req.To); err != nil {
		  return err
	   }
	  if err := validateAmount(req.Amount); err != nil {
		   return err
	   }
	   if err := validateNonce(req.Nonce); err != nil {
		  return err
	   }
	   if err := validateActor(req.Actor); err != nil {
		   return err
	   }
	   return nil
  }

  func distinctActors(req, appr, sign string) error {
	  req, appr, sign = NormalizeActor(req), NormalizeActor(appr), NormalizeActor(sign)
	   if req == "" || appr == "" || sign == "" {
		  return nil
	  }
	  if strings.EqualFold(req, appr) || strings.EqualFold(req, sign) || strings.EqualFold(appr, sign) {
		  return ErrDualActor
	   }
	   return nil
  }

   func Canonical(movementID, chain, to, amount string) string {
	   return strings.Join([]string{
		   NormalizeMovement(movementID),
		  NormalizeChain(chain),
		  strings.ToLower(NormalizeDest(to)),
		  strings.TrimSpace(amount),
	  }, "|")
  }

   func isolationOK(mark string) error {
	  low := strings.ToLower(mark)
	  if strings.Contains(low, "por_root") || strings.Contains(low, "stmt_manifest") || strings.Contains(low, "balproof_root") {
		  return ErrIsolationViolation
	   }
	   return nil
   }
