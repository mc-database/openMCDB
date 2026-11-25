package modrinth

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

type stateSpec struct {
	table   string
	snap    string
	key     []string
	content []string
}

func (s *store) applyState(ctx context.Context, sp stateSpec) error {
	start := time.Now()
	defer func() {
		log.Printf("openmcdb/modrinth: merge %s took %s", sp.table, time.Since(start).Round(time.Millisecond))
	}()
	if err := s.exec(ctx, `CREATE UNIQUE INDEX ON `+stagingTable(sp.snap)+` (`+strings.Join(sp.key, ",")+`)`); err != nil {
		return fmt.Errorf("index snapshot %s: %w", sp.snap, err)
	}
	if err := s.exec(ctx, `ANALYZE `+stagingTable(sp.snap)); err != nil {
		return err
	}
	if err := s.exec(ctx, `ANALYZE modrinth.`+sp.table); err != nil {
		return err
	}
	if err := s.exec(ctx, buildRetire(sp)); err != nil {
		return fmt.Errorf("retire %s: %w", sp.table, err)
	}
	if err := s.exec(ctx, buildClose(sp.table, sp.snap, sp.key, sp.content, s.crawlID)); err != nil {
		return fmt.Errorf("refresh %s: %w", sp.table, err)
	}
	if err := s.exec(ctx, buildInsert(sp.table, sp.snap, sp.key, sp.content, s.crawlID)); err != nil {
		return fmt.Errorf("insert %s: %w", sp.table, err)
	}
	return nil
}

func buildRetire(sp stateSpec) string {
	owner, _ := stateScope(sp)
	return `UPDATE modrinth.` + sp.table + ` t SET is_current = false
	 WHERE t.is_current AND t.` + owner + ` IN (SELECT ` + owner + ` FROM ` + snapshotScope(sp) + `)
	 AND NOT EXISTS (SELECT 1 FROM ` + stagingTable(sp.snap) + ` x WHERE ` + keyJoin(sp.key, "t", "x") + contentEquals(sp.content, "t", "x") + `)`
}

func contentEquals(cols []string, a, b string) string {
	var result strings.Builder
	for _, col := range cols {
		_, _ = fmt.Fprintf(&result, " AND %s.%s IS NOT DISTINCT FROM %s.%s", a, col, b, col)
	}
	return result.String()
}

func buildClose(table, snap string, k, cols []string, crawlID int) string {
	return `UPDATE modrinth.` + table + ` t SET last_seen_crawl = ` + sqlInt(crawlID) + `
	 FROM ` + stagingTable(snap) + ` x WHERE t.is_current AND ` + keyJoin(k, "t", "x") + contentEquals(cols, "t", "x")
}

func buildInsert(table, snap string, k, cols []string, crawlID int) string {
	allCols := append(append(append([]string{}, k...), cols...), "first_seen_crawl", "last_seen_crawl", "is_current")
	sel := append(append(prefixEach(k, "x."), prefixEach(cols, "x.")...), sqlInt(crawlID), sqlInt(crawlID), "true")
	return `WITH current_keys AS MATERIALIZED (SELECT ` + strings.Join(k, ",") + ` FROM modrinth.` + table + ` WHERE is_current)
	 INSERT INTO modrinth.` + table + ` (` + strings.Join(allCols, ",") + `)
	 SELECT ` + strings.Join(sel, ",") + ` FROM ` + stagingTable(snap) + ` x
	 WHERE NOT EXISTS (SELECT 1 FROM current_keys t WHERE ` + keyJoin(k, "t", "x") + `)`
}

func keyJoin(k []string, a, b string) string {
	parts := make([]string, len(k))
	for i, col := range k {
		parts[i] = a + "." + col + "=" + b + "." + col
	}
	return strings.Join(parts, " AND ")
}

func prefixEach(ss []string, p string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = p + s
	}
	return out
}

func (s *store) applyMetrics(ctx context.Context, table, snap string, k, valueCols []string) error {
	allCols := append(append(append([]string{}, k...), valueCols...), "crawl_id")
	contentMatch := ""
	if len(valueCols) > 0 {
		parts := make([]string, len(valueCols))
		for i, col := range valueCols {
			parts[i] = fmt.Sprintf("l.%s IS NOT DISTINCT FROM x.%s", col, col)
		}
		contentMatch = " AND " + strings.Join(parts, " AND ")
	}
	sel := append(append(prefixEach(k, "x."), prefixEach(valueCols, "x.")...), sqlInt(s.crawlID))
	var populated bool
	if err := s.tx.QueryRowxContext(ctx, `SELECT EXISTS (SELECT 1 FROM modrinth.`+table+`)`).Scan(&populated); err != nil {
		return err
	}
	if !populated {
		return s.exec(ctx, `INSERT INTO modrinth.`+table+` (`+strings.Join(allCols, ",")+`) SELECT `+strings.Join(sel, ",")+` FROM `+stagingTable(snap)+` x`)
	}
	if err := s.exec(ctx, `ANALYZE modrinth.`+table); err != nil {
		return err
	}
	return s.exec(ctx, fmt.Sprintf(`
		INSERT INTO modrinth.%[1]s (%[2]s)
		SELECT %[3]s FROM %[4]s x
		LEFT JOIN LATERAL (
		 SELECT %[5]s, crawl_id FROM modrinth.%[1]s l WHERE %[6]s ORDER BY crawl_id DESC LIMIT 1
		) l ON true
		WHERE l.crawl_id IS NULL OR NOT (true%[7]s)`,
		table, strings.Join(allCols, ","), strings.Join(sel, ","), stagingTable(snap),
		strings.Join(valueCols, ","), keyJoin(k, "l", "x"), contentMatch))
}

func stateSpecs() []stateSpec {
	return []stateSpec{
		{"project_states", "_st_project", []string{"project_id"},
			[]string{"team_id", "slug", "name", "status", "requested_status", "monetization",
				"gallery_count", "color", "has_icon", "license_url",
				"side_types_migration_review_status", "queued_on", "license_id", "org_id"}},
		{"version_states", "_st_version", []string{"version_id"},
			[]string{"author_id", "status", "name", "version_number", "version_type",
				"featured", "environment"}},
		{"user_state", "_st_user", []string{"user_id"},
			[]string{"username", "role", "badges"}},
		{"organisation_states", "_st_org", []string{"org_id"},
			[]string{"slug", "name", "color"}},
		{"team_member_states", "_st_team_member", []string{"team_id", "user_id"},
			[]string{"role", "is_owner"}},
		{"project_category_states", "_st_p_cat",
			[]string{"project_id", "category_id"}, []string{"is_additional"}},
		{"project_link_states", "_st_p_link", []string{"project_id", "platform_id"},
			[]string{"url"}},
		{"project_disclosure_states", "_st_p_disc",
			[]string{"project_id", "disclosure_kind"}, nil},
		{"version_project_type_states", "_st_v_pt",
			[]string{"version_id", "project_type"}, nil},
		{"version_game_states", "_st_v_game", []string{"version_id", "game"}, nil},
		{"version_loader_states", "_st_v_loader",
			[]string{"version_id", "loader_id"}, nil},
		{"version_game_version_states", "_st_v_gv",
			[]string{"version_id", "game_version_id"}, nil},
		{"server_game_version_states", "_st_server_gv",
			[]string{"project_id", "game_version_id"}, nil},
		{"server_state", "_st_server", []string{"project_id"},
			[]string{"content_kind", "content_version_id", "content_project_id",
				"content_version_mr_id", "content_project_mr_id",
				"recommended_game_version", "address", "country", "region", "languages",
				"active_version_id", "max_players", "active_version_mr_id"}},
		{"version_dependencies", "_st_dep", []string{"version_id", "seq"},
			[]string{"dep_kind", "target_project_id", "target_version_id", "target_mr_id", "target_version_mr_id", "file_name"}},
	}
}

func stateScope(sp stateSpec) (string, string) {
	switch sp.key[0] {
	case "project_id":
		return "project_id", "projects"
	case "version_id":
		return "version_id", "versions"
	case "team_id":
		return "team_id", "teams"
	case "user_id":
		return "user_id", "app_users"
	case "org_id":
		return "org_id", "organisations"
	default:
		panic("unrecognized state owner: " + sp.table)
	}
}

func snapshotScope(sp stateSpec) string {
	owner, _ := stateScope(sp)
	switch owner {
	case "project_id":
		return "pg_temp._st_m_project"
	case "version_id":
		return "pg_temp._st_m_version"
	case "team_id":
		return "pg_temp._observed_team"
	case "user_id":
		return "pg_temp._st_user"
	case "org_id":
		return "pg_temp._st_org"
	default:
		panic("unrecognized snapshot owner: " + owner)
	}
}

func (s *store) applyAll(ctx context.Context) error {
	for _, sp := range stateSpecs() {
		if err := s.applyState(ctx, sp); err != nil {
			return err
		}
	}

	if err := s.applyMetrics(ctx, "project_metrics", "_st_m_project",
		[]string{"project_id"}, []string{"downloads", "followers"}); err != nil {
		return err
	}
	if err := s.applyMetrics(ctx, "version_metrics", "_st_m_version",
		[]string{"version_id"}, []string{"downloads"}); err != nil {
		return err
	}
	if err := s.applyMetrics(ctx, "server_metrics", "_st_m_server",
		[]string{"project_id"},
		[]string{"verified_plays_2w", "verified_plays_4w", "ping_players_online",
			"ping_players_max", "ping_version_protocol", "ping_latency_ms", "pinged_at"}); err != nil {
		return err
	}
	return s.applyFiles(ctx)
}

func (s *store) applyFiles(ctx context.Context) error {
	if err := s.exec(ctx, `CREATE UNIQUE INDEX ON pg_temp._st_file (file_id)`); err != nil {
		return fmt.Errorf("index snapshot files: %w", err)
	}
	if err := s.exec(ctx, `ANALYZE pg_temp._st_file`); err != nil {
		return err
	}
	if err := s.exec(ctx, buildRetireFiles()); err != nil {
		return fmt.Errorf("retire files: %w", err)
	}
	if err := s.exec(ctx, buildUpsertFiles(s.crawlID)); err != nil {
		return fmt.Errorf("upsert files: %w", err)
	}
	return nil
}

func buildRetireFiles() string {
	return `UPDATE modrinth.version_files t SET is_current = false
	 WHERE t.is_current AND t.version_id IN (SELECT version_id FROM pg_temp._st_m_version)
	 AND NOT EXISTS (SELECT 1 FROM pg_temp._st_file x WHERE x.file_id = t.file_id AND x.version_id = t.version_id)`
}

func buildUpsertFiles(crawlID int) string {
	return `INSERT INTO modrinth.version_files (file_id, version_id, sha1, filename, size, primary_file, first_seen_crawl, last_seen_crawl, is_current)
	 SELECT file_id, version_id, sha1, filename, size, primary_file, ` + sqlInt(crawlID) + `, ` + sqlInt(crawlID) + `, true
	 FROM pg_temp._st_file
	 ON CONFLICT (file_id) DO UPDATE SET
		version_id = EXCLUDED.version_id, sha1 = EXCLUDED.sha1, filename = EXCLUDED.filename,
		size = EXCLUDED.size, primary_file = EXCLUDED.primary_file,
		last_seen_crawl = EXCLUDED.last_seen_crawl, is_current = true`
}
