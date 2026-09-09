// Package version holds Koalmine's version number. It's the single source
// of truth the updater compares against — bump it here (and the matching
// "productVersion" in wails.json, which NSIS reads separately for the
// installer's metadata) when cutting a release. See RELEASING.md.
package version

const Current = "0.3.0"
