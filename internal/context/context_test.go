package context

import (
	"errors"
	"strings"
	"testing"
)

func TestRepoIDFromRemoteStripsCredentialsAndHashes(t *testing.T) {
	httpsID, err := RepoIDFromRemote("https://alice:secret-token@github.com/org/repo.git?x=1")
	if err != nil {
		t.Fatal(err)
	}
	sshID, err := RepoIDFromRemote("git@github.com:org/repo.git")
	if err != nil {
		t.Fatal(err)
	}
	if httpsID != sshID {
		t.Fatalf("equivalent remotes produced different IDs: %s %s", httpsID, sshID)
	}
	for _, forbidden := range []string{"github", "alice", "secret", "org/"} {
		if strings.Contains(httpsID, forbidden) {
			t.Fatalf("repo id leaked %q in %q", forbidden, httpsID)
		}
	}
	if !strings.HasPrefix(httpsID, "repo-") {
		t.Fatalf("repo id = %q", httpsID)
	}
}

func TestResolverOutsideGitReturnsNone(t *testing.T) {
	ctx, err := NewResolver(t.TempDir()).Current()
	if err != nil {
		t.Fatal(err)
	}
	if ctx.RepoID != RepoNone {
		t.Fatalf("repo id = %q", ctx.RepoID)
	}
}

func TestRepoIDFromRemoteIncludesNonDefaultPorts(t *testing.T) {
	first, err := RepoIDFromRemote("ssh://git@example.internal:2222/org/repo.git")
	if err != nil {
		t.Fatal(err)
	}
	second, err := RepoIDFromRemote("ssh://git@example.internal:7999/org/repo.git")
	if err != nil {
		t.Fatal(err)
	}
	defaultSSH, err := RepoIDFromRemote("ssh://git@example.internal:22/org/repo.git")
	if err != nil {
		t.Fatal(err)
	}
	noPort, err := RepoIDFromRemote("ssh://git@example.internal/org/repo.git")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("distinct ports collided: %s", first)
	}
	if defaultSSH != noPort {
		t.Fatalf("default port should canonicalize away: %s %s", defaultSSH, noPort)
	}
}

func TestRepoIDFromLocalRemotesIsDistinctAndSanitized(t *testing.T) {
	first, err := RepoIDFromRemote("file:///srv/repo-one.git")
	if err != nil {
		t.Fatal(err)
	}
	second, err := RepoIDFromRemote("/srv/repo-two.git")
	if err != nil {
		t.Fatal(err)
	}
	relative, err := RepoIDFromRemote("../repo-three.git")
	if err != nil {
		t.Fatal(err)
	}
	if first == second || first == relative || second == relative {
		t.Fatalf("local remotes collided: %s %s %s", first, second, relative)
	}
	for _, id := range []string{first, second, relative} {
		if !strings.HasPrefix(id, "repo-") || strings.Contains(id, "srv") || strings.Contains(id, "repo-one") {
			t.Fatalf("local repo id leaked path or wrong prefix: %s", id)
		}
	}
}

func TestRepoIDFromRemoteKeepsIPv6PortsUnambiguous(t *testing.T) {
	withPort, err := RepoIDFromRemote("ssh://git@[2001:db8::1]:2222/org/repo.git")
	if err != nil {
		t.Fatal(err)
	}
	hostLiteral, err := RepoIDFromRemote("ssh://git@[2001:db8::1:2222]/org/repo.git")
	if err != nil {
		t.Fatal(err)
	}
	if withPort == hostLiteral {
		t.Fatalf("ipv6 host-port canonicalization collided: %s", withPort)
	}
}

func TestGitFailureClassification(t *testing.T) {
	if !isExpectedNotGit(errors.New("git rev-parse --show-toplevel: exit status 128: fatal: not a git repository")) {
		t.Fatal("expected not-git error to be classified")
	}
	if isExpectedNotGit(errors.New("git rev-parse --show-toplevel: exit status 128: fatal: detected dubious ownership")) {
		t.Fatal("safe-directory failure should not be classified as not-git")
	}
	if !isExpectedNoRemote(errors.New("git remote get-url origin: exit status 2: error: No such remote 'origin'")) {
		t.Fatal("expected no-remote error to be classified")
	}
	if isExpectedNoRemote(errors.New("git remote get-url origin: exit status 128: fatal: bad config")) {
		t.Fatal("config failure should not be classified as no-remote")
	}
}
