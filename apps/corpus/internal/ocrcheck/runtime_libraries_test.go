package ocrcheck

import (
	"bytes"
	"debug/macho"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func writeMachO(t *testing.T, path string, libs, rpaths []string) {
	t.Helper()
	var loads bytes.Buffer
	command := func(kind uint32, offset int, value string) {
		size := (offset + len(value) + 1 + 7) &^ 7
		data := make([]byte, size)
		binary.LittleEndian.PutUint32(data, kind)
		binary.LittleEndian.PutUint32(data[4:], uint32(size))
		binary.LittleEndian.PutUint32(data[8:], uint32(offset))
		copy(data[offset:], value)
		loads.Write(data)
	}
	for _, lib := range libs {
		command(uint32(macho.LoadCmdDylib), 24, lib)
	}
	for _, path := range rpaths {
		command(uint32(macho.LoadCmdRpath), 12, path)
	}
	var data bytes.Buffer
	for _, value := range []uint32{macho.Magic64, uint32(macho.CpuArm64), 0, uint32(macho.TypeExec), uint32(len(libs) + len(rpaths)), uint32(loads.Len()), 0, 0} {
		if err := binary.Write(&data, binary.LittleEndian, value); err != nil {
			t.Fatal(err)
		}
	}
	data.Write(loads.Bytes())
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestRuntime_ResolvesLibraryClosure(t *testing.T) {
	root := runtimeTestDir(t)
	tool := filepath.Join(root, "Cellar/poppler/1/bin/pdfinfo")
	poppler := filepath.Join(root, "Cellar/poppler/1/lib/libpoppler.dylib")
	dep := filepath.Join(root, "Cellar/font/2/lib/libfont.dylib")
	leaf := filepath.Join(root, "Cellar/font/2/lib/libleaf.dylib")
	alias := filepath.Join(root, "opt/font")
	if err := os.MkdirAll(filepath.Dir(alias), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "Cellar/font/2"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "Cellar/font/2"), alias); err != nil {
		t.Fatal(err)
	}
	writeMachO(t, tool, []string{"@rpath/libpoppler.dylib"}, []string{"@loader_path/../lib"})
	writeMachO(t, poppler, []string{filepath.Join(alias, "lib/libfont.dylib")}, nil)
	writeMachO(t, dep, []string{"@loader_path/libleaf.dylib"}, nil)
	writeMachO(t, leaf, []string{"@loader_path/libfont.dylib"}, nil)
	got, err := resolveLibraries([]string{tool})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{tool, poppler, filepath.Join(alias, "lib/libfont.dylib"), dep, leaf}
	slices.Sort(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("closure=%v want=%v", got, want)
	}
	if err = os.Remove(leaf); err != nil {
		t.Fatal(err)
	}
	if _, err = resolveLibraries([]string{tool}); err == nil {
		t.Fatal("missing transitive library accepted")
	}
}
func TestRuntime_InheritsRpathsAndExecutablePath(t *testing.T) {
	dir := runtimeTestDir(t)
	tool := filepath.Join(dir, "bin/tool")
	library := filepath.Join(dir, "lib/libparent.dylib")
	leaf := filepath.Join(dir, "lib/libleaf.dylib")
	writeMachO(t, tool, []string{"@rpath/libparent.dylib"}, []string{"@executable_path/../lib"})
	writeMachO(t, library, []string{"@rpath/libleaf.dylib"}, nil)
	writeMachO(t, leaf, nil, nil)
	got, err := resolveLibraries([]string{tool})
	want := []string{tool, library, leaf}
	slices.Sort(want)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("inherited closure=%v error=%v", got, err)
	}
}
func TestRuntime_RejectsUnresolvedAndMalformedLibraries(t *testing.T) {
	for _, lib := range []string{"@rpath/absent.dylib", "@unknown/library", "relative.dylib"} {
		tool := filepath.Join(t.TempDir(), "tool")
		writeMachO(t, tool, []string{lib}, nil)
		if _, err := resolveLibraries([]string{tool}); err == nil {
			t.Fatalf("unresolved import accepted: %s", lib)
		}
	}
	tool := filepath.Join(t.TempDir(), "tool")
	if err := os.WriteFile(tool, []byte("not a Mach-O image"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveLibraries([]string{tool}); err == nil {
		t.Fatal("malformed image accepted")
	}
}

func runtimeTestDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRuntime_InstalledPopplerClosure(t *testing.T) {
	tools, err := discoverTools(runtime.GOOS, exec.LookPath, os.Stat)
	if err != nil {
		t.Skip("trusted native tools absent")
	}
	access, err := resolveRuntime([]string{tools.info, tools.raster})
	if err != nil {
		t.Fatal(err)
	}
	if len(access.files) <= 2 {
		t.Fatal("transitive runtime files absent")
	}
	for _, dir := range access.dirs {
		if dir == "/opt/homebrew/Cellar" || dir == "/opt/homebrew/opt" || dir == "/System/Library" || dir == "/usr/lib" || dir == "/usr/share" {
			t.Fatalf("broad runtime directory: %s", dir)
		}
	}
	profile := sandboxProfile(tools.info, "/private/tmp/ocr-test", access)
	for _, path := range access.files {
		if !strings.Contains(profile, `(literal `+strconv.Quote(path)+`)`) {
			t.Fatal("dependency absent from sandbox")
		}
	}
}

func TestRuntime_BareLoaderRpath(t *testing.T) {
	dir := runtimeTestDir(t)
	tool := filepath.Join(dir, "tool")
	lib := filepath.Join(dir, "libsame.dylib")
	writeMachO(t, tool, []string{"@rpath/libsame.dylib"}, []string{"@loader_path"})
	writeMachO(t, lib, nil, nil)
	got, err := resolveLibraries([]string{tool})
	want := []string{lib, tool}
	slices.Sort(want)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("bare loader path closure=%v error=%v", got, err)
	}
}
func TestRuntime_UniversalAndWeakImports(t *testing.T) {
	dir := runtimeTestDir(t)
	libA := filepath.Join(dir, "libA.dylib")
	libB := filepath.Join(dir, "libB.dylib")
	writeMachO(t, libA, nil, nil)
	writeMachO(t, libB, nil, nil)
	left, right := filepath.Join(dir, "arm"), filepath.Join(dir, "amd")
	writeMachO(t, left, []string{libA}, nil)
	writeMachO(t, right, []string{libB}, nil)
	arm, err := os.ReadFile(left)
	if err != nil {
		t.Fatal(err)
	}
	amd, err := os.ReadFile(right)
	if err != nil {
		t.Fatal(err)
	}
	binary.LittleEndian.PutUint32(amd[4:], uint32(macho.CpuAmd64))
	// The second slice uses a weak load command, which debug/macho leaves raw.
	binary.LittleEndian.PutUint32(amd[32:], 0x80000018)
	var fat bytes.Buffer
	for _, v := range []uint32{macho.MagicFat, 2, uint32(macho.CpuArm64), 0, 48, uint32(len(arm)), 2, uint32(macho.CpuAmd64), 0, uint32(48 + len(arm)), uint32(len(amd)), 2} {
		if err = binary.Write(&fat, binary.BigEndian, v); err != nil {
			t.Fatal(err)
		}
	}
	fat.Write(arm)
	fat.Write(amd)
	tool := filepath.Join(dir, "universal")
	if err = os.WriteFile(tool, fat.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := resolveLibraries([]string{tool})
	want := []string{tool, libA, libB}
	slices.Sort(want)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("universal closure=%v error=%v", got, err)
	}
}
