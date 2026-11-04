//go:build !linux

package method

func dropPrivileges(username, cacheDir string) error {
	return nil
}
