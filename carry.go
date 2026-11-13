package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/hanzoai/orm"
	ormdb "github.com/hanzoai/orm/db"
)

// carryShares brings the share index an earlier agent kept in {dataDir}/shares.db
// into the host's database, once: every key and viewer is created where it is not
// already, and the file is renamed so the next open does not read it. A host with no
// such file has nothing to carry.
func (s *store) carryShares(into orm.DB) error {
	path := filepath.Join(s.dataDir, "shares.db")
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	old, err := orm.OpenSQLite(&ormdb.SQLiteDBConfig{
		Path:   path,
		Config: ormdb.SQLiteConfig{BusyTimeout: 5000, JournalMode: "WAL"},
	})
	if err != nil {
		return err
	}
	ctx := context.Background()
	keys, err := orm.TypedQuery[ShareKey](old).GetAll(ctx)
	if err != nil {
		_ = old.Close()
		return err
	}
	viewers, err := orm.TypedQuery[Viewer](old).GetAll(ctx)
	if err != nil {
		_ = old.Close()
		return err
	}
	for _, k := range keys {
		n := orm.New[ShareKey](into)
		n.SetId(k.Id())
		n.Org, n.Share = k.Org, k.Share
		n.CreatedAt, n.UpdatedAt = k.CreatedAt, k.UpdatedAt
		if _, err := into.CreateIfAbsent(ctx, n.Key(), n); err != nil {
			_ = old.Close()
			return err
		}
	}
	for _, v := range viewers {
		n := orm.New[Viewer](into)
		n.SetId(v.Id())
		n.Org, n.Share, n.ConversationId, n.User, n.Name, n.Removed = v.Org, v.Share, v.ConversationId, v.User, v.Name, v.Removed
		n.CreatedAt, n.UpdatedAt = v.CreatedAt, v.UpdatedAt
		if _, err := into.CreateIfAbsent(ctx, n.Key(), n); err != nil {
			_ = old.Close()
			return err
		}
	}
	if err := old.Close(); err != nil {
		return err
	}
	return os.Rename(path, path+".carried")
}
