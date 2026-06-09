package changelogger

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const defaultDir = ".changelogs"

var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

var (
	validBumps = map[string]bool{"patch": true, "minor": true, "major": true}
	validTypes = map[string]bool{"fix": true, "feat": true, "deps": true}
	semverRE   = regexp.MustCompile(`^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$`)
)

type Fragment struct {
	File     string
	Meta     map[string]string
	Body     string
	BodyLine string
}

type Options struct {
	Dir          string
	Component    string
	Base         string
	PR           bool
	PRTitle      string
	PRBody       string
	VersionFile  string
	ManifestFile string
	Remote       string
	PRSJSON      string
}

func Run(args []string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
	if len(args) == 0 {
		usage(stderr)
		return errors.New("missing command")
	}

	switch args[0] {
	case "new":
		return runNew(args[1:], stdin, stdout)
	case "check":
		return runCheck(args[1:], stdout)
	case "consume":
		return runConsume(args[1:], stdout)
	case "release-pr-info":
		return runReleasePRInfo(args[1:], stdout)
	case "release-tag":
		return runReleaseTag(args[1:], stdout)
	case "version":
		fmt.Fprintf(stdout, "changelogger %s (%s, %s)\n", Version, Commit, Date)
		return nil
	default:
		usage(stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func usage(w io.Writer) {
	fmt.Fprintln(w, `Usage:
  changelogger new --component <name>
  changelogger check --component <name> [--base <ref>] [--pr] [--pr-title <title>] [--pr-body <body>]
  changelogger consume
  changelogger release-pr-info --prs-json <json>
  changelogger release-tag --component <name> --version-file .ochain.json --manifest-file .release-please-manifest.json`)
}

func runNew(args []string, stdin io.Reader, stdout io.Writer) error {
	fs := flag.NewFlagSet("new", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	component := fs.String("component", "", "component name")
	dir := fs.String("dir", defaultDir, "fragment directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *component == "" {
		return errors.New("--component is required")
	}

	reader := bufio.NewReader(stdin)
	bump, err := ask(reader, stdout, "Bump (patch/minor/major): ")
	if err != nil {
		return err
	}
	if !validBumps[bump] {
		return errors.New("bump must be patch, minor, or major")
	}
	defaultType := "fix"
	if bump == "minor" || bump == "major" {
		defaultType = "feat"
	}
	changeType, err := ask(reader, stdout, fmt.Sprintf("Release Please type (fix/feat/deps) [%s]: ", defaultType))
	if err != nil {
		return err
	}
	if changeType == "" {
		changeType = defaultType
	}
	if !validTypes[changeType] {
		return errors.New("type must be fix, feat, or deps")
	}
	summary, err := ask(reader, stdout, "One-line release note: ")
	if err != nil {
		return err
	}
	if strings.TrimSpace(summary) == "" {
		return errors.New("release note cannot be empty")
	}
	version, err := ask(reader, stdout, "Explicit version (optional): ")
	if err != nil {
		return err
	}
	if version != "" && !semverRE.MatchString(version) {
		return errors.New("version must be semantic version syntax")
	}

	if err := os.MkdirAll(*dir, 0o755); err != nil {
		return err
	}
	hash := sha1.Sum([]byte(fmt.Sprintf("%d:%s", time.Now().UnixNano(), summary)))
	name := fmt.Sprintf("%s-%s.md", slug(summary), hex.EncodeToString(hash[:])[:8])
	path := filepath.Join(*dir, name)
	lines := []string{
		"---",
		"component: " + *component,
		"bump: " + bump,
		"type: " + changeType,
	}
	if version != "" {
		lines = append(lines, "version: "+version)
	}
	lines = append(lines, "---", "", summary, "")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return err
	}

	fragment, err := ParseFragment(path)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Wrote %s\n", path)
	fmt.Fprintf(stdout, "Use this PR title so Release Please sees the release intent:\n%s\n", ReleasePleaseSubject(fragment))
	if version != "" {
		fmt.Fprintf(stdout, "Add this to the PR body:\nRelease-As: %s\n", version)
	}
	return nil
}

func runCheck(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	component := fs.String("component", "", "component name")
	dir := fs.String("dir", defaultDir, "fragment directory")
	base := fs.String("base", "", "base git ref")
	pr := fs.Bool("pr", false, "pull request mode")
	prTitle := fs.String("pr-title", os.Getenv("CHANGELOGGER_PR_TITLE"), "pull request title")
	prBody := fs.String("pr-body", os.Getenv("CHANGELOGGER_PR_BODY"), "pull request body")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *component == "" {
		return errors.New("--component is required")
	}

	fragments, err := LoadFragments(*dir)
	if err != nil {
		return err
	}
	var allErrors []string
	for _, fragment := range fragments {
		allErrors = append(allErrors, ValidateFragment(fragment, *component)...)
	}
	changed := SignificantChangedFiles(*base)
	if *pr && len(changed) > 0 && len(fragments) == 0 && !isReleasePRTitle(*prTitle, *component) {
		allErrors = append(allErrors, fmt.Sprintf("add a %s/<slug>.md release note fragment", *dir))
	}
	if *pr && len(fragments) > 0 {
		allErrors = append(allErrors, ValidateReleaseSignal(fragments, *prTitle, *prBody)...)
	}
	if len(allErrors) > 0 {
		return errors.New(strings.Join(allErrors, "\n"))
	}
	if len(changed) > 0 && len(fragments) > 0 {
		fmt.Fprintf(stdout, "Validated %d changelog fragment(s) for %d significant changed file(s).\n", len(fragments), len(changed))
	} else {
		fmt.Fprintf(stdout, "Validated %d changelog fragment(s).\n", len(fragments))
	}
	return nil
}

func runConsume(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("consume", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dir := fs.String("dir", defaultDir, "fragment directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	files, err := FragmentFiles(*dir)
	if err != nil {
		return err
	}
	for _, file := range files {
		if err := os.Remove(file); err != nil {
			return err
		}
	}
	fmt.Fprintf(stdout, "Consumed %d changelog fragment(s).\n", len(files))
	return nil
}

func runReleasePRInfo(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("release-pr-info", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	prsJSON := fs.String("prs-json", os.Getenv("CHANGELOGGER_RELEASE_PLEASE_PRS"), "Release Please prs output JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	number, head, err := ReleasePRInfo(*prsJSON)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "number=%s\nhead_ref=%s\n", number, head)
	return nil
}

func runReleaseTag(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("release-tag", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	component := fs.String("component", "", "component name")
	dir := fs.String("dir", defaultDir, "fragment directory")
	versionFile := fs.String("version-file", ".ochain.json", "JSON version file")
	manifestFile := fs.String("manifest-file", ".release-please-manifest.json", "Release Please manifest")
	remote := fs.String("remote", "origin", "git remote")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *component == "" {
		return errors.New("--component is required")
	}

	result, err := ReleaseTagDecision(Options{
		Dir:          *dir,
		Component:    *component,
		VersionFile:  *versionFile,
		ManifestFile: *manifestFile,
		Remote:       *remote,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "should_tag=%t\ntag=%s\nreason=%s\n", result.ShouldTag, result.Tag, result.Reason)
	return nil
}

func ask(reader *bufio.Reader, stdout io.Writer, prompt string) (string, error) {
	fmt.Fprint(stdout, prompt)
	value, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func slug(value string) string {
	lowered := strings.ToLower(value)
	re := regexp.MustCompile(`[^a-z0-9]+`)
	slug := strings.Trim(re.ReplaceAllString(lowered, "-"), "-")
	if slug == "" {
		return "change"
	}
	if len(slug) > 64 {
		return strings.Trim(slug[:64], "-")
	}
	return slug
}

func FragmentFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") || entry.Name() == "README.md" {
			continue
		}
		files = append(files, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(files)
	return files, nil
}

func LoadFragments(dir string) ([]Fragment, error) {
	files, err := FragmentFiles(dir)
	if err != nil {
		return nil, err
	}
	fragments := make([]Fragment, 0, len(files))
	for _, file := range files {
		fragment, err := ParseFragment(file)
		if err != nil {
			return nil, err
		}
		fragments = append(fragments, fragment)
	}
	return fragments, nil
}

func ParseFragment(file string) (Fragment, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return Fragment{}, err
	}
	raw := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(raw, "---\n") {
		return Fragment{}, fmt.Errorf("%s: expected YAML frontmatter delimited by ---", file)
	}
	rest := strings.TrimPrefix(raw, "---\n")
	parts := strings.SplitN(rest, "\n---\n", 2)
	if len(parts) != 2 {
		return Fragment{}, fmt.Errorf("%s: expected YAML frontmatter delimited by ---", file)
	}
	meta := map[string]string{}
	for _, line := range strings.Split(parts[0], "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return Fragment{}, fmt.Errorf("%s: invalid frontmatter line %q", file, line)
		}
		meta[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	body := strings.TrimSpace(parts[1])
	line := body
	if idx := strings.Index(line, "\n"); idx >= 0 {
		line = line[:idx]
	}
	return Fragment{File: file, Meta: meta, Body: body, BodyLine: strings.TrimSpace(line)}, nil
}

func ValidateFragment(fragment Fragment, component string) []string {
	var errors []string
	if fragment.Meta["component"] != component {
		errors = append(errors, fmt.Sprintf("%s: component must be %s", fragment.File, component))
	}
	if !validBumps[fragment.Meta["bump"]] {
		errors = append(errors, fmt.Sprintf("%s: bump must be patch, minor, or major", fragment.File))
	}
	if !validTypes[fragment.Meta["type"]] {
		errors = append(errors, fmt.Sprintf("%s: type must be fix, feat, or deps", fragment.File))
	}
	if fragment.Body == "" {
		errors = append(errors, fmt.Sprintf("%s: body must describe the user-visible change", fragment.File))
	}
	if (fragment.Meta["bump"] == "minor" || fragment.Meta["bump"] == "major") && fragment.Meta["type"] != "feat" {
		errors = append(errors, fmt.Sprintf("%s: minor and major bumps must use type: feat", fragment.File))
	}
	if version := fragment.Meta["version"]; version != "" && !semverRE.MatchString(version) {
		errors = append(errors, fmt.Sprintf("%s: version must be semantic version syntax", fragment.File))
	}
	return errors
}

func ReleasePleaseSubject(fragment Fragment) string {
	bang := ""
	if fragment.Meta["bump"] == "major" {
		bang = "!"
	}
	summary := strings.TrimRight(fragment.BodyLine, ".!?")
	return fmt.Sprintf("%s%s(%s): %s", fragment.Meta["type"], bang, fragment.Meta["component"], summary)
}

func ValidateReleaseSignal(fragments []Fragment, title string, body string) []string {
	var subjects []string
	for _, fragment := range fragments {
		subjects = append(subjects, ReleasePleaseSubject(fragment))
	}
	title = strings.TrimSpace(title)
	matched := false
	for _, subject := range subjects {
		if title == subject {
			matched = true
			break
		}
	}
	var errors []string
	if !matched {
		errors = append(errors, "PR title must match one changelog fragment's Release Please subject:\n  - "+strings.Join(subjects, "\n  - "))
	}
	for _, fragment := range fragments {
		version := fragment.Meta["version"]
		if version == "" {
			continue
		}
		pattern := regexp.MustCompile(`(?i)Release-As:\s*` + regexp.QuoteMeta(version))
		if !pattern.MatchString(body) {
			errors = append(errors, fmt.Sprintf("%s: PR body must include 'Release-As: %s' for an explicit version request", fragment.File, version))
		}
	}
	return errors
}

func isReleasePRTitle(title string, component string) bool {
	pattern := regexp.MustCompile(`^chore\(release\): ` + regexp.QuoteMeta(component) + ` v\d+\.\d+\.\d+`)
	return pattern.MatchString(strings.TrimSpace(title))
}

func SignificantChangedFiles(base string) []string {
	if base == "" {
		return nil
	}
	mergeBase := gitOutput("merge-base", base, "HEAD")
	if mergeBase == "" {
		return nil
	}
	output := gitOutput("diff", "--name-only", mergeBase+"...HEAD")
	if output == "" {
		return nil
	}
	var changed []string
	for _, file := range strings.Split(output, "\n") {
		if file == "" || ignoredChange(file) {
			continue
		}
		changed = append(changed, file)
	}
	return changed
}

func ignoredChange(file string) bool {
	if strings.HasPrefix(file, ".changelogs/") || strings.HasPrefix(file, ".github/") || strings.HasPrefix(file, "docs/") {
		return true
	}
	switch file {
	case "README.md", "CHANGELOG.md", "AGENTS.md", "release-please-config.json", ".release-please-manifest.json":
		return true
	}
	return strings.HasSuffix(file, ".md")
}

func ReleasePRInfo(raw string) (string, string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", "", errors.New("Release Please did not report a release PR")
	}
	var payload any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return "", "", err
	}
	item := payload
	if list, ok := payload.([]any); ok {
		if len(list) == 0 {
			return "", "", errors.New("Release Please did not report a release PR")
		}
		item = list[0]
	}
	object, ok := item.(map[string]any)
	if !ok {
		return "", "", fmt.Errorf("unexpected Release Please PR payload: %T", item)
	}
	number := stringField(object, "number", "pullRequestNumber")
	head := stringField(object, "headBranchName", "headRefName", "headBranch", "branchName")
	if number == "" || head == "" {
		return "", "", fmt.Errorf("could not determine release PR number/head from: %s", raw)
	}
	return number, head, nil
}

func stringField(object map[string]any, names ...string) string {
	for _, name := range names {
		value, ok := object[name]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			return typed
		case float64:
			return fmt.Sprintf("%.0f", typed)
		}
	}
	return ""
}

type TagDecision struct {
	ShouldTag bool
	Tag       string
	Reason    string
}

func ReleaseTagDecision(options Options) (TagDecision, error) {
	version, err := jsonString(options.VersionFile, "version")
	if err != nil {
		return TagDecision{}, err
	}
	if !semverRE.MatchString(version) {
		return TagDecision{}, fmt.Errorf("invalid version in %s: %s", options.VersionFile, version)
	}
	manifestVersion, _ := jsonString(options.ManifestFile, ".")
	tag := "v" + version
	subject := gitOutput("log", "-1", "--pretty=%s")
	changed := map[string]bool{}
	for _, file := range changedFilesInHead() {
		changed[file] = true
	}
	releaseFilesChanged := changed[options.VersionFile] && changed[options.ManifestFile] && changed["CHANGELOG.md"]
	releaseCommit := subject == fmt.Sprintf("chore(release): %s v%s", options.Component, version) || releaseFilesChanged
	fragments, err := FragmentFiles(options.Dir)
	if err != nil {
		return TagDecision{}, err
	}
	tagExists := gitOutput("ls-remote", "--exit-code", "--tags", options.Remote, "refs/tags/"+tag) != ""
	switch {
	case tagExists:
		return TagDecision{Tag: tag, Reason: "tag-exists"}, nil
	case len(fragments) > 0:
		return TagDecision{Tag: tag, Reason: "pending-changelog-fragments"}, nil
	case manifestVersion != version:
		return TagDecision{Tag: tag, Reason: "release-manifest-version-mismatch"}, nil
	case !releaseCommit:
		return TagDecision{Tag: tag, Reason: "not-release-commit"}, nil
	default:
		return TagDecision{ShouldTag: true, Tag: tag, Reason: "release-commit"}, nil
	}
}

func changedFilesInHead() []string {
	parent := gitOutput("rev-parse", "HEAD^1")
	if parent == "" {
		return nil
	}
	output := gitOutput("diff", "--name-only", parent, "HEAD")
	if output == "" {
		return nil
	}
	return strings.Split(output, "\n")
}

func jsonString(file string, key string) (string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	var object map[string]any
	if err := json.Unmarshal(data, &object); err != nil {
		return "", err
	}
	value, _ := object[key].(string)
	return value, nil
}

func gitOutput(args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Stderr = io.Discard
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}
