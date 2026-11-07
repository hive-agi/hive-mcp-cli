package doctor

import (
	"context"

	"github.com/hive-agi/hive-mcp-cli/internal/store"
)

// CheckStore reports whether this machine's artifact token still opens the
// store gateway, and when it does not, why, in the gateway's own words. A
// Maven client shows only "401"; this is the one place the reason surfaces.
func CheckStore() []CheckResult {
	w := store.Inspect()
	if !w.ServerFound || w.Token == "" {
		return []CheckResult{storeCheckResult(w, store.Diagnosis{})}
	}
	return []CheckResult{storeCheckResult(w, store.DiagnoseToken(context.Background(), w.Token))}
}

// storeCheckResult folds the wiring and the probe's diagnosis into one check.
// Pure: the probe ran (or did not) before it is called.
func storeCheckResult(w store.Wiring, d store.Diagnosis) CheckResult {
	const name = "Store artifact token"
	login := "hive store login"
	switch {
	case !w.ServerFound:
		return CheckResult{Name: name, Status: StatusWarning,
			Message: "no " + store.RepoID + " server in " + w.SettingsPath + " (only needed for licensed addons)",
			FixHint: login}
	case w.Token == "" || !w.UserEqualsPass:
		return CheckResult{Name: name, Status: StatusError,
			Message: "the token must be in both <username> and <password>",
			FixHint: login}
	}
	token := store.Redact(w.Token)
	switch d.Result {
	case store.ProbeAccepted:
		return CheckResult{Name: name, Status: StatusOK, Message: token + " accepted"}
	case store.ProbeRejected:
		return CheckResult{Name: name, Status: StatusError,
			Message: token + " refused (" + d.Status + ")",
			Details: d.Explanation, FixHint: login}
	case store.ProbeNotEntitled:
		return CheckResult{Name: name, Status: StatusWarning,
			Message: token + " authenticated, but refused this artifact (" + d.Status + ")",
			Details: d.Explanation}
	default:
		return CheckResult{Name: name, Status: StatusWarning,
			Message: "could not reach " + store.MavenURL() + " (" + d.Status + ")"}
	}
}
