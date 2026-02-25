package datadir

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
)

const (
	dirName  = ".coderag-go"
	projects = "projects"
	hashLen  = 16
)

// DataDir returns the project-specific data directory under ~/.coderag-go/projects/<hash>/.
// Hash is the first 16 characters of SHA-256 of the absolute path of root.
func DataDir(root string) (string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256([]byte(filepath.Clean(abs)))
	hash := hex.EncodeToString(h[:])[:hashLen]
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, dirName, projects, hash), nil
}
