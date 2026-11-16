package app

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/Unfield/Tern/sealed"
	"github.com/Unfield/Tern/settings"
)

// setUpKeys gives the sealed package the key that encrypts mailbox
// passwords: PETREL_SECRET_KEY, or else a key Petrel makes once and keeps
// in petrel.key beside the database. Back that file up with the database:
// without it, the saved passwords can't be read.
func setUpKeys(dbPath string, logger *slog.Logger) error {
	if k := os.Getenv("PETREL_SECRET_KEY"); k != "" {
		if len(k) < 32 {
			return errors.New("app: PETREL_SECRET_KEY must be at least 32 characters")
		}
		var old []settings.Secret
		if o := os.Getenv("PETREL_OLD_SECRET_KEY"); o != "" {
			old = append(old, settings.Secret(o))
		}
		return sealed.SetKeys(settings.Secret(k), old...)
	}
	path := filepath.Join(filepath.Dir(dbPath), "petrel.key")
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			return err
		}
		b = []byte(hex.EncodeToString(buf))
		if err := os.WriteFile(path, b, 0o600); err != nil {
			return err
		}
		logger.Warn("made a key for mailbox passwords; set PETREL_SECRET_KEY in production, or back up this file", "path", path)
	} else if err != nil {
		return err
	}
	return sealed.SetKeys(settings.Secret(strings.TrimSpace(string(b))))
}
