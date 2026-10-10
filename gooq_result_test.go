package gooq

import (
	"database/sql"
	"testing"
)

func TestRecordStructOrmTag(t *testing.T) {
	var row struct {
		MetaField string  `json:"other" orm:"meta"`
		Name      string  `json:"name"`
		Alias     *string `json:"aliasName" orm:"alias"`
	}
	err := Record{"meta": "m", "name": "n", "alias": nil}.Struct(&row)
	if err != nil {
		t.Fatal(err)
	}
	if row.MetaField != "m" || row.Name != "n" || row.Alias != nil {
		t.Fatalf("scanned row: %+v", row)
	}

	var pointer *struct {
		Name string `orm:"name"`
	}
	if err = (Record{"name": "n"}).Struct(&pointer); err != nil {
		t.Fatal(err)
	}
	if pointer == nil || pointer.Name != "n" {
		t.Fatalf("scanned pointer: %+v", pointer)
	}

	var existing struct {
		Name string `orm:"name"`
	}
	if err = (Record{}).Struct(&existing); err != sql.ErrNoRows {
		t.Fatalf("empty record error: %v", err)
	}
}
