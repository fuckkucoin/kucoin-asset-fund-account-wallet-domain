package haltepochgate

import "context"

// Store 世代 / 证明 / 清窗案 / 意图 / 投票 / 审计。
type Store interface {
	GetEpoch(ctx context.Context, scope ScopeKey) (*HaltEpoch, error)
	PutEpoch(ctx context.Context, e HaltEpoch) error
	ListEpochs(ctx context.Context, limit int) ([]HaltEpoch, error)
	GetIntent(ctx context.Context,
		id string) (*FundIntent, error)
	GetIntentByIdem(ctx context.Context, idem string) (*FundIntent, error)
	PutIntent(ctx context.Context,
		in FundIntent) error
	ListIntentsByScope(ctx context.Context, scope ScopeKey, states []IntentState, limit int) ([]FundIntent, error)
	ListAllowedIntents(ctx context.Context, limit int) ([]FundIntent, error)
	GetProof(ctx context.Context, id string) (*SlipProof, error)
	PutProof(ctx context.Context, p SlipProof) error
	ListProofs(ctx context.Context, state SlipState, limit int) ([]SlipProof, error)
	ListProofsByScope(ctx context.Context,
		scope ScopeKey, limit int) ([]SlipProof, error)
	GetCase(ctx context.Context, id string) (*ClearWindowCase, error)
	PutCase(ctx context.Context,
		c ClearWindowCase) error
	ListOpenCases(ctx context.Context, limit int) ([]ClearWindowCase, error)
	GetQuorum(ctx context.Context, id string) (*QuorumBundle, error)
	PutQuorum(ctx context.Context, q QuorumBundle) error
	GetQuorumByEpoch(ctx context.Context, epochID string) (*QuorumBundle, error)
	AppendAudit(ctx context.Context, e AuditEvent) error
	ListAudit(ctx context.Context, limit int) ([]AuditEvent, error)
}
