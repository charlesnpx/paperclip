package context

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/charlesnpx/paperclip/internal/domain"
)

const (
	RepoNone          = "none"
	RepoLocalNoRemote = "git-local:no-remote"
)

type Resolver struct {
	dir string
}

func NewResolver(dir string) Resolver {
	return Resolver{dir: dir}
}

func (r Resolver) Current() (domain.Context, error) {
	dir := r.dir
	if dir == "" {
		dir = "."
	}
	if _, err := gitOutput(dir, "rev-parse", "--show-toplevel"); err != nil {
		if !isExpectedNotGit(err) {
			return domain.Context{}, err
		}
		return domain.Context{RepoID: RepoNone}, nil
	}
	remote, err := gitOutput(dir, "remote", "get-url", "origin")
	if err != nil {
		if !isExpectedNoRemote(err) {
			return domain.Context{}, err
		}
		return domain.Context{RepoID: RepoLocalNoRemote}, nil
	}
	if strings.TrimSpace(string(remote)) == "" {
		return domain.Context{RepoID: RepoLocalNoRemote}, nil
	}
	id, err := RepoIDFromRemote(strings.TrimSpace(string(remote)))
	if err != nil {
		return domain.Context{RepoID: RepoLocalNoRemote}, nil
	}
	return domain.Context{RepoID: id}, nil
}

func RepoIDFromRemote(remote string) (string, error) {
	canonical, err := canonicalRemote(remote)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(canonical))
	return "repo-" + hex.EncodeToString(sum[:8]), nil
}

func canonicalRemote(remote string) (string, error) {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return "", errors.New("remote is empty")
	}
	if strings.Contains(remote, "://") {
		u, err := url.Parse(remote)
		if err != nil {
			return "", err
		}
		if u.Scheme == "file" {
			if u.Path == "" {
				return "", errors.New("file remote missing path")
			}
			return stripGitSuffix("local/" + path.Clean(u.Path)), nil
		}
		if u.Host == "" || u.Path == "" {
			return "", errors.New("remote url missing host or path")
		}
		host := canonicalHostPort(u)
		p := strings.TrimPrefix(path.Clean(u.EscapedPath()), "/")
		p, _ = url.PathUnescape(p)
		return stripGitSuffix(host + "/" + p), nil
	}
	if match := scpLikeRemote.FindStringSubmatch(remote); match != nil {
		host := strings.ToLower(match[1])
		p := strings.TrimPrefix(filepath.ToSlash(match[2]), "/")
		return stripGitSuffix(host + "/" + path.Clean(p)), nil
	}
	if strings.HasPrefix(remote, "/") || strings.HasPrefix(remote, "./") || strings.HasPrefix(remote, "../") {
		cleaned := path.Clean(filepath.ToSlash(remote))
		if cleaned == "." || cleaned == "" {
			return "", errors.New("local remote missing path")
		}
		return stripGitSuffix("local/" + cleaned), nil
	}
	return "", errors.New("unsupported remote format")
}

var scpLikeRemote = regexp.MustCompile(`^(?:[^@/:]+@)?([^:]+):(.+)$`)

func stripGitSuffix(value string) string {
	value = strings.TrimSuffix(value, "/")
	value = strings.TrimSuffix(value, ".git")
	return value
}

func canonicalHostPort(u *url.URL) string {
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if port == "" {
		return host
	}
	if defaultPort(u.Scheme) == port {
		return host
	}
	return net.JoinHostPort(host, port)
}

func defaultPort(scheme string) string {
	switch strings.ToLower(scheme) {
	case "http":
		return "80"
	case "https":
		return "443"
	case "ssh":
		return "22"
	default:
		return ""
	}
}

func gitOutput(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func isExpectedNotGit(err error) bool {
	message := err.Error()
	return strings.Contains(message, "not a git repository") ||
		strings.Contains(message, "not a git repo") ||
		strings.Contains(message, "not in a git directory")
}

func isExpectedNoRemote(err error) bool {
	message := err.Error()
	return strings.Contains(message, "No such remote") ||
		strings.Contains(message, "No such remote 'origin'") ||
		strings.Contains(message, "error: No such remote")
}
