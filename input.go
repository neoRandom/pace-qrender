package main

import (
	"archive/zip"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// openInput opens a directory or a .zip file and returns an fs.FS rooted at
// the directory that contains questions/ (and images/).
func openInput(path string) (fs.FS, func(), error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	var fsys fs.FS
	closeFn := func() {}
	switch {
	case st.IsDir():
		fsys = os.DirFS(path)
	case strings.EqualFold(filepath.Ext(path), ".zip"):
		zr, err := zip.OpenReader(path)
		if err != nil {
			return nil, nil, err
		}
		fsys, closeFn = &zr.Reader, func() { zr.Close() }
	default:
		return nil, nil, fmt.Errorf("%s is neither a directory nor a .zip file", path)
	}
	root, err := findRoot(fsys)
	if err != nil {
		closeFn()
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	if root != "." {
		sub, err := fs.Sub(fsys, root)
		if err != nil {
			closeFn()
			return nil, nil, err
		}
		fsys = sub
	}
	return fsys, closeFn, nil
}

func isDir(fsys fs.FS, name string) bool {
	st, err := fs.Stat(fsys, name)
	return err == nil && st.IsDir()
}

// findRoot accepts questions/ at the top level, or inside exactly one
// top-level folder (a zip made from a folder).
func findRoot(fsys fs.FS) (string, error) {
	if isDir(fsys, "questions") {
		return ".", nil
	}
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return "", err
	}
	var cands []string
	for _, e := range entries {
		if e.IsDir() && isDir(fsys, e.Name()+"/questions") {
			cands = append(cands, e.Name())
		}
	}
	if len(cands) == 1 {
		return cands[0], nil
	}
	return "", fmt.Errorf("no questions/ directory found")
}
