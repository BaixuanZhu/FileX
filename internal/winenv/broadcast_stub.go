//go:build !windows

package winenv

func BroadcastEnvChange() error {
	return nil // No-op on non-Windows platforms
}
