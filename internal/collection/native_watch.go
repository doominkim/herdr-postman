package collection

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Signature fingerprints Native Git documents, including GUI-managed metadata.
// Content hashes also detect edits that preserve file sizes and timestamps.
func (s *NativeStore) Signature() string {
	hash := sha256.New()
	root, err := os.OpenRoot(s.Root)
	if err != nil {
		return fmt.Sprintf("unavailable:%v", err)
	}
	defer root.Close()
	err = fs.WalkDir(root.FS(), ".", func(file string, entry fs.DirEntry, err error) error {
		if err != nil {
			fmt.Fprintf(hash, "error:%s:%v\n", file, err)
			return nil
		}
		if entry.IsDir() {
			if file != "." && (entry.Name() == ".git" || entry.Name() == "node_modules" || strings.HasPrefix(entry.Name(), ".") && entry.Name() != ".resources") {
				return fs.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		ext := filepath.Ext(entry.Name())
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}
		fmt.Fprintf(hash, "file:%s\x00", file)
		f, err := root.Open(file)
		if err != nil {
			fmt.Fprintf(hash, "error:%v\n", err)
			return nil
		}
		digest := sha256.New()
		_, err = io.Copy(digest, f)
		f.Close()
		fmt.Fprintf(hash, "%x:%v\n", digest.Sum(nil), err)
		return nil
	})
	if err != nil {
		fmt.Fprintf(hash, "walk:%v", err)
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}
