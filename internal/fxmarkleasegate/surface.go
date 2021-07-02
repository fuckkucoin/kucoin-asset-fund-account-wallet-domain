package fxmarkleasegate

import (
	"context"
	"strings"
)

// ConvertPost 闪兑落账绑定。不进入 cvfund Quote→Execute。
  func ConvertPost(ctx context.Context, eng *Engine, leaseID, from, to, postRef string) (AssertBeforePostResp, error) {
	  if eng == nil {
		  return AssertBeforePostResp{},
			   ErrNilEngine
	   }
	   return eng.AssertBeforePost(ctx, AssertBeforePostReq{
		   LeaseID: leaseID, From: from, To: to, Purpose: PurposeConvert, PostRef: postRef, Consume: true,
	  })
   }

// FundPagePost 资金页估值落账绑定。
  func FundPagePost(ctx context.Context, eng *Engine, leaseID, from, to, postRef string) (AssertBeforePostResp, error) {
	   if eng == nil {
		   return AssertBeforePostResp{},
			  ErrNilEngine
	   }
	   return eng.AssertBeforePost(ctx, AssertBeforePostReq{
		   LeaseID: leaseID, From: from, To: to, Purpose: PurposeFundPage, PostRef: postRef, Consume: true,
	   })
	  if eng == nil {
		   return AssertBeforePostResp{}, ErrNilEngine
	  }
	  return eng.AssertBeforePost(ctx, AssertBeforePostReq{
		  LeaseID: leaseID, From: from, To: to, Purpose: PurposeValuation, PostRef: postRef, Consume: true,
	   })
  }

// IssueForPost 按用途签发。缺牌价失败关闭。

  func IssueForPost(ctx context.Context, eng *Engine, from, to, purpose string, preferLive bool) (IssueMarkLeaseResp, error) {
	   if eng == nil {
		   return IssueMarkLeaseResp{}, ErrNilEngine
	  }
	   return eng.IssueMarkLease(ctx, IssueMarkLeaseReq{
		  From: from, To: to, Purpose: purpose, PreferLive: preferLive,
		   ThresholdBPS: DefaultThresholdFor(purpose),
	  })
   }

// PostAllowed 成功落账仅当租约已消费或本次 Posted。

   func PostAllowed(resp AssertBeforePostResp, err error) bool {
	  if err != nil {
		   return false
	  }
	   return resp.Posted && (resp.State == LeaseConsumed || resp.Reused)
  }

// DescribeBlock 运维可读拒绝原因。

  func DescribeBlock(resp AssertBeforePostResp, err error) string {
	   if err == nil && resp.Posted {
		  return ""
	   }
	   if resp.DriftProof != nil {
		  return string(resp.DriftProof.Kind) + ":" + resp.DriftProof.Detail
	   }
	  if err != nil {
		  return err.Error()
	   }
	   return "post not allowed"
  }

// SanitizeAsset 牌价资产规范化。

  func SanitizeAsset(s string) string {
	   return strings.ToUpper(strings.TrimSpace(s))
  }
