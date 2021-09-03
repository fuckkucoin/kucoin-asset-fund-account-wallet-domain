package memoeffectgate

import (
	"context"
	"time"
)

type Store interface {
	SaveEffect(ctx context.Context,
		e MemoEffect) (MemoEffect, error)
	GetEffect(ctx context.Context, id string) (MemoEffect, error)
	GetEffectByTicket(ctx context.Context, ticketID string) (MemoEffect, error)
	GetEffectByIdem(ctx context.Context, idem string) (MemoEffect, error)
	GetEffectByOutput(ctx context.Context, outputKey string) (MemoEffect, error)
	ListEffectsByState(ctx context.Context, state EffectState, limit int) ([]MemoEffect, error)
	ListExpiredBound(ctx context.Context, before time.Time, limit int) ([]MemoEffect, error)
	ListEffects(ctx context.Context, limit int) ([]MemoEffect, error)
	CountEffects(ctx context.Context,
		state EffectState) (int, error)
	SaveFence(ctx context.Context, f TicketCreditFence) (TicketCreditFence, error)
	GetFence(ctx context.Context, id string) (TicketCreditFence, error)
	GetFenceByTicket(ctx context.Context, ticketID string) (TicketCreditFence, error)
	ListHalfCutFences(ctx context.Context,
		limit int) ([]TicketCreditFence, error)
	ListFences(ctx context.Context, limit int) ([]TicketCreditFence, error)
	SaveProof(ctx context.Context,
		p MemoHalfCreditProof) (MemoHalfCreditProof, error)
	GetProof(ctx context.Context, id string) (MemoHalfCreditProof, error)
	ListProofsByTicket(ctx context.Context, ticketID string, limit int) ([]MemoHalfCreditProof, error)
	ListProofsByState(ctx context.Context, state HalfState, limit int) ([]MemoHalfCreditProof, error)
	ListProofs(ctx context.Context, kind ProofKind, limit int) ([]MemoHalfCreditProof, error)
	CountProofs(ctx context.Context) (int, error)
	SaveAudit(ctx context.Context, a EffectAudit) (EffectAudit, error)
	ListAudit(ctx context.Context, effectID string, limit int) ([]EffectAudit, error)
	CountAudits(ctx context.Context) (int, error)
}
