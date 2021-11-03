package ratelimit

import (
	"net/http"
	"account-assets.kucoin.net/funding/internal/perm"
	"github.com/gin-gonic/gin"
)

// QueryMW 绑在公共查询路上。超限 429，不得落到账本读。
  func QueryMW(g *QueryGuard, fallback string) gin.HandlerFunc {
	  if g == nil {
		  g = NewQuery(0, 0)
	  }
	   return func(c *gin.Context) {
		  actor := perm.FromRequest(c.Request, fallback).ID
		  if actor == "" {
			   actor = fallback
		  }
		  if actor == "" {
			  actor = "anon-query"
		   }
		  if err := g.Allow(actor); err != nil {
			  c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
			  return
		   }
		  c.Next()
	  }
   }

// WithdrawMW 绑在出金尝试路上。超限 429，不得接着冻额。
  func WithdrawMW(g *WithdrawGuard, fallback string) gin.HandlerFunc {
	   if g == nil {
		   g = NewWithdraw(0, 0)
	   }
	  return func(c *gin.Context) {
		  actor := perm.FromRequest(c.Request,
			  fallback).ID
		  if actor == "" {
			   actor = fallback
		  }
		   if actor == "" {
			   actor = "anon-wd"
		  }
		   if err := g.Allow(actor); err != nil {
			   c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
			   return
		  }
		   c.Next()
	   }
   }
