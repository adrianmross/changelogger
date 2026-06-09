package changelogger

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestParseAndValidateFragment(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bootstrap.md")
	err := os.WriteFile(file, []byte("---\ncomponent: trqp_vdr_go\nbump: patch\ntype: fix\n---\n\nFix bootstrap debug flow.\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	fragment, err := ParseFragment(file)
	if err != nil {
		t.Fatal(err)
	}
	if errors := ValidateFragment(fragment, "trqp_vdr_go"); len(errors) != 0 {
		t.Fatalf("expected valid fragment, got %v", errors)
	}
	if got := ReleasePleaseSubject(fragment); got != "fix(trqp_vdr_go): Fix bootstrap debug flow" {
		t.Fatalf("unexpected subject: %s", got)
	}
}

func TestValidateReleaseSignalRequiresMatchingTitle(t *testing.T) {
	fragment := Fragment{
		File:     ".changelogs/bootstrap.md",
		Meta:     map[string]string{"component": "trqp_vdr_go", "bump": "patch", "type": "fix"},
		Body:     "Fix bootstrap debug flow.",
		BodyLine: "Fix bootstrap debug flow.",
	}

	if errors := ValidateReleaseSignal([]Fragment{fragment}, "fix(trqp_vdr_go): Fix bootstrap debug flow", ""); len(errors) != 0 {
		t.Fatalf("expected matching title, got %v", errors)
	}
	if errors := ValidateReleaseSignal([]Fragment{fragment}, "fix: something else", ""); len(errors) == 0 {
		t.Fatal("expected title mismatch")
	}
}

func TestReleasePRInfo(t *testing.T) {
	number, head, err := ReleasePRInfo(`[{"number":123,"headBranchName":"release-please--branches--main--components--trqp_vdr_go"}]`)
	if err != nil {
		t.Fatal(err)
	}
	if number != "123" || head != "release-please--branches--main--components--trqp_vdr_go" {
		t.Fatalf("unexpected release PR info: %s %s", number, head)
	}
}

func TestInitWritesReadme(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".changelogs")
	var stdout bytes.Buffer

	if err := Run([]string{"init", "--component", "trqp_vdr_go", "--dir", dir}, nil, &stdout, &stdout); err != nil {
		t.Fatal(err)
	}

	readme, err := os.ReadFile(filepath.Join(dir, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(readme, []byte("changelogger new --component trqp_vdr_go")) {
		t.Fatalf("README did not include component command:\n%s", readme)
	}
}

func TestNewUsesThreeWordSlug(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".changelogs")
	input := bytes.NewBufferString("patch\nfix\nFix bootstrap debug flow.\n\n")
	var stdout bytes.Buffer

	if err := Run([]string{"new", "--component", "trqp_vdr_go", "--dir", dir}, input, &stdout, &stdout); err != nil {
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
