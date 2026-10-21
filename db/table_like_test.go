package db

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateTableLikeUnsupportedDialect(t *testing.T) {
	path := filepath.Join(t.TempDir(), "like.db")
	database, err := Open(Config{Link: "sqlite::@file(" + filepath.ToSlash(path) + ")"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	}()

	err = database.CreateTableLike(context.Background(), "template", "target")
	if err == nil || !strings.Contains(err.Error(), "does not support CREATE TABLE LIKE") {
		t.Fatalf("unexpected error: %v", err)
	}
}
