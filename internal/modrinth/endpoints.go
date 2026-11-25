package modrinth

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

func (c *client) searchPage(ctx context.Context, offset, limit int) (searchResults, error) {
	q := url.Values{}
	q.Set("limit", strconv.Itoa(limit))
	q.Set("offset", strconv.Itoa(offset))
	q.Set("index", "newest")
	if c.scope != "" {
		q.Set("facets", c.scope)
	}
	var out searchResults
	err := c.getJSON(ctx, "/v3/search", q, &out)
	return out, err
}

func (c *client) projectsBatch(ctx context.Context, ids []string) ([]apiProject, error) {
	return getIDBatch(ctx, c, "/v3/projects", ids, nil, func(p apiProject) string { return p.ID })
}

func (c *client) versionsBatch(ctx context.Context, ids []string) ([]apiVersion, error) {
	q := url.Values{"include_changelog": {"false"}}
	return getIDBatch(ctx, c, "/v3/versions", ids, q, func(v apiVersion) string { return v.ID })
}

func (c *client) teamsBatch(ctx context.Context, ids []string) ([][]apiTeamMember, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var out [][]apiTeamMember
	if err := c.getJSON(ctx, "/v3/teams", url.Values{"ids": {jsonList(ids)}}, &out); err != nil {
		return out, err
	}
	return out, validateTeamsBatch(ids, out)
}

func (c *client) orgsBatch(ctx context.Context, ids []string) ([]apiOrganization, error) {
	return getIDBatch(ctx, c, "/v3/organizations", ids, nil, func(o apiOrganization) string { return o.ID })
}

func (c *client) usersBatch(ctx context.Context, ids []string) ([]apiUser, error) {
	return getIDBatch(ctx, c, "/v3/users", ids, nil, func(u apiUser) string { return u.ID })
}

func getIDBatch[T any](ctx context.Context, c *client, path string, ids []string, q url.Values, id func(T) string) ([]T, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	if q == nil {
		q = url.Values{}
	}
	q.Set("ids", jsonList(ids))
	var out []T
	if err := c.getJSON(ctx, path, q, &out); err != nil {
		return out, err
	}
	return out, validateBatchIDs(path, ids, out, id)
}

func validateBatchIDs[T any](path string, requested []string, rows []T, id func(T) string) error {
	wanted := make(map[string]bool, len(requested))
	for _, v := range requested {
		if v == "" {
			return fmt.Errorf("modrinth: %s batch requested an empty ID", path)
		}
		wanted[v] = true
	}
	seen := make(map[string]bool, len(rows))
	duplicateSet := make(map[string]bool)
	var missing, unexpected, duplicates []string
	for _, row := range rows {
		v := id(row)
		if seen[v] {
			if !duplicateSet[v] {
				duplicates = append(duplicates, v)
				duplicateSet[v] = true
			}
			continue
		}
		seen[v] = true
		if !wanted[v] {
			unexpected = append(unexpected, v)
		}
	}
	for _, v := range requested {
		if wanted[v] && !seen[v] {
			missing = append(missing, v)
			wanted[v] = false
		}
	}
	if len(missing)+len(unexpected)+len(duplicates) == 0 {
		return nil
	}
	return &batchCoverageError{path: path, requested: len(wanted), returned: len(rows), missing: missing, unexpected: unexpected, duplicates: duplicates}
}

type batchCoverageError struct {
	path                            string
	requested, returned             int
	missing, unexpected, duplicates []string
}

func (e *batchCoverageError) Error() string {
	return fmt.Sprintf("modrinth: %s batch coverage mismatch: requested_unique=%d returned=%d; missing IDs %s (objects may be nonexistent or invisible); unexpected IDs %s; duplicate IDs %s", e.path, e.requested, e.returned, idPreview(e.missing), idPreview(e.unexpected), idPreview(e.duplicates))
}

func (e *batchCoverageError) onlyMissing() bool {
	return len(e.missing) > 0 && len(e.unexpected) == 0 && len(e.duplicates) == 0
}

func idPreview(ids []string) string {
	const previewLimit = 8
	preview := make([]string, min(len(ids), previewLimit))
	for i := range preview {
		preview[i] = strconv.Quote(truncate(ids[i], 64))
	}
	suffix := ""
	if len(ids) > len(preview) {
		suffix = fmt.Sprintf(" (+%d more)", len(ids)-len(preview))
	}
	return fmt.Sprintf("%d [%s]%s", len(ids), strings.Join(preview, ", "), suffix)
}

func validateTeamsBatch(requested []string, teams [][]apiTeamMember) error {
	wanted := make(map[string]bool, len(requested))
	for _, id := range requested {
		if id == "" {
			return fmt.Errorf("modrinth: /v3/teams batch requested an empty ID")
		}
		wanted[id] = true
	}
	if len(teams) > len(wanted) {
		return fmt.Errorf("modrinth: /v3/teams returned %d groups for %d unique requested IDs", len(teams), len(wanted))
	}
	seen := make(map[string]bool)
	for _, members := range teams {
		if len(members) == 0 {
			continue
		}
		teamID := members[0].TeamID
		if !wanted[teamID] {
			return fmt.Errorf("modrinth: /v3/teams unexpected team IDs %s", idPreview([]string{teamID}))
		}
		if seen[teamID] {
			return fmt.Errorf("modrinth: /v3/teams duplicate team IDs %s", idPreview([]string{teamID}))
		}
		seen[teamID] = true
		users := make(map[string]bool, len(members))
		for _, member := range members {
			if member.TeamID != teamID {
				return fmt.Errorf("modrinth: /v3/teams mixed team IDs in one group %s", idPreview([]string{teamID, member.TeamID}))
			}
			if member.User.ID == "" {
				return fmt.Errorf("modrinth: /v3/teams empty user ID in team %s", idPreview([]string{teamID}))
			}
			if users[member.User.ID] {
				return fmt.Errorf("modrinth: /v3/teams duplicate member IDs %s in team %s", idPreview([]string{member.User.ID}), idPreview([]string{teamID}))
			}
			users[member.User.ID] = true
		}
	}
	return nil
}

func (c *client) categories(ctx context.Context) ([]apiCategory, error) {
	var out []apiCategory
	err := c.getJSON(ctx, "/v3/tag/category", nil, &out)
	return out, err
}

func (c *client) loaders(ctx context.Context) ([]apiLoader, error) {
	var out []apiLoader
	err := c.getJSON(ctx, "/v3/tag/loader", nil, &out)
	return out, err
}

func (c *client) linkPlatforms(ctx context.Context) ([]apiLinkPlatform, error) {
	var out []apiLinkPlatform
	err := c.getJSON(ctx, "/v3/link_platform", nil, &out)
	return out, err
}

func (c *client) gameVersions(ctx context.Context) ([]apiGameVersion, error) {
	var out []apiGameVersion
	err := c.getJSON(ctx, "/v3/loader_field", url.Values{"loader_field": {"game_versions"}}, &out)
	return out, err
}

func (c *client) licenses(ctx context.Context) ([]apiLicenseRef, error) {
	var out []apiLicenseRef
	err := c.getJSON(ctx, "/v3/license", nil, &out)
	return out, err
}

func (c *client) statistics(ctx context.Context) (apiStatistics, error) {
	var out apiStatistics
	err := c.getJSON(ctx, "/v3/statistics", nil, &out)
	return out, err
}

func (c *client) platformRevenue(ctx context.Context) (apiRevenue, error) {
	var out apiRevenue
	err := c.getJSON(ctx, "/v3/payout/platform_revenue", nil, &out)
	return out, err
}
