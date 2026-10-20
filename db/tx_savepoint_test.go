package db

import (
	"testing"

	"github.com/lanceadd/gooq"
)

func TestReleaseSavepointCapability(t *testing.T) {
	if supportsReleaseSavepoint(gooq.DialectDM) {
		t.Fatal("dm must not release savepoints")
	}
	if !supportsReleaseSavepoint(gooq.DialectMySQL) || !supportsReleaseSavepoint(gooq.DialectSQLite) {
		t.Fatal("known dialects must release savepoints")
	}
	if !supportsReleaseSavepoint(gooq.Dialect("unregistered")) {
		t.Fatal("unregistered dialect should default to release")
	}
}
