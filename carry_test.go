package agent

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanzoai/orm"
)

// The share index an earlier agent kept in {dataDir}/shares.db is carried into the
// host's database once, and read from there afterwards.
func TestTheOldShareIndexIsCarriedOnce(t *testing.T) {
	ctx := context.Background()
	host, dataDir := t.TempDir(), t.TempDir()
	old := newStore(dataDir, nil, nil)
	oldKeys, err := old.index()
	if err != nil {
		t.Fatal(err)
	}
	k := orm.New[ShareKey](oldKeys)
	k.SetId("hash-1")
	k.Org, k.Share = "acme", "share-1"
	if err := k.CreateCtx(ctx); err != nil {
		t.Fatal(err)
	}
	if err := old.closeAll(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(host, "shares.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	s := newStore(dataDir, nil, func() (*sql.DB, error) { return db, nil })
	keys, err := s.index()
	if err != nil {
		t.Fatal(err)
	}
	got, err := orm.Get[ShareKey](keys, "hash-1")
	if err != nil || got.Org != "acme" || got.Share != "share-1" {
		t.Fatalf("carried key = %+v, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "shares.db")); !os.IsNotExist(err) {
		t.Fatalf("the old file is still read: %v", err)
	}
}
