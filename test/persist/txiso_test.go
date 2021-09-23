package persist_test

import (
	"errors"
	"testing"

	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/go-sql-driver/mysql"
	"github.com/lib/pq"
)

func TestRetryableTxConflict(t *testing.T) {
	if persist.RetryableTxConflict(nil) {
		t.Fatal("nil")
	}
	if !persist.RetryableTxConflict(&mysql.MySQLError{Number: 1213, Message: "Deadlock found"}) {
		t.Fatal("mysql deadlock")
	}
	if persist.RetryableTxConflict(&mysql.MySQLError{Number: 1205, Message: "Lock wait timeout"}) {
		t.Fatal("lock wait must not retry")
	}
	if !persist.RetryableTxConflict(&pq.Error{Code: "40001", Message: "could not serialize access"}) {
		t.Fatal("pg serialization")
	}
	if persist.RetryableTxConflict(errors.New("insufficient available balance")) {
		t.Fatal("business error")
	}
}
