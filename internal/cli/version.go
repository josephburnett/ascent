package cli

// Version is the release version, and the git tag is its one owner: the
// release build stamps it with -ldflags -X from ${GITHUB_REF_NAME#v}, and
// nothing in the tree carries a version between releases. So an unstamped
// binary — every build off a dev box — is honestly "dev" rather than a
// number that has drifted from what shipped.
var Version = ""

// VersionString is what a person is shown. It never returns empty, so a
// support question about a binary always gets an answer.
func VersionString() string {
	if Version == "" {
		return "dev"
	}
	return Version
}
