package latecreditgate

import "strings"

// CheckFence 校验围栏凭证。
// write=true：epoch=0 或 token 空必须返回围栏错误，调用方不得推进状态。
// write=false 且 policy.AllowLegacyRead：允许 epoch=0 && token 空（只读）。

   func CheckFence(fence FenceToken, write bool, policy FencePolicy) error {
	  if policy.Mode == "" {
		  policy = DefaultFencePolicy()
	   }
	  if !write && policy.AllowLegacyRead {
		  return nil
	   }
	   if fence.Epoch == 0 {
		   return ErrFenceEpochZero
	  }
	  if strings.TrimSpace(fence.Token) == "" {
		   return ErrFenceTokenEmpty
	  }
	  min := policy.MinEpoch
	  if min <= 0 {
		   min = 1
	   }
	  if fence.Epoch < min {
		   return ErrFenceRequired
	   }
	   if policy.RequireToken && strings.TrimSpace(fence.Token) == "" {
		   return ErrFenceTokenEmpty
	   }
	  return nil
  }

// MustFenceWrite 写路径入口：省略围栏一律拒绝。
  func MustFenceWrite(fence FenceToken) error {
	  return CheckFence(fence, true, DefaultFencePolicy())
   }

// IsOmittedFence 判断是否为历史可省略形态（epoch=0 且 token 空）。
  func IsOmittedFence(fence FenceToken) bool {
	   return fence.Epoch == 0 && strings.TrimSpace(fence.Token) == ""
  }

// ValidateWritePath 校验写路径名称。

  func ValidateWritePath(p WritePath) error {
	   switch p {
	   case PathReserve, PathReclaim, PathResolve, PathAutoCredit, PathOccupy, PathCredit:
		   return nil
	   default:
		   return ErrInvalidRequest
	   }
  }
