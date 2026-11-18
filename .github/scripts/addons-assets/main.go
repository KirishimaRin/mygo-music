// Command addons-assets verifies the Addons matrix outputs and the draft
// release's asset inventory without downloading the uploaded files again.
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

type asset struct {
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"`
}

type localAsset struct {
	asset
	path string
	hash []byte
}

type download struct {
	URL       string `json:"url"`
	Size      int64  `json:"size"`
	Signature string `json:"signature"`
}

type manifest struct {
	Version string `json:"version"`
	download
	Deltas []download `json:"deltas,omitempty"`
}

var safeName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

func main() {
	var err error
	args := os.Args[1:]
	switch {
	case len(args) == 6 && args[0] == "prepare":
		err = prepare(args[1], args[2], args[3], args[4], args[5], os.Getenv("PUBLIC_KEY"))
	case len(args) == 4 && args[0] == "check":
		var local, remote []asset
		if err = readJSON(args[1], &local); err == nil {
			if err = readJSON(args[2], &remote); err == nil {
				err = check(local, remote, args[3], os.Stdout)
			}
		}
	default:
		err = errors.New("expected prepare input output inventory repo version or check inventory remote payloads|manifests|complete")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, value); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

func inspect(path, name string) (localAsset, error) {
	f, err := os.Open(path)
	if err != nil {
		return localAsset{}, err
	}
	defer f.Close()
	sum := sha256.New()
	n, err := io.Copy(sum, f)
	if err != nil {
		return localAsset{}, err
	}
	if n == 0 {
		return localAsset{}, fmt.Errorf("empty release asset: %s", path)
	}
	hash := sum.Sum(nil)
	return localAsset{asset{name, n, "sha256:" + hex.EncodeToString(hash)}, path, hash}, nil
}

func collect(input string) (map[string]localAsset, error) {
	files := map[string]localAsset{}
	err := filepath.WalkDir(input, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".deb") || strings.HasSuffix(name, ".rpm") {
			// GitHub replaces ~ in asset filenames. Normalize before hashing and uploading.
			name = strings.ReplaceAll(name, "~", ".")
		}
		if !entry.Type().IsRegular() || !safeName.MatchString(name) {
			return fmt.Errorf("unexpected release asset: %s", path)
		}
		file, err := inspect(path, name)
		if err != nil {
			return err
		}
		if other, ok := files[name]; ok {
			if (name == "install.sh" || name == "install.ps1") && other.Digest == file.Digest {
				return nil
			}
			return fmt.Errorf("duplicate release asset: %s", name)
		}
		files[name] = file
		return nil
	})
	return files, err
}

// expectedFiles accounts for every supported installer, all six signed full
// updates, and any delta referenced by their manifests.
func expectedFiles(files map[string]localAsset, repo, version string, key ed25519.PublicKey) (map[string]bool, error) {
	expected := map[string]bool{"install.sh": true, "install.ps1": true}
	baseURL := "https://github.com/" + repo + "/releases/download/v" + version + "/"
	for _, arch := range []string{"amd64", "arm64"} {
		rpmArch := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[arch]
		linuxVersion := strings.ReplaceAll(version, "-", ".")
		expected["mygo-music-"+version+"-"+arch+".dmg"] = true
		expected["mygo-music-setup-"+version+"-"+arch+".exe"] = true
		expected["mygo-music_"+linuxVersion+"_"+arch+".deb"] = true
		expected["mygo-music-"+linuxVersion+"-1."+rpmArch+".rpm"] = true
		for _, system := range []string{"darwin", "linux", "windows"} {
			target := system + "-" + arch
			name := "update-" + target + ".json"
			expected[name] = true
			var m manifest
			if file, ok := files[name]; !ok {
				return nil, fmt.Errorf("missing manifest: %s", name)
			} else if err := readJSON(file.path, &m); err != nil {
				return nil, err
			}
			archive := "mygo-music-" + version + "-" + target + ".tar.gz"
			if m.Version != version || m.URL != baseURL+archive {
				return nil, fmt.Errorf("%s points to the wrong version or archive", name)
			}
			for _, d := range append([]download{m.download}, m.Deltas...) {
				assetName, ok := strings.CutPrefix(d.URL, baseURL)
				if !ok || assetName != archive && !strings.HasSuffix(assetName, "-to-"+version+"-"+target+".delta") {
					return nil, fmt.Errorf("%s points outside its target's release assets: %s", name, d.URL)
				}
				file, ok := files[assetName]
				sig, err := base64.StdEncoding.DecodeString(d.Signature)
				if !ok || file.Size != d.Size || err != nil || !ed25519.Verify(key, file.hash, sig) {
					return nil, fmt.Errorf("%s has no matching file, size or signature for %s", name, assetName)
				}
				expected[assetName] = true
			}
		}
	}
	return expected, nil
}

func prepare(input, output, inventoryPath, repo, version, publicKey string) error {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(publicKey))
	if err != nil || len(key) != ed25519.PublicKeySize {
		return errors.New("invalid Addons updater public key")
	}
	files, err := collect(input)
	if err != nil {
		return err
	}
	expected, err := expectedFiles(files, repo, version, key)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(files))
	for name := range files {
		if !expected[name] {
			return fmt.Errorf("unexpected release asset: %s", name)
		}
		names = append(names, name)
	}
	for name := range expected {
		if _, ok := files[name]; !ok {
			return fmt.Errorf("missing release asset: %s", name)
		}
	}
	slices.Sort(names)
	if err := os.Mkdir(output, 0o755); err != nil {
		return err
	}
	var sums strings.Builder
	inventory := make([]asset, 0, len(files)+1)
	for _, name := range names {
		file := files[name]
		dst := filepath.Join(output, name)
		if err := os.Rename(file.path, dst); err != nil {
			return err
		}
		inventory = append(inventory, file.asset)
		if !isManifest(name) {
			fmt.Fprintf(&sums, "%x  %s\n", file.hash, name)
		}
	}
	path := filepath.Join(output, "SHA256SUMS")
	if err := os.WriteFile(path, []byte(sums.String()), 0o644); err != nil {
		return err
	}
	checksums, err := inspect(path, "SHA256SUMS")
	if err != nil {
		return err
	}
	data, err := json.Marshal(append(inventory, checksums.asset))
	if err != nil {
		return err
	}
	return os.WriteFile(inventoryPath, data, 0o644)
}

func isManifest(name string) bool {
	return strings.HasPrefix(name, "update-") && strings.HasSuffix(name, ".json")
}

func check(local, remote []asset, phase string, missing io.Writer) error {
	if phase != "payloads" && phase != "manifests" && phase != "complete" {
		return fmt.Errorf("unknown verification phase: %s", phase)
	}
	expected := make(map[string]asset, len(local))
	for _, a := range local {
		expected[a.Name] = a
	}
	for _, a := range remote {
		want, ok := expected[a.Name]
		if !ok || want != a {
			return fmt.Errorf("draft asset %q is unexpected or differs from the verified build; start a new dispatch", a.Name)
		}
		delete(expected, a.Name)
	}
	for _, a := range local {
		if _, ok := expected[a.Name]; !ok {
			continue
		}
		manifest := isManifest(a.Name)
		if phase == "complete" || (phase == "manifests" && !manifest) {
			return fmt.Errorf("draft is missing asset %s", a.Name)
		}
		if (phase == "manifests") == manifest {
			if _, err := fmt.Fprintln(missing, a.Name); err != nil {
				return err
			}
		}
	}
	return nil
}
