package cli

// Version is stamped by the release build from the git tag, its one owner.
// An unstamped binary is honestly "dev" rather than a drifted number.
var Version = ""

// VersionString never returns empty, so a support question always gets an
// answer.
func VersionString() string {
	if Version == "" {
		return "dev"
	}
	return Version
}
