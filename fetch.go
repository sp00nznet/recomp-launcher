package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// fetch puts repo at ref into dest from GitHub's zip of it, so the player
// needs no git. A git checkout already there is left alone: its setup
// updates it. Files the new version dropped stay behind.
func fetch(repo, ref, dest string) error {
	if isDir(filepath.Join(dest, ".git")) {
		return nil
	}
	resp, err := http.Get("https://github.com/" + repo + "/archive/" + ref + ".zip")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("downloading %s: %s", repo, resp.Status)
	}
	tmp, err := os.CreateTemp("", "recomp-*.zip")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		return fmt.Errorf("downloading %s: %w", repo, err)
	}
	return unzip(tmp.Name(), dest)
}

// unzip extracts a GitHub archive into dest without its top folder (<repo>-<ref>/).
func unzip(zipPath, dest string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()
	dest = filepath.Clean(dest)
	for _, f := range r.File {
		_, rel, _ := strings.Cut(f.Name, "/")
		if rel == "" {
			continue
		}
		p := filepath.Join(dest, filepath.FromSlash(rel))
		if !strings.HasPrefix(p, dest+string(os.PathSeparator)) {
			return fmt.Errorf("%s: a path outside the folder", f.Name)
		}
		mode := f.Mode()
		switch {
		case mode.IsDir():
			if err := os.MkdirAll(p, 0o755); err != nil {
				return err
			}
			continue
		case mode&os.ModeSymlink != 0:
			continue // ponytail: no repo of ours has one; write the link if one ever does
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := extract(f, p, 0o644|mode&0o111); err != nil {
			return err
		}
	}
	return nil
}

func extract(f *zip.File, p string, mode os.FileMode) error {
	in, err := f.Open()
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chmod(p, mode) // an existing file keeps its old mode otherwise
}

// newer reads GitHub's release list (newest first, pre-releases included)
// and gives the newest release's tag and page when it isn't version.
func newer(list []byte, version string) (tag, url string) {
	var rs []struct {
		Tag   string `json:"tag_name"`
		URL   string `json:"html_url"`
		Draft bool   `json:"draft"`
	}
	json.Unmarshal(list, &rs)
	for _, r := range rs {
		if !r.Draft {
			if r.Tag != version {
				return r.Tag, r.URL
			}
			break
		}
	}
	return "", ""
}

// latest asks GitHub for repo's newest release; offline or rate-limited, it
// says nothing. /releases/latest skips pre-releases, and alphas are all we have.
func latest(repo, version string) (tag, url string) {
	c := http.Client{Timeout: 5 * time.Second}
	resp, err := c.Get("https://api.github.com/repos/" + repo + "/releases?per_page=5")
	if err != nil {
		return "", ""
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", ""
	}
	return newer(b, version)
}
