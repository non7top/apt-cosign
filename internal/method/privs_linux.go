//go:build linux

package method

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
)

// dropPrivileges drops root privileges to the specified sandbox user (defaulting
// to "_apt" if username is empty). If running unprivileged or if username is
// "root", it is a no-op.
func dropPrivileges(username, cacheDir string) error {
	if os.Getuid() != 0 && os.Geteuid() != 0 {
		return nil
	}
	if username == "" {
		username = "_apt"
	}
	if username == "root" {
		return nil
	}

	u, err := user.Lookup(username)
	if err != nil {
		return fmt.Errorf("lookup sandbox user %q: %w", username, err)
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return fmt.Errorf("invalid uid %q for sandbox user %q: %w", u.Uid, username, err)
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return fmt.Errorf("invalid gid %q for sandbox user %q: %w", u.Gid, username, err)
	}

	// Prepare cache directory and existing log file ownership while still root
	// so the unprivileged user can write to them.
	if cacheDir != "" {
		_ = os.MkdirAll(cacheDir, 0700)
		_ = os.Chown(cacheDir, uid, gid)
		debugLog := filepath.Join(cacheDir, "debug.log")
		if info, err := os.Lstat(debugLog); err == nil && info.Mode().IsRegular() {
			_ = os.Chown(debugLog, uid, gid)
		}
	}

	// Set PR_SET_NO_NEW_PRIVS (38) so child processes cannot acquire new privileges.
	_, _, _ = syscall.RawSyscall(syscall.SYS_PRCTL, 38, 1, 0)

	// Clear supplementary groups (keeping primary gid) and set GID before setting UID, matching apt.
	if err := syscall.Setgroups([]int{gid}); err != nil {
		return fmt.Errorf("setgroups(%d) for %q: %w", gid, username, err)
	}
	if err := syscall.Setresgid(gid, gid, gid); err != nil {
		return fmt.Errorf("setresgid(%d): %w", gid, err)
	}
	if err := syscall.Setresuid(uid, uid, uid); err != nil {
		return fmt.Errorf("setresuid(%d): %w", uid, err)
	}

	if os.Getuid() != uid || os.Geteuid() != uid || os.Getgid() != gid || os.Getegid() != gid {
		return fmt.Errorf("privilege drop verification failed: uid=%d, euid=%d, gid=%d, egid=%d (want uid=%d, gid=%d)",
			os.Getuid(), os.Geteuid(), os.Getgid(), os.Getegid(), uid, gid)
	}

	return nil
}
