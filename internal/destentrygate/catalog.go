package destentrygate

// KnownProofKinds 全部漂移证明类型。
   func KnownProofKinds() []ProofKind {
	  return []ProofKind{
		  DriftNoLease,
		   DriftExpired,
		  DriftRevoked,
		  DriftConsumed,
		   DriftTempExpire,
		  DriftBreakglassReclaim,
		   DriftRevoke,
		   DriftEntryGen,
		  DriftMode,
		  DriftCooldown,
		  DriftAddr,
		  DriftMissing,
		   DriftPatrol,
		  DriftWhitelistOnly,
	  }
   }

// KnownAuditActions 全部审计动作。
   func KnownAuditActions() []AuditAction {
	   return []AuditAction{
		  AuditIssue,
		  AuditBind,
		  AuditConsume,
		  AuditExpire,
		  AuditRevokeTemp,
		  AuditRevokeGlass,
		   AuditRevokeEntry,
		   AuditRevokeCooldown,
		   AuditRevokeOps,
		   AuditProof,
		   AuditGenBump,
		  AuditPatrol,
		   AuditRenewReject,
		  AuditExport,
		   AuditPrecheckDeny,
		   AuditRestoreNotFull,
	  }
   }

// KnownLifecycleKinds Revoke 泵可识别的事件 Kind。

   func KnownLifecycleKinds() []string {
	   return []string{"temp_expire", "breakglass_reclaim", "revoke", "cooldown_not_full"}
  }

// IsTerminalLease 终态判定。

   func IsTerminalLease(s LeaseState) bool {
	   switch s {
	  case LeaseConsumed, LeaseExpired, LeaseRevoked:
		   return true
	   default:
		   return false
	   }
   }

// ProofKindValid 证明类型是否在目录内。
  func ProofKindValid(k ProofKind) bool {
	  for _, x := range KnownProofKinds() {
		   if x == k {
			  return true
		  }
	  }
	   return false
  }
