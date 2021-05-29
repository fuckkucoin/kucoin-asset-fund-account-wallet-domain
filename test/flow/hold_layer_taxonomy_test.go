package flow_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/holdcat"
)

   func TestLayerProperties(t *testing.T) {
	  cases := []struct {
		   layer     holdcat.Layer
		   wantName  string
		   wantPrio  int
		   wantCode  string
		  wantValid bool
	   }{
		   {holdcat.LayerLegal, "legal", 100, "legal", true},
		   {holdcat.LayerAML, "aml", 80, "aml", true},
		  {holdcat.LayerRisk, "risk", 60, "risk", true},
		  {holdcat.LayerOps, "ops", 40, "ops", true},
		   {holdcat.LayerUnknown, "unknown", 0, "unknown", false},
	   }
	  for _, tc := range cases {
		  tc := tc
		  t.Run(tc.wantName, func(t *testing.T) {
			  if got := tc.layer.Name(); got != tc.wantName {
				  t.Errorf("Name()=%q want %q", got, tc.wantName)
			  }
			  if got := tc.layer.Priority(); got != tc.wantPrio {
				   t.Errorf("Priority()=%d want %d", got, tc.wantPrio)
			   }
			  if got := tc.layer.Code(); got != tc.wantCode {
				  t.Errorf("Code()=%q want %q", got, tc.wantCode)
			  }
			   if got := tc.layer.Valid(); got != tc.wantValid {
				  t.Errorf("Valid()=%v want %v", got, tc.wantValid)
			   }
		   })
	   }
   }

   func TestCanRelease_Matrix(t *testing.T) {
	  // 完整 4×4 矩阵
	  type matrixCase struct {
		  caller holdcat.Layer
		  target  holdcat.Layer
		   allowed bool
	   }
	  cases := []matrixCase{
		  // legal 行（priority=100）：可释放所有层
		  {holdcat.LayerLegal, holdcat.LayerLegal, true},
		   {holdcat.LayerLegal, holdcat.LayerAML, true},
		  {holdcat.LayerLegal, holdcat.LayerRisk, true},
		  {holdcat.LayerLegal, holdcat.LayerOps, true},
		  // aml 行（priority=80）：可释放 aml/risk/ops，不可释放 legal
		   {holdcat.LayerAML, holdcat.LayerLegal, false},
		  {holdcat.LayerAML, holdcat.LayerAML, true},
		   {holdcat.LayerAML, holdcat.LayerRisk, true},
		  {holdcat.LayerAML, holdcat.LayerOps, true},
		   // risk 行（priority=60）：可释放 risk/ops，不可释放 legal/aml
		  {holdcat.LayerRisk, holdcat.LayerLegal, false},
		  {holdcat.LayerRisk, holdcat.LayerAML, false},
		  {holdcat.LayerRisk, holdcat.LayerRisk, true},
		  {holdcat.LayerRisk, holdcat.LayerOps, true},
		   // ops 行（priority=40）：只可释放 ops
		   {holdcat.LayerOps, holdcat.LayerLegal, false},
		  {holdcat.LayerOps, holdcat.LayerAML, false},
		  {holdcat.LayerOps, holdcat.LayerRisk, false},
		  {holdcat.LayerOps, holdcat.LayerOps, true},
	  }
	   for _, tc := range cases {
		  tc := tc
		  name := tc.caller.Code() + "_releases_" + tc.target.Code()
		  t.Run(name, func(t *testing.T) {
			  got := holdcat.CanRelease(tc.caller, tc.target)
			  if got != tc.allowed {
				  t.Errorf("CanRelease(%s,%s)=%v want %v", tc.caller, tc.target, got, tc.allowed)
			  }
		  })
	   }
  }

  func TestParseLayer(t *testing.T) {
	  cases := []struct {
		  input   string
		   want    holdcat.Layer
		   wantErr bool
	  }{
		   {"legal", holdcat.LayerLegal, false},
		  {"Legal", holdcat.LayerLegal, false},
		   {"LEGAL", holdcat.LayerLegal, false},
		  {"aml", holdcat.LayerAML, false},
		   {"AML", holdcat.LayerAML, false},
		  {"risk", holdcat.LayerRisk, false},
		   {"Risk", holdcat.LayerRisk, false},
		  {"ops", holdcat.LayerOps, false},
		  {"OPS", holdcat.LayerOps, false},
		  {"  ops  ", holdcat.LayerOps, false},
		   {"unknown", holdcat.LayerUnknown, true},
		   {"", holdcat.LayerUnknown, true},
		   {"FRAUD", holdcat.LayerUnknown, true},
		   {"kyc", holdcat.LayerUnknown, true},
	   }
	  for _, tc := range cases {
		  tc := tc
		   t.Run("input="+tc.input, func(t *testing.T) {
			   got, err := holdcat.ParseLayer(tc.input)
			   if (err != nil) != tc.wantErr {
				   t.Errorf("ParseLayer(%q) err=%v wantErr=%v", tc.input, err, tc.wantErr)
			  }
			   if !tc.wantErr && got != tc.want {
				  t.Errorf("ParseLayer(%q)=%v want %v", tc.input, got, tc.want)
			  }
		  })
	   }
   }

   func TestValidReasonForLayer(t *testing.T) {
	  cases := []struct {
		  layer  holdcat.Layer
		   reason holdcat.ReasonCode
		   want bool
	   }{
		   // legal 层合法原因码
		  {holdcat.LayerLegal, holdcat.ReasonCourtOrder, true},
		   {holdcat.LayerLegal, holdcat.ReasonRegulatoryFreeze, true},
		  {holdcat.LayerLegal, holdcat.ReasonLawEnforcement, true},
		   // legal 层拒绝其他层原因码
		  {holdcat.LayerLegal,
			   holdcat.ReasonSuspiciousTx, false},
		  {holdcat.LayerLegal, holdcat.ReasonKYTHighRisk, false},
		   {holdcat.LayerLegal, holdcat.ReasonWithdrawReview, false},
		  // aml 层合法原因码
		  {holdcat.LayerAML, holdcat.ReasonSuspiciousTx, true},
		   {holdcat.LayerAML,
			   holdcat.ReasonSARFiled, true},
		   {holdcat.LayerAML, holdcat.ReasonVASPRequest, true},
		   {holdcat.LayerAML, holdcat.ReasonHighRiskProfile, true},
		   // aml 层拒绝
		  {holdcat.LayerAML, holdcat.ReasonCourtOrder, false},
		  {holdcat.LayerAML, holdcat.ReasonWithdrawReview, false},
		  // risk 层
		  {holdcat.LayerRisk,
			   holdcat.ReasonKYTHighRisk, true},
		  {holdcat.LayerRisk, holdcat.ReasonKYCPending, true},
		   {holdcat.LayerRisk,
			   holdcat.ReasonVelocityBreach, true},
		  {holdcat.LayerRisk, holdcat.ReasonManualRiskHold, true},
		  {holdcat.LayerRisk, holdcat.ReasonChainReorgRisk, true},
		  {holdcat.LayerRisk, holdcat.ReasonCourtOrder, false},
		  {holdcat.LayerRisk, holdcat.ReasonWithdrawReview, false},
		  // ops 层
		  {holdcat.LayerOps, holdcat.ReasonWithdrawReview, true},
		   {holdcat.LayerOps, holdcat.ReasonPromoHold, true},
		   {holdcat.LayerOps, holdcat.ReasonOpsManualHold, true},
		  {holdcat.LayerOps, holdcat.ReasonFeeAdjustment, true},
		   {holdcat.LayerOps, holdcat.ReasonCourtOrder, false},
		  {holdcat.LayerOps, holdcat.ReasonKYTHighRisk, false},
		   // LayerUnknown 始终返回 false
		  {holdcat.LayerUnknown, holdcat.ReasonCourtOrder, false},
	  }
	   for _, tc := range cases {
		   tc := tc
		  name := tc.layer.Code() + "_" + string(tc.reason)
		   t.Run(name, func(t *testing.T) {
			  got := holdcat.ValidReasonForLayer(tc.layer, tc.reason)
			   if got != tc.want {
				   t.Errorf("ValidReasonForLayer(%s,%s)=%v want %v", tc.layer, tc.reason, got, tc.want)
			  }
		  })
	  }
   }

   func TestHighestLayer(t *testing.T) {
	   cases := []struct {
		   name   string
		  layers []holdcat.Layer
		  want   holdcat.Layer
	  }{
		  {"empty", nil, holdcat.LayerUnknown},
		  {"single_legal", []holdcat.Layer{holdcat.LayerLegal}, holdcat.LayerLegal},
		  {"single_ops", []holdcat.Layer{holdcat.LayerOps}, holdcat.LayerOps},
		   {"legal_ops", []holdcat.Layer{holdcat.LayerLegal, holdcat.LayerOps}, holdcat.LayerLegal},
		  {"risk_aml", []holdcat.Layer{holdcat.LayerRisk, holdcat.LayerAML}, holdcat.LayerAML},
		  {"all_four", []holdcat.Layer{holdcat.LayerOps, holdcat.LayerRisk, holdcat.LayerAML, holdcat.LayerLegal}, holdcat.LayerLegal},
		  {"risk_ops", []holdcat.Layer{holdcat.LayerRisk, holdcat.LayerOps}, holdcat.LayerRisk},
		   {"duplicate_aml", []holdcat.Layer{holdcat.LayerAML, holdcat.LayerAML}, holdcat.LayerAML},
	   }
	  for _, tc := range cases {
		  tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			   got := holdcat.HighestLayer(tc.layers)
			  if got != tc.want {
				   t.Errorf("HighestLayer=%v want %v", got, tc.want)
			  }
		  })
	  }
  }

   func TestBuildPriorityMatrix_Complete(t *testing.T) {
	   matrix := holdcat.BuildPriorityMatrix()
	  if len(matrix) != 16 {
		   t.Fatalf("matrix len=%d want 16 (4×4)", len(matrix))
	  }
	  // 主对角线：同层总是允许
	  for _, m := range matrix {
		   if m.Caller == m.Target && !m.Permitted {
			  t.Errorf("same-layer %s should always be permitted", m.Caller)
		   }
	   }
	  // 上三角（caller > target 先级）：总是允许
	  for _, m := range matrix {
		   if m.Caller.Priority() > m.Target.Priority() && !m.Permitted {
			  t.Errorf("%s(prio=%d) should be able to release %s(prio=%d)",
				   m.Caller, m.Caller.Priority(), m.Target, m.Target.Priority())
		   }
	  }
	   // 下三角（caller < target 先级）：总是拒绝
	  for _, m := range matrix {
		  if m.Caller.Priority() < m.Target.Priority() && m.Permitted {
			  t.Errorf("%s(prio=%d) should NOT be able to release %s(prio=%d)",
				   m.Caller,
				  m.Caller.Priority(), m.Target, m.Target.Priority())
		   }
	  }
   }
