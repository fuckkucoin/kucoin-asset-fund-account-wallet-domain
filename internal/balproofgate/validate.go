package balproofgate

import (
	"strings"
	"unicode"
)

const (
	maxIDLen = 64
	maxNonceLen = 128
	maxBookLen  = 32
	maxAssetLen = 16
)

// NormalizeIssue 规范化签发请求。

   func NormalizeIssue(req IssueReq) (IssueReq, error) {
	  req.UserID = strings.TrimSpace(req.UserID)
	  req.Book = strings.TrimSpace(req.Book)
	   req.Asset = strings.ToUpper(strings.TrimSpace(req.Asset))
	   req.Nonce = strings.TrimSpace(req.Nonce)
	  if err := checkToken(req.UserID, maxIDLen, ErrUserInvalid); err != nil {
		   return req, err
	  }
	  if req.UserID == "system" {
		  return req, ErrUserInvalid
	  }
	  if err := checkToken(req.Book, maxBookLen, ErrBookInvalid); err != nil {
		   return req, err
	   }
	   if err := checkToken(req.Asset, maxAssetLen, ErrAssetInvalid); err != nil {
		  return req, err
	   }
	  if req.Nonce == "" || len(req.Nonce) > maxNonceLen {
		   return req, ErrNonceInvalid
	   }
	   if err := RejectPorOrStmtField(req.Nonce); err != nil {
		   return req, err
	  }
	   if req.TTLSec < 0 {
		  req.TTLSec = 0
	   }
	  if req.TTLSec > 3600 {
		   req.TTLSec = 3600
	  }
	   return req, nil
  }

  func checkToken(s string, max int, ferr error) error {
	   if s == "" || len(s) > max {
		   return ferr
	   }
	   for _, r := range s {
		  if r > 127 {
			   return ferr
		   }
		   if unicode.IsSpace(r) {
			  return ferr
		   }
		   if r == '/' || r == '\\' || r == 0 {
			  return ferr
		   }
	   }
	  return nil
   }
