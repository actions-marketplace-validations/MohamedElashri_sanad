package workflow

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDiscoverWorkflowFiles(t *testing.T) {
	root := t.TempDir()
	workflows := filepath.Join(root, ".github", "workflows")
	if err := os.MkdirAll(filepath.Join(workflows, "nested.yml"), 0o755); err != nil {
		t.Fatal(err)
	}

	writeFile(t, filepath.Join(workflows, "build.yaml"))
	writeFile(t, filepath.Join(workflows, "ci.yml"))
	writeFile(t, filepath.Join(workflows, "notes.txt"))
	writeFile(t, filepath.Join(workflows, "nested", "release.yml"))

	got, err := DiscoverWorkflowFiles([]string{workflows})
	if err != nil {
		t.Fatalf("DiscoverWorkflowFiles returned error: %v", err)
	}

	want := []string{
		filepath.ToSlash(filepath.Join(workflows, "build.yaml")),
		filepath.ToSlash(filepath.Join(workflows, "ci.yml")),
		filepath.ToSlash(filepath.Join(workflows, "nested", "release.yml")),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DiscoverWorkflowFiles = %#v, want %#v", got, want)
	}
}

func TestDiscoverWorkflowFilesMissingDirectory(t *testing.T) {
	got, err := DiscoverWorkflowFiles([]string{filepath.Join(t.TempDir(), ".github", "workflows")})
	if err != nil {
		t.Fatalf("DiscoverWorkflowFiles returned error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("DiscoverWorkflowFiles = %#v, want empty list", got)
	}
}

func TestDiscoverWorkflowFilesFindsNestedGitHubWorkflowDirectories(t *testing.T) {
	root := t.TempDir()
	workflowFile := filepath.Join(root, "action", "test", "integration", ".github", "workflows", "fixture.yml")
	writeFile(t, workflowFile)
	writeFile(t, filepath.Join(root, "action", "node_modules", "pkg", ".travis.yml"))

	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })

	want := []string{"action/test/integration/.github/workflows/fixture.yml"}
	for _, configuredPath := range []string{"**/.github/workflows", `**\.github\workflows`} {
		got, err := DiscoverWorkflowFiles([]string{configuredPath})
		if err != nil {
			t.Fatalf("DiscoverWorkflowFiles(%q) returned error: %v", configuredPath, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("DiscoverWorkflowFiles(%q) = %#v, want %#v", configuredPath, got, want)
		}
	}
}

func TestDiscoverWorkflowFilesDeduplicates(t *testing.T) {
	root := t.TempDir()
	workflows := filepath.Join(root, ".github", "workflows")
	file := filepath.Join(workflows, "ci.yml")
	writeFile(t, file)

	got, err := DiscoverWorkflowFiles([]string{workflows, workflows})
	if err != nil {
		t.Fatalf("DiscoverWorkflowFiles returned error: %v", err)
	}

	want := []string{filepath.ToSlash(file)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DiscoverWorkflowFiles = %#v, want %#v", got, want)
	}
}

func TestWorkflowPathHelpersHandleWindowsStylePaths(t *testing.T) {
	path := `C:\repo\.github\workflows\ci.YML`
	if !isWorkflowYAML(path) {
		t.Fatalf("isWorkflowYAML(%q) = false, want true", path)
	}
	if got := normalizeWorkflowPath(path); got != "C:/repo/.github/workflows/ci.YML" {
		t.Fatalf("normalizeWorkflowPath(%q) = %q", path, got)
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("name: test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}
