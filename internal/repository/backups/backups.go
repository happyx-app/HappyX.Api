// Package backups lists the database backups the stack's backup container writes, so the admin
// dashboard can show the latest ones. It only reads names, sizes and dates, never the files.
package backups

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/happyx-app/HappyX.Api/internal/domain"
)

// Max is how many backups List returns.
const Max = 7

// Dir is a folder of backups as postgres-backup-local writes them: one file per day under
// daily/, e.g. daily/happyx-20261025.sql.gz. Files right in the folder are listed too.
type Dir struct{ Path string }

func (d Dir) List(_ context.Context) ([]domain.Backup, bool, error) {
	if d.Path == "" {
		return nil, false, nil
	}
	var out []domain.Backup
	for _, sub := range []string{"daily", "."} {
		entries, err := os.ReadDir(filepath.Join(d.Path, sub))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, true, err
		}
		for _, e := range entries {
			if !e.Type().IsRegular() || !isBackup(e.Name()) {
				continue
			}
			info, err := e.Info()
			if err != nil {
				return nil, true, err
			}
			out = append(out, domain.Backup{Name: e.Name(), Size: info.Size(), Modified: info.ModTime().UTC()})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Modified.After(out[j].Modified) })
	if len(out) > Max {
		out = out[:Max]
	}
	return out, true, nil
}

func isBackup(name string) bool {
	return strings.HasSuffix(name, ".sql.gz") || strings.HasSuffix(name, ".sql") || strings.HasSuffix(name, ".dump")
}
