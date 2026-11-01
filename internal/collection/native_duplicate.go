package collection

import (
	"crypto/rand"
	"fmt"
	"io/fs"
	"os"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

// duplicateExamples gives a copy its own example documents. Upload references
// stay unchanged because the new request lives beside the original request.
func (s *NativeStore) duplicateExamples(data []byte, source, destination string) ([]byte, func(), error) {
	noop := func() {}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, noop, err
	}
	ref := nativeNode(doc.Content[0], "examples")
	if ref == nil {
		return data, noop, nil
	}
	if ref.Kind != yaml.ScalarNode || ref.Tag != "!!str" || ref.Value == "" || strings.HasPrefix(ref.Value, "/") || strings.Contains(ref.Value, "\\") {
		return nil, noop, fmt.Errorf("invalid native examples directory")
	}
	sourceDir := path.Clean(path.Join(path.Dir(source), ref.Value))
	if !fs.ValidPath(sourceDir) || sourceDir == "." {
		return nil, noop, fmt.Errorf("examples directory must stay inside collection")
	}
	root, err := os.OpenRoot(s.Root)
	if err != nil {
		return nil, noop, err
	}
	defer root.Close()
	// Reject symlinks in ancestors as well as in the copied tree.
	current := ""
	for _, part := range strings.Split(sourceDir, "/") {
		current = path.Join(current, part)
		info, err := root.Lstat(current)
		if err != nil {
			return nil, noop, err
		}
		if !info.IsDir() {
			return nil, noop, fmt.Errorf("examples path must contain only directories")
		}
	}
	parent := path.Join(path.Dir(destination), ".resources")
	if parent == sourceDir || strings.HasPrefix(parent, sourceDir+"/") {
		return nil, noop, fmt.Errorf("examples directory cannot contain its copy destination")
	}
	if err := root.MkdirAll(parent, 0700); err != nil {
		return nil, noop, err
	}
	resourceName := strings.TrimSuffix(path.Base(destination), NativeSuffix) + "-" + rand.Text() + ".resources"
	target := path.Join(parent, resourceName)
	if err := root.Mkdir(target, 0700); err != nil {
		return nil, noop, err
	}
	cleanup := func() {
		r, err := os.OpenRoot(s.Root)
		if err == nil {
			defer r.Close()
			_ = r.RemoveAll(target)
		}
	}
	exampleTarget := path.Join(target, "examples")
	err = fs.WalkDir(root.FS(), sourceDir, func(file string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative := strings.TrimPrefix(strings.TrimPrefix(file, sourceDir), "/")
		dest := path.Join(exampleTarget, relative)
		if entry.IsDir() {
			return root.Mkdir(dest, 0700)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("example resources must be regular files")
		}
		content, err := root.ReadFile(file)
		if err != nil {
			return err
		}
		f, err := root.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, err = f.Write(content)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		return closeErr
	})
	if err != nil {
		cleanup()
		return nil, noop, err
	}
	ref.SetString("./.resources/" + resourceName + "/examples/")
	rendered, err := yaml.Marshal(&doc)
	if err != nil {
		cleanup()
		return nil, noop, err
	}
	return rendered, cleanup, nil
}
