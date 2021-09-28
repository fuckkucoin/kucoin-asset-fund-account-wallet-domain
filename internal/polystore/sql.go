package polystore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/policypack"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

   func (s *Store) BindSQL(db *persist.SQLDB) *Store {
	   if s != nil {
		  s.sql = db
	  }
	  return s
  }

  func (s *Store) sqlOn() bool {
	  return s != nil && s.sql != nil && s.sql.DB != nil
   }

   func (s *Store) publishSQL(ctx context.Context, p policypack.Pack, body []byte) error {
	   sum := sha256.Sum256(body)
	   return sqlrepo.NewPolicy(s.sql).Put(ctx, sqlrepo.PolicyDoc{
		   Pack: strings.ToLower(strings.TrimSpace(p.Name)), Version: p.Doc.Version, Body: string(body),
	   }, sqlrepo.PolicyMeta{
		   Pack: strings.ToLower(strings.TrimSpace(p.Name)), Version: p.Doc.Version, KYC: p.KYC,
		   DestReuseMax: p.DestReuseMax, HourlyVelocity: p.HourlyVelocity, DailyVelocity: p.DailyVelocity,
		   Checksum: hex.EncodeToString(sum[:]),
	  })
   }

   func (s *Store) getSQL(ctx context.Context, pack string, ver int) (policypack.Pack, error) {
	   doc, _, err := sqlrepo.NewPolicy(s.sql).Get(ctx, pack, ver)
	  if err != nil {
		  return policypack.Pack{}, err
	  }
	   var p policypack.Pack
	  if err := json.Unmarshal([]byte(doc.Body), &p); err != nil {
		  return policypack.Pack{}, err
	  }
	   return p, nil
  }

  func (s *Store) latestSQL(ctx context.Context, pack string) (policypack.Pack, error) {
	  doc, _, err := sqlrepo.NewPolicy(s.sql).Latest(ctx, pack)
	   if err != nil {
		  return policypack.Pack{}, err
	  }
	   var p policypack.Pack
	  if err := json.Unmarshal([]byte(doc.Body), &p); err != nil {
		  return policypack.Pack{}, err
	   }
	  return p, nil
   }

   func ignoreSQLMiss(err error) bool {
	   return errors.Is(err, sqlrepo.ErrNotFound) || errors.Is(err, sqlrepo.ErrUnavailable)
   }
