package changelogger

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
)

func TestParseAndValidateFragment(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bootstrap.md")
	err := os.WriteFile(file, []byte("---\ncomponent: example-service\nbump: patch\ntype: fix\n---\n\nFix bootstrap debug flow.\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	fragment, err := ParseFragment(file)
	if err != nil {
		t.Fatal(err)
	}
	if errors := ValidateFragment(fragment, "example-service"); len(errors) != 0 {
		t.Fatalf("expected valid fragment, got %v", errors)
	}
	if got := ReleasePleaseSubject(fragment); got != "fix(example-service): Fix bootstrap debug flow" {
		t.Fatalf("unexpected subject: %s", got)
	}
}

func TestValidateReleaseSignalRequiresMatchingTitle(t *testing.T) {
	fragment := Fragment{
		File:     ".changelogs/bootstrap.md",
		Meta:     map[string]string{"component": "example-service", "bump": "patch", "type": "fix"},
		Body:     "Fix bootstrap debug flow.",
		BodyLine: "Fix bootstrap debug flow.",
	}

	if errors := ValidateReleaseSignal([]Fragment{fragment}, "fix(example-service): Fix bootstrap debug flow", ""); len(errors) != 0 {
		t.Fatalf("expected matching title, got %v", errors)
	}
	if errors := ValidateReleaseSignal([]Fragment{fragment}, "fix: something else", ""); len(errors) == 0 {
		t.Fatal("expected title mismatch")
	}
}

func TestReleasePRInfo(t *testing.T) {
	number, head, err := ReleasePRInfo(`[{"number":123,"headBranchName":"release-please--branches--main--components--example-service"}]`)
	if err != nil {
		t.Fatal(err)
	}
	if number != "123" || head != "release-please--branches--main--components--example-service" {
		t.Fatalf("unexpected release PR info: %s %s", number, head)
	}
}

func TestReleaseInfoCommand(t *testing.T) {
	var stdout bytes.Buffer

	err := Run([]string{"release-info", "--prs-json", `[{"number":123,"headBranchName":"release-branch"}]`}, nil, &stdout, &stdout)
	if err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); !bytes.Contains([]byte(got), []byte("number=123\nhead_ref=release-branch\n")) {
		t.Fatalf("unexpected release-info output: %s", got)
	}
}

func TestInitWritesReadme(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".changelogs")
	var stdout bytes.Buffer

	if err := Run([]string{"init", "--component", "example-service", "--dir", dir}, nil, &stdout, &stdout); err != nil {
		t.Fatal(err)
	}

	readme, err := os.ReadFile(filepath.Join(dir, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(readme, []byte("changelogger\n")) {
		t.Fatalf("README did not include default changelogger command:\n%s", readme)
	}
	if bytes.Contains(readme, []byte("changelogger new --component example-service")) {
		t.Fatalf("README still documented repeated component command:\n%s", readme)
	}
	if bytes.Contains(readme, []byte("three-word random slug")) {
		t.Fatalf("README exposed implementation-specific slug language:\n%s", readme)
	}
	if bytes.Contains(readme, []byte("Release Please")) || bytes.Contains(readme, []byte("GoReleaser")) {
		t.Fatalf("README exposed release-tool-specific language:\n%s", readme)
	}

	configData, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config Config
	if err := json.Unmarshal(configData, &config); err != nil {
		t.Fatal(err)
	}
	component, err := config.Component.Resolve(filepath.Dir(dir))
	if err != nil {
		t.Fatal(err)
	}
	if component != "example-service" {
		t.Fatalf("unexpected component: %s", component)
	}
}

func TestNewUsesThreeWordSlug(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".changelogs")
	input := bytes.NewBufferString("patch\nfix\nFix bootstrap debug flow.\n\n")
	var stdout bytes.Buffer

	if err := Init(dir, "example-service"); err != nil {
		t.Fatal(err)
	}

	if err := Run([]string{"new", "--dir", dir}, input, &stdout, &stdout); err != nil {
		t.Fatal(err)
	}

	files, err := FragmentFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one fragment, got %d", len(files))
	}
	name := filepath.Base(files[0])
	if !regexp.MustCompile(`^[a-z]+-[a-z]+-[a-z]+\.md$`).MatchString(name) {
		t.Fatalf("expected three-word slug filename, got %s", name)
	}
}

func TestRunWithoutCommandCreatesFragment(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".changelogs")
	input := bytes.NewBufferString("patch\nfix\nFix bootstrap debug flow.\n\n")
	var stdout bytes.Buffer

	if err := Init(dir, "example-service"); err != nil {
		t.Fatal(err)
	}

	if err := Run([]string{"--dir", dir}, input, &stdout, &stdout); err != nil {
		t.Fatal(err)
	}

	files, err := FragmentFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one fragment, got %d", len(files))
	}
}

func TestBareRunCreatesFragment(t *testing.T) {
	repo := t.TempDir()
	dir := filepath.Join(repo, ".changelogs")
	input := bytes.NewBufferString("patch\nfix\nFix bootstrap debug flow.\n\n")
	var stdout bytes.Buffer

	if err := Init(dir, "example-service"); err != nil {
		t.Fatal(err)
	}

	current, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(current); err != nil {
			t.Fatal(err)
		}
	}()

	if err := Run(nil, input, &stdout, &stdout); err != nil {
		t.Fatal(err)
	}

	files, err := FragmentFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("expected one fragment, got %d", len(files))
	}
}

func TestResolveComponentAllowsFlagOverride(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".changelogs")
	if err := Init(dir, "example-service"); err != nil {
		t.Fatal(err)
	}

	component, err := ResolveComponent(dir, "other", "")
	if err != nil {
		t.Fatal(err)
	}
	if component != "other" {
		t.Fatalf("expected flag component override, got %s", component)
	}
}

func TestRepositoryNameFromRemote(t *testing.T) {
	cases := map[string]string{
		"https://github.com/adrianmross/changelogger.git": "changelogger",
		"git@github.com:adrianmross/changelogger.git":     "changelogger",
		"ssh://git@github.com/adrianmross/changelogger":   "changelogger",
		"https://github.com/adrianmross/changelogger/":    "changelogger",
	}
	for remote, want := range cases {
		if got := RepositoryNameFromRemote(remote); got != want {
			t.Fatalf("RepositoryNameFromRemote(%q) = %q, want %q", remote, got, want)
		}
	}
}

func TestInitInfersComponentFromGitRemote(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "remote", "add", "origin", "git@github.com:adrianmross/inferred-name.git")

	dir := filepath.Join(repo, ".changelogs")
	var stdout bytes.Buffer
	current, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(current); err != nil {
			t.Fatal(err)
		}
	}()

	if err := Run([]string{"init", "--dir", dir}, nil, &stdout, &stdout); err != nil {
		t.Fatal(err)
	}

	configData, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config Config
	if err := json.Unmarshal(configData, &config); err != nil {
		t.Fatal(err)
	}
	component, err := config.Component.Resolve(repo)
	if err != nil {
		t.Fatal(err)
	}
	if component != "inferred-name" {
		t.Fatalf("unexpected inferred component: %s", component)
	}
}

func TestInitUsesPackageNameSource(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "package.json"), []byte(`{"name":"example-service"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(repo, ".changelogs")
	var stdout bytes.Buffer
	current, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(current); err != nil {
			t.Fatal(err)
		}
	}()

	if err := Run([]string{"init", "--dir", dir}, nil, &stdout, &stdout); err != nil {
		t.Fatal(err)
	}

	configData, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config Config
	if err := json.Unmarshal(configData, &config); err != nil {
		t.Fatal(err)
	}
	if config.Component.Source != "package.json" || config.Component.JSONPath != "$.name" {
		t.Fatalf("unexpected component source: %#v", config.Component)
	}
	component, err := ResolveComponent(dir, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if component != "example-service" {
		t.Fatalf("unexpected resolved component: %s", component)
	}
}

func TestResolveComponentUsesPackageConfig(t *testing.T) {
	repo := t.TempDir()
	serviceDir := filepath.Join(repo, "services", "api")
	if err := os.MkdirAll(serviceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(serviceDir, "package.json"), []byte(`{"name":"service-component"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(repo, ".changelogs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	config := []byte(`{
  "packages": {
    "api": {
      "path": "services/api",
      "component": {
        "source": "package.json",
        "jsonPath": "$.name"
      }
    }
  }
}
`)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), config, 0o644); err != nil {
		t.Fatal(err)
	}

	component, err := ResolveComponent(dir, "", "api")
	if err != nil {
		t.Fatal(err)
	}
	if component != "service-component" {
		t.Fatalf("unexpected package component: %s", component)
	}
}

func TestResolveComponentDefaultsPackageComponentToPackageKey(t *testing.T) {
	repo := t.TempDir()
	dir := filepath.Join(repo, ".changelogs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	config := []byte(`{
  "packages": {
    "api": {
      "path": "services/api"
    }
  }
}
`)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), config, 0o644); err != nil {
		t.Fatal(err)
	}

	component, err := ResolveComponent(dir, "", "api")
	if err != nil {
		t.Fatal(err)
	}
	if component != "api" {
		t.Fatalf("unexpected package component: %s", component)
	}
}

func TestResolveComponentDefaultsPathLikePackageKeyToBasename(t *testing.T) {
	repo := t.TempDir()
	dir := filepath.Join(repo, ".changelogs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	config := []byte(`{
  "packages": {
    "services/api": {}
  }
}
`)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), config, 0o644); err != nil {
		t.Fatal(err)
	}

	component, err := ResolveComponent(dir, "", "services/api")
	if err != nil {
		t.Fatal(err)
	}
	if component != "api" {
		t.Fatalf("unexpected package component: %s", component)
	}
}

func TestPackageConfigDefaultsComponentToPathBasename(t *testing.T) {
	component, err := (PackageConfig{Path: "services/api"}).Resolve(".", "")
	if err != nil {
		t.Fatal(err)
	}
	if component != "api" {
		t.Fatalf("unexpected package component: %s", component)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, output)
	}
}
