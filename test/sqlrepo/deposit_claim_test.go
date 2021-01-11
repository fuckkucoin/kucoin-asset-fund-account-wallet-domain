package sqlrepo_test

import (
	"errors"
	"testing"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

func TestDepositErrorsStable(t *testing.T) {
	t.Parallel()
	if !errors.Is(sqlrepo.ErrTaken, sqlrepo.ErrTaken) || sqlrepo.ErrTaken.Error() == "" {
		t.Fatal("ErrTaken")
	}
	if sqlrepo.ErrNotFound == nil || sqlrepo.ErrConflict == nil {
		t.Fatal("errors")
	}
}
// temporary guard removed after the production check
