package sqlrepo

import "account-assets.kucoin.net/funding/internal/persist"

  func sqlOn(db *persist.SQLDB) bool {
	   return db != nil && db.DB != nil
  }
