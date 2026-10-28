package shellcomplete

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const blockStart = "# >>> ClipRAG terminal completion >>>"
const blockEnd = "# <<< ClipRAG terminal completion <<<"

// UpdateRC changes only ClipRAG's marked block, preserving other shell setup.
// A first-time backup is kept beside an existing startup file.
func UpdateRC(path, shell, binary string, enable bool) (bool, error) {
	if !filepath.IsAbs(path) || !filepath.IsAbs(binary) {
		return false, errors.New("startup file and binary paths must be absolute")
	}
	if shell != "bash" && shell != "zsh" {
		return false, errors.New("choose bash or zsh")
	}
	info, err := os.Lstat(path)
	exists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if exists && !info.Mode().IsRegular() {
		return false, errors.New("startup file must be regular; for symlinked dotfiles use --rc-file with the actual target")
	}
	old := []byte{}
	if exists {
		old, err = os.ReadFile(path)
		if err != nil {
			return false, err
		}
	}
	text := string(old)
	start, end := strings.Index(text, blockStart), strings.Index(text, blockEnd)
	if (start < 0) != (end < 0) || (start >= 0 && end < start) || strings.Count(text, blockStart) > 1 || strings.Count(text, blockEnd) > 1 {
		return false, errors.New("incomplete or duplicate ClipRAG startup block; review the file before installing")
	}
	if start >= 0 && ((start > 0 && text[start-1] != '\n') || (end+len(blockEnd) < len(text) && text[end+len(blockEnd)] != '\n')) {
		return false, errors.New("ClipRAG markers must be on separate lines; review the startup file")
	}
	updated := text
	block := ""
	if enable {
		block = blockStart + "\nif [ -x " + Quote(binary) + " ]; then\n    eval \"$(" + Quote(binary) + " init " + shell + ")\"\nfi\n" + blockEnd + "\n"
	}
	if start >= 0 {
		end += len(blockEnd)
		if end < len(text) && text[end] == '\n' {
			end++
		}
		updated = text[:start] + block + text[end:]
	} else if enable {
		if updated != "" && !strings.HasSuffix(updated, "\n") {
			updated += "\n"
		}
		updated += block
	}
	if bytes.Equal(old, []byte(updated)) {
		return false, nil
	}
	if exists {
		backup, e := os.OpenFile(path+".cliprag-backup", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e == nil {
			_, e = backup.Write(old)
			closeErr := backup.Close()
			if e != nil {
				return false, e
			}
			if closeErr != nil {
				return false, closeErr
			}
		} else if !errors.Is(e, os.ErrExist) {
			return false, e
		}
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return false, err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".cliprag-rc-*")
	if err != nil {
		return false, err
	}
	defer os.Remove(temp.Name())
	mode := os.FileMode(0600)
	if exists {
		mode = info.Mode().Perm()
	}
	if err = temp.Chmod(mode); err == nil {
		_, err = temp.WriteString(updated)
	}
	if err == nil {
		err = temp.Sync()
	}
	closeErr := temp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return false, err
	}
	if err = os.Rename(temp.Name(), path); err != nil {
		return false, err
	}
	return true, nil
}
