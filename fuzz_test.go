package gitgrab

// Security fuzz tests for gitgrab.
//
// These targets fuzz the untrusted-input boundaries of the tool: the clone
// method flag, GitHub-supplied repository/organization names and URLs, the
// path that a repository name is resolved to, the clone URL handed to git, and
// the JSON decoding of GitHub API responses. Each target asserts a security
// *invariant* (path containment, no argument injection, no command splitting)
// rather than a fixed expected value, so they detect regressions that would
// re-open a traversal or injection hole.
//
// Two ways to run them:
//
//   Lightweight (CI): the seed corpora below run as ordinary tests on every
//   `go test`/`task test`, and `task fuzz-ci` adds a short, time-bounded fuzz
//   burst per target. Fast and deterministic.
//
//   Heavyweight (ad hoc, on a developer machine): `task fuzz` runs each target
//   for a long, configurable duration, e.g. `task fuzz FUZZTIME=5m`.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

// fixedTarget is an arbitrary, safe target directory used where the test needs
// a stable base for path resolution. resolveRepoPath does not touch the
// filesystem, so this need not exist.
const fixedTarget = "/tmp/gitgrab-target"

// hasControlOrSpace reports whether s contains any control character,
// whitespace, or the Unicode replacement character (which marks invalid UTF-8).
func hasControlOrSpace(s string) bool {
	for _, r := range s {
		if r == unicode.ReplacementChar || unicode.IsControl(r) || unicode.IsSpace(r) {
			return true
		}
	}
	return false
}

// FuzzParseCloneMethod checks that parsing the untrusted -m flag never panics
// and always yields a known clone method, defaulting to SSH on error.
func FuzzParseCloneMethod(f *testing.F) {
	seeds := []string{
		"ssh", "http", "SSH", "HTTP", "Ssh", "hTtP",
		"", " ", "ftp", "git", "https", "ssh ", " http",
		"ssh\n", "http;rm -rf /", "-o", "--config", "$(id)",
		"ssh\x00http", "SSH\t", strings.Repeat("s", 4096),
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		m, err := ParseCloneMethod(s)

		// The returned method must always be one of the known values.
		if m != CloneMethodSSH && m != CloneMethodHTTP {
			t.Fatalf("ParseCloneMethod(%q) returned unknown method %d", s, m)
		}
		if got := m.String(); got != "ssh" && got != "http" {
			t.Fatalf("ParseCloneMethod(%q).String() = %q, want ssh|http", s, got)
		}

		low := strings.ToLower(s)
		if err == nil && low != "ssh" && low != "http" {
			t.Fatalf("ParseCloneMethod(%q) returned nil error for invalid input", s)
		}
		if err != nil && m != CloneMethodSSH {
			t.Fatalf("ParseCloneMethod(%q) errored but did not default to ssh", s)
		}
	})
}

// FuzzRepositoryNameValidation asserts that any repository name accepted by
// RepositoryName.IsValid is genuinely safe: no path separators, not "." or
// "..", no leading dash (flag injection), no control/whitespace characters, and
// that it resolves to a path contained directly within the target directory.
func FuzzRepositoryNameValidation(f *testing.F) {
	seeds := []string{
		"repo", "my-repo", "my_repo", "repo.git", "a", "R2-D2",
		"", ".", "..", "...", "-repo", "repo-",
		"../../etc/passwd", "..\\..\\windows", "a/b", "a\\b",
		"foo bar", "foo\tbar", "repo\n", "repo\x00", "$(whoami)",
		"`id`", "repo;rm -rf /", "--upload-pack=x", "-o", "café",
		strings.Repeat("a", 8192),
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, name string) {
		r := RepositoryName(name)
		if !r.IsValid() {
			return
		}

		if strings.ContainsAny(name, `/\`) {
			t.Fatalf("valid repo name %q contains a path separator", name)
		}
		if name == "." || name == ".." {
			t.Fatalf("valid repo name %q is a traversal component", name)
		}
		if strings.HasPrefix(name, "-") {
			t.Fatalf("valid repo name %q starts with '-' (flag injection risk)", name)
		}
		if hasControlOrSpace(name) {
			t.Fatalf("valid repo name %q contains control/whitespace", name)
		}

		// A valid name must resolve to a contained path.
		p, err := resolveRepoPath(fixedTarget, r)
		if err != nil {
			t.Fatalf("valid repo name %q failed to resolve: %v", name, err)
		}
		if filepath.Dir(p) != filepath.Clean(fixedTarget) {
			t.Fatalf("valid repo name %q resolved outside target: %q", name, p)
		}
		if filepath.Base(p) != name {
			t.Fatalf("valid repo name %q resolved to base %q", name, filepath.Base(p))
		}
	})
}

// FuzzOrganizationNameValidation asserts that any organization name accepted by
// OrganizationName.IsValid is safe to interpolate into a clone URL: no
// separators, '@', ':' or spaces, no leading dash, no control characters, and
// that the resulting private-repo HTTP URL is well-formed and contains the org
// intact.
func FuzzOrganizationNameValidation(f *testing.F) {
	seeds := []string{
		"github", "my-org", "Octocat", "a", "org123",
		"", "-org", "org-", "my org", "my/org", "my\\org",
		"a@b", "a:b", "org\n", "org\x00", "$(id)", "café",
		strings.Repeat("o", 8192),
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, org string) {
		o := OrganizationName(org)
		if !o.IsValid() {
			return
		}

		if strings.ContainsAny(org, `/\@: `) {
			t.Fatalf("valid org name %q contains a URL metacharacter", org)
		}
		if strings.HasPrefix(org, "-") {
			t.Fatalf("valid org name %q starts with '-'", org)
		}
		if hasControlOrSpace(org) {
			t.Fatalf("valid org name %q contains control/whitespace", org)
		}

		// A valid org must produce a safe private-repo HTTP clone URL.
		cfg := CloneConfig{
			Repository:   Repository{Name: RepositoryName("repo"), Private: true},
			Organization: o,
			Token:        GitHubToken("ghp_safeToken"),
			Method:       CloneMethodHTTP,
		}
		u, err := buildCloneURL(cfg)
		if err != nil {
			t.Fatalf("valid org name %q failed URL build: %v", org, err)
		}
		if !isSafeURL(u) {
			t.Fatalf("valid org name %q produced unsafe URL %q", org, u)
		}
		if !strings.Contains(u, "/"+org+"/") {
			t.Fatalf("org %q not present intact in URL %q", org, u)
		}
	})
}

// FuzzResolveRepoPath is the core path-traversal guard. For ANY target and
// name, if resolveRepoPath succeeds the result must sit directly inside the
// cleaned target directory and must not escape it via "..".
func FuzzResolveRepoPath(f *testing.F) {
	seeds := []struct{ target, name string }{
		{"/tmp/x", "repo"},
		{"/tmp/x", "../escape"},
		{"/tmp/x", "../../etc/passwd"},
		{"/tmp/x", ".."},
		{"/tmp/x", "."},
		{"/tmp/x", "a/b"},
		{"/tmp/x", "a\\b"},
		{"", "repo"},
		{".", "repo"},
		{"relative/dir", "repo"},
		{"/tmp/x", "repo\x00/../../etc"},
		{"/a/b/c", "sub/../../../../root"},
	}
	for _, s := range seeds {
		f.Add(s.target, s.name)
	}

	f.Fuzz(func(t *testing.T, target, name string) {
		p, err := resolveRepoPath(target, RepositoryName(name))
		if err != nil {
			return
		}

		clean := filepath.Clean(target)
		if filepath.Dir(p) != clean {
			t.Fatalf("resolveRepoPath(%q, %q) = %q; parent %q != target %q",
				target, name, p, filepath.Dir(p), clean)
		}
		if filepath.Base(p) != name {
			t.Fatalf("resolveRepoPath(%q, %q) = %q; base %q != name",
				target, name, p, filepath.Base(p))
		}
		if rel, rerr := filepath.Rel(clean, p); rerr == nil {
			if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
				t.Fatalf("resolveRepoPath(%q, %q) escapes target: rel=%q",
					target, name, rel)
			}
		}
	})
}

// FuzzBuildCloneURL asserts that whatever combination of untrusted fields is
// supplied, any URL buildCloneURL returns is safe to pass to `git clone`:
// non-empty, no leading dash, and free of whitespace/control/NUL characters
// that could split or corrupt the command.
func FuzzBuildCloneURL(f *testing.F) {
	// Args: ssh, clone, name, org, token, private, useHTTP.
	seeds := []struct {
		ssh, clone, name, org, token string
		private, useHTTP             bool
	}{
		{"git@github.com:o/r.git", "https://github.com/o/r.git", "r", "o", "ghp_x", false, false},
		{"git@github.com:o/r.git", "https://github.com/o/r.git", "r", "o", "ghp_x", true, true},
		{"git@github.com:o/r.git", "https://github.com/o/r.git", "r", "o", "ghp_x", false, true},
		{"-oProxyCommand=id", "https://github.com/o/r.git", "r", "o", "ghp_x", false, false},
		{"git@github.com:o/r.git", "--upload-pack=id", "r", "o", "ghp_x", false, true},
		{"ext::sh -c id", "https://github.com/o/r.git", "r", "o", "ghp_x", false, false},
		{"git@github.com:o/r.git", "https://x/r.git", "r", "o", "tok en", true, true},
		{"git@github.com:o/r.git", "https://x/r.git", "r", "o", "tok\nen", true, true},
		{"git@ github.com", "https://git hub.com/r.git", "r", "o", "ghp_x", false, false},
		{"", "", "", "", "", true, true},
	}
	for _, s := range seeds {
		f.Add(s.ssh, s.clone, s.name, s.org, s.token, s.private, s.useHTTP)
	}

	f.Fuzz(func(t *testing.T, ssh, clone, name, org, token string, private, useHTTP bool) {
		method := CloneMethodSSH
		if useHTTP {
			method = CloneMethodHTTP
		}
		cfg := CloneConfig{
			Repository: Repository{
				Name:     RepositoryName(name),
				CloneURL: HTTPURL(clone),
				SSHURL:   SSHURL(ssh),
				Private:  private,
			},
			TargetDir:    fixedTarget,
			Token:        GitHubToken(token),
			Organization: OrganizationName(org),
			Method:       method,
		}

		u, err := buildCloneURL(cfg)
		if err != nil {
			return
		}
		if !isSafeURL(u) {
			t.Fatalf("buildCloneURL returned unsafe URL %q", u)
		}
		if strings.HasPrefix(u, "-") {
			t.Fatalf("buildCloneURL returned URL that git reads as an option: %q", u)
		}
		if strings.ContainsAny(u, " \t\r\n\x00") {
			t.Fatalf("buildCloneURL returned URL with command-splitting char: %q", u)
		}
	})
}

// FuzzParseRepos feeds arbitrary bytes to the GitHub API response decoder. It
// must never panic, and every repository it successfully decodes must, when run
// through the real sinks, either be rejected or produce a contained path and a
// safe clone URL. This ties the JSON boundary to the downstream safety checks.
func FuzzParseRepos(f *testing.F) {
	seeds := []string{
		`[]`,
		`[{"name":"repo","clone_url":"https://github.com/o/repo.git","ssh_url":"git@github.com:o/repo.git","private":false,"default_branch":"main"}]`,
		`[{"name":"../../etc/passwd","ssh_url":"git@github.com:o/x.git"}]`,
		`[{"name":"-oProxyCommand=id"}]`,
		`[{"name":"repo","ssh_url":"ext::sh -c id"}]`,
		`[{"name":"repo","ssh_url":"git@github.com:o/r.git\n--config=x"}]`,
		`[{"name":"a","private":true},{"name":"b"}]`,
		`{"not":"an array"}`,
		`null`,
		`[123, "string", null]`,
		`[{"name":`,
		`[] trailing garbage`,
		strings.Repeat("[", 4096),
		`[{"name":"` + strings.Repeat("a", 65536) + `"}]`,
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		repos, err := parseRepos(data)
		if err != nil {
			return
		}

		for _, repo := range repos {
			// Path sink: never escapes the target when it succeeds.
			if p, perr := resolveRepoPath(fixedTarget, repo.Name); perr == nil {
				if filepath.Dir(p) != filepath.Clean(fixedTarget) {
					t.Fatalf("decoded repo %q resolved outside target: %q", repo.Name, p)
				}
				if filepath.Base(p) != repo.Name.String() {
					t.Fatalf("decoded repo %q resolved to base %q", repo.Name, filepath.Base(p))
				}
			}

			// URL sink: never returns an injectable URL for any method.
			for _, m := range []CloneMethod{CloneMethodSSH, CloneMethodHTTP} {
				cfg := CloneConfig{
					Repository:   repo,
					TargetDir:    fixedTarget,
					Token:        GitHubToken("ghp_safeToken"),
					Organization: OrganizationName("safeorg"),
					Method:       m,
				}
				if u, uerr := buildCloneURL(cfg); uerr == nil && !isSafeURL(u) {
					t.Fatalf("decoded repo %q produced unsafe URL %q (method %s)", repo.Name, u, m)
				}
			}
		}
	})
}
