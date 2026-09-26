package checker

import (
	"context"
	"time"

	"totalplug/internal/buildinfo"
	"totalplug/internal/domain"
)

// CheckAppRelease compares the running build with the latest published release.
// A development build has no comparable version, so the latest release is
// reported without claiming the running one is out of date.
func CheckAppRelease(ctx context.Context, current string) (domain.AppUpdate, error) {
	release, err := FetchLatestGitHubRelease(ctx, buildinfo.ReleaseRepo)
	if err != nil {
		return domain.AppUpdate{}, err
	}

	result := domain.AppUpdate{
		Current:    current,
		Latest:     release.TagName,
		Comparable: isResolvableVersion(current),
		ReleaseURL: release.HTMLURL,
		CheckedAt:  time.Now().UTC(),
	}
	result.Available = result.Comparable && CompareVersions(current, release.TagName) < 0
	return result, nil
}
