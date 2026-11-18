package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testVersion = "0.1.25-addons.2"
const testRepo = "test/mygo-music"

func fixture(t *testing.T) (input, output, inventory, key string) {
	t.Helper()
	root := t.TempDir()
	input, output, inventory = filepath.Join(root, "in"), filepath.Join(root, "out"), filepath.Join(root, "inventory.json")
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	key = base64.StdEncoding.EncodeToString(public)
	write := func(target, name string, data []byte) {
		t.Helper()
		dir := filepath.Join(input, target)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, arch := range []string{"amd64", "arm64"} {
		rpmArch := map[string]string{"amd64": "x86_64", "arm64": "aarch64"}[arch]
		write("darwin-"+arch, "mygo-music-"+testVersion+"-"+arch+".dmg", []byte("dmg"))
		write("windows-"+arch, "mygo-music-setup-"+testVersion+"-"+arch+".exe", []byte("exe"))
		write("linux-"+arch, "mygo-music_0.1.25~addons.2_"+arch+".deb", []byte("deb"))
		write("linux-"+arch, "mygo-music-0.1.25~addons.2-1."+rpmArch+".rpm", []byte("rpm"))
		write("darwin-"+arch, "install.sh", []byte("same shell script"))
		write("linux-"+arch, "install.sh", []byte("same shell script"))
		write("windows-"+arch, "install.ps1", []byte("same PowerShell script"))
		for _, system := range []string{"darwin", "linux", "windows"} {
			target := system + "-" + arch
			sign := func(name string, data []byte) download {
				write(target, name, data)
				digest := sha256.Sum256(data)
				return download{
					URL:  "https://github.com/" + testRepo + "/releases/download/v" + testVersion + "/" + name,
					Size: int64(len(data)), Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(private, digest[:])),
				}
			}
			m := manifest{Version: testVersion, download: sign("mygo-music-"+testVersion+"-"+target+".tar.gz", []byte(target))}
			if target == "linux-amd64" {
				m.Deltas = []download{sign("mygo-music-0.1.25-addons-to-"+testVersion+"-"+target+".delta", []byte("delta"))}
			}
			data, err := json.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			write(target, "update-"+target+".json", data)
		}
	}
	return input, output, inventory, key
}

// Three platform outputs become one signed release with checksums under GitHub's filenames.
func TestPrepareRelease(t *testing.T) {
	input, output, inventory, key := fixture(t)
	if err := prepare(input, output, inventory, testRepo, testVersion, key); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(output, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"mygo-music_0.1.25.addons.2_amd64.deb", "mygo-music_0.1.25.addons.2_arm64.deb",
		"mygo-music-0.1.25.addons.2-1.x86_64.rpm", "mygo-music-0.1.25.addons.2-1.aarch64.rpm",
	} {
		file, err := inspect(filepath.Join(output, name), name)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(data, fmt.Appendf(nil, "%x  %s\n", file.hash, name)) {
			t.Fatalf("checksum missing for uploaded name %s", name)
		}
	}
	if bytes.ContainsAny(data, "~") || bytes.Count(data, []byte("  install.sh\n")) != 1 {
		t.Fatalf("wrong names or duplicate scripts:\n%s", data)
	}
	var assets []asset
	if err := readJSON(inventory, &assets); err != nil || len(assets) != 24 {
		t.Fatalf("inventory has %d assets: %v", len(assets), err)
	}
}

// A missing installer or a tampered signed update blocks draft upload.
func TestPrepareRejectsIncompleteOrTamperedBuild(t *testing.T) {
	for _, test := range []struct {
		file string
		edit func(string) error
		want string
	}{
		{"darwin-arm64/mygo-music-" + testVersion + "-arm64.dmg", os.Remove, "missing release asset"},
		{"linux-amd64/mygo-music-" + testVersion + "-linux-amd64.tar.gz", func(path string) error {
			return os.WriteFile(path, []byte("Linux-amd64"), 0o644)
		}, "size or signature"},
	} {
		t.Run(test.file, func(t *testing.T) {
			input, output, inventory, key := fixture(t)
			if err := test.edit(filepath.Join(input, test.file)); err != nil {
				t.Fatal(err)
			}
			if err := prepare(input, output, inventory, testRepo, testVersion, key); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got %v, want %q", err, test.want)
			}
		})
	}
}

// Draft retries upload only missing files and block publish on a missing or changed asset.
func TestCheckDraftAssets(t *testing.T) {
	payload := asset{"app.tar.gz", 50, "sha256:archive"}
	update := asset{"update-linux-amd64.json", 30, "sha256:manifest"}
	expected := []asset{payload, update}
	for _, test := range []struct {
		phase        string
		remote       []asset
		output, fail string
	}{
		{"payloads", nil, "app.tar.gz\n", ""},
		{"payloads", []asset{payload}, "", ""},
		{"manifests", []asset{payload}, "update-linux-amd64.json\n", ""},
		{"manifests", nil, "", "missing asset app.tar.gz"},
		{"complete", []asset{payload}, "", "missing asset update-linux-amd64.json"},
		{"complete", []asset{payload, update}, "", ""},
		{"payloads", []asset{{payload.Name, payload.Size, "sha256:changed"}}, "", "differs"},
	} {
		var output strings.Builder
		err := check(expected, test.remote, test.phase, &output)
		if (test.fail == "" && err != nil) || (test.fail != "" && (err == nil || !strings.Contains(err.Error(), test.fail))) || output.String() != test.output {
			t.Errorf("%s %v: output %q, error %v", test.phase, test.remote, output.String(), err)
		}
	}
}
