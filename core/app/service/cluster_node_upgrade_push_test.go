package service

import (
	"os"
	"path"
	"testing"

	"github.com/1Panel-dev/1Panel/core/app/dto"
	"github.com/1Panel-dev/1Panel/core/global"
)

func TestNormalizeReleaseArch(t *testing.T) {
	cases := map[string]string{"x86_64": "amd64", "aarch64": "arm64", "arm64": "arm64", "armv7l": "armv7"}
	for in, want := range cases {
		if got, err := normalizeReleaseArch(in); err != nil || got != want {
			t.Fatalf("%s: got %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := normalizeReleaseArch("mips"); err == nil {
		t.Fatal("mips must be rejected")
	}
}

func TestPushedPackage(t *testing.T) {
	old := global.CONF.Base.InstallDir
	global.CONF.Base.InstallDir = t.TempDir()
	defer func() { global.CONF.Base.InstallDir = old }()

	dir := pushedPackageDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	name := forkPackageFileName("v2.20261126.1", "arm64")
	pkg := path.Join(dir, name)
	if err := os.WriteFile(pkg, []byte("package"), 0o600); err != nil {
		t.Fatal(err)
	}
	sum, _ := fileSha256(pkg)

	ok := dto.Upgrade{Version: "v2.20261126.1", PackagePath: pkg, PackageSha256: sum}
	if got, file, err := checkPushedPackage(ok, "arm64"); err != nil || got != pkg || file != name {
		t.Fatalf("valid package rejected: %v", err)
	}
	bad := []dto.Upgrade{
		{Version: "v2.20261126.1", PackagePath: pkg},                                               // no checksum
		{Version: "v2.20261126.1", PackagePath: "/etc/" + name, PackageSha256: sum},                // outside push dir
		{Version: "v2.20261126.1", PackagePath: dir + "/../pushed/../" + name, PackageSha256: sum}, // escapes
		{Version: "v2.20261126.2", PackagePath: pkg, PackageSha256: sum},                           // other version
	}
	for i, req := range bad {
		if _, _, err := checkPushedPackage(req, "arm64"); err == nil {
			t.Fatalf("case %d must be rejected", i)
		}
	}
	if _, _, err := checkPushedPackage(ok, "amd64"); err == nil {
		t.Fatal("package of another arch must be rejected")
	}

	dst := path.Join(t.TempDir(), name)
	if err := takePushedPackage(pkg, dst, "00"+sum[2:]); err == nil {
		t.Fatal("checksum mismatch must fail")
	}
	if _, err := os.Stat(pkg); !os.IsNotExist(err) {
		t.Fatal("a mismatching package must be removed")
	}
	_ = os.WriteFile(pkg, []byte("package"), 0o600)
	if err := takePushedPackage(pkg, dst, sum); err != nil {
		t.Fatalf("take package: %v", err)
	}
}
