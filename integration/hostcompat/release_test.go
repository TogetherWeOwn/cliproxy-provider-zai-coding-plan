//go:build cgo && linux && amd64

package hostcompat

import (
	"archive/zip"
	"crypto/sha256"
	"debug/elf"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v8/internal/pluginstore"
)

func TestReleasePackagingAndRealStoreRegistry(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "registry.json"))
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := pluginstore.ParseRegistry(raw)
	if err != nil || manifest.SchemaVersion != 1 || len(manifest.Plugins) != 1 {
		t.Fatalf("real store registry parser %+v: %v", manifest, err)
	}
	p := manifest.Plugins[0]
	if p.ID != "zai-coding-plan" || p.Version != "0.1.2" || p.Repository != "https://github.com/TogetherWeOwn/cliproxy-provider-zai-coding-plan" || pluginstore.PluginInstallType(p) != "github-release" || !p.AuthRequired {
		t.Fatalf("store metadata %+v", p)
	}
	outDir := t.TempDir()
	cmd := exec.Command(filepath.Join(root, "scripts", "package-release.sh"), outDir)
	cmd.Dir = root
	// Even a caller's test-only build flags must not affect the release image.
	cmd.Env = append(os.Environ(), "GOFLAGS=-tags=hostcompat")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("package release: %v\n%s", err, out)
	}
	name := pluginstore.ArchiveName(p.ID, p.Version, "linux", "amd64")
	if name != "zai-coding-plan_0.1.2_linux_amd64.zip" {
		t.Fatalf("store archive name %q", name)
	}
	archive, err := os.ReadFile(filepath.Join(outDir, name))
	if err != nil {
		t.Fatal(err)
	}
	checksums, err := os.ReadFile(filepath.Join(outDir, "checksums.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(checksums) != fmt.Sprintf("%x  %s\n", sha256.Sum256(archive), name) {
		t.Fatal("release checksum does not match archive")
	}
	z, err := zip.OpenReader(filepath.Join(outDir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	if len(z.File) != 2 || z.File[0].Name != "zai-coding-plan.so" || z.File[1].Name != "LICENSE" {
		t.Fatalf("archive must contain only the production shared library and LICENSE: %+v", z.File)
	}
	wantLicense, err := os.ReadFile(filepath.Join(root, "LICENSE"))
	if err != nil {
		t.Fatal(err)
	}
	lr, err := z.File[1].Open()
	if err != nil {
		t.Fatal(err)
	}
	gotLicense, err := io.ReadAll(lr)
	lr.Close()
	if err != nil || string(gotLicense) != string(wantLicense) {
		t.Fatalf("archive LICENSE does not match repository LICENSE: %v", err)
	}
	r, err := z.File[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	lib := filepath.Join(t.TempDir(), "zai-coding-plan.so")
	body, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(lib, body, 0600); err != nil {
		t.Fatal(err)
	}
	e, err := elf.Open(lib)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if e.Machine != elf.EM_X86_64 || e.Type != elf.ET_DYN {
		t.Fatalf("not a Linux amd64 shared library: %+v", e.FileHeader)
	}
	symbols, err := e.DynamicSymbols()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range symbols {
		if s.Name == "cliproxy_plugin_init" && s.Section != elf.SHN_UNDEF {
			found = true
		}
	}
	if !found {
		t.Fatal("missing exported cliproxy_plugin_init")
	}
	// An accidental hostcompat-tagged release embeds the loopback test setting.
	if strings.Contains(string(body), "hostcompat_base") {
		t.Fatal("test-only upstream configuration leaked into release")
	}
}
