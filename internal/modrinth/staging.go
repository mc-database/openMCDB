package modrinth

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

func stagingTable(name string) string { return "pg_temp." + name }

func stagingStatements() []string {
	return []string{
		`DROP TABLE IF EXISTS pg_temp._vocab_license`,
		`CREATE TEMP TABLE _vocab_license (spdx text, name text)`,
		`DROP TABLE IF EXISTS pg_temp._vocab_loader`,
		`CREATE TEMP TABLE _vocab_loader (code text, types text[])`,
		`DROP TABLE IF EXISTS pg_temp._vocab_gv`,
		`CREATE TEMP TABLE _vocab_gv (label text, released_at timestamptz, vtype text, major bool)`,
		`DROP TABLE IF EXISTS pg_temp._vocab_cat`,
		`CREATE TEMP TABLE _vocab_cat (name text, ptype text, header text)`,
		`DROP TABLE IF EXISTS pg_temp._vocab_plat`,
		`CREATE TEMP TABLE _vocab_plat (name text, donation bool)`,

		`DROP TABLE IF EXISTS pg_temp._search_disclosure`,
		`CREATE TEMP TABLE _search_disclosure (mr_id text, kinds text[])`,
		`DROP TABLE IF EXISTS pg_temp._raw_project`,
		`CREATE TEMP TABLE _raw_project (
			mr_id text, team_mr text, org_mr text, slug text, name text, status text,
			requested_status text, monetization text, gallery_count int, color int, has_icon bool,
			license_spdx text, license_name text, license_url text, side_types text,
			queued_on timestamptz, published_on timestamptz, approved_on timestamptz,
			downloads bigint, followers int, project_types text[], category_namespaces text[],
			categories text[], addl_categories text[],
			link_platforms text[], link_donations bool[], link_urls text[],
			has_server bool,
			server_address text, server_content_kind text, server_content_version_mr text,
			server_content_project_mr text, server_rec_gv text, server_country text,
			server_region text, server_languages text[], server_active_version_mr text,
			server_max_players int, server_gvs text[],
			server_v2w bigint, server_v4w bigint, server_ping_online int, server_ping_max int,
			server_ping_proto int, server_ping_latency_ms int, server_pinged_at timestamptz)`,
		`DROP TABLE IF EXISTS pg_temp._raw_version`,
		`CREATE TEMP TABLE _raw_version (
			mr_id text, project_mr text, author_mr text, name text, version_number text,
			version_type text, status text, featured bool, environment text,
			loaders text[], games text[], project_types text[], game_versions text[],
			downloads int, published_on timestamptz)`,
		`DROP TABLE IF EXISTS pg_temp._raw_version_file`,
		`CREATE TEMP TABLE _raw_version_file (
			file_id text, version_mr text, sha1 text, filename text, size bigint, primary_file bool)`,
		`DROP TABLE IF EXISTS pg_temp._raw_version_dep`,
		`CREATE TEMP TABLE _raw_version_dep (
			version_mr text, seq smallint, dep_kind text, target_project_mr text,
			target_version_mr text, file_name text)`,
		`DROP TABLE IF EXISTS pg_temp._raw_user`,
		`CREATE TEMP TABLE _raw_user (
			mr_id text, username text, role text, badges bigint, created_on timestamptz)`,
		`DROP TABLE IF EXISTS pg_temp._raw_team_member`,
		`CREATE TEMP TABLE _raw_team_member (team_mr text, user_mr text, role text, is_owner bool)`,
		`DROP TABLE IF EXISTS pg_temp._raw_org`,
		`CREATE TEMP TABLE _raw_org (mr_id text, team_mr text, slug text, name text, color int)`,

		`DROP TABLE IF EXISTS pg_temp._st_project`,
		`CREATE TEMP TABLE _st_project (
			project_id int, team_id int, slug text, name text, status modrinth.project_status, requested_status modrinth.project_status,
			monetization modrinth.monetization_status, gallery_count int, color int, has_icon bool, license_url text,
			side_types_migration_review_status modrinth.side_types_review_status, queued_on timestamptz, license_id int, org_id int)`,
		`DROP TABLE IF EXISTS pg_temp._st_version`,
		`CREATE TEMP TABLE _st_version (
			version_id int, author_id int, status modrinth.version_status, name text, version_number text,
			version_type modrinth.version_type, featured bool, environment modrinth.environment)`,
		`DROP TABLE IF EXISTS pg_temp._st_user`,
		`CREATE TEMP TABLE _st_user (user_id int, username text, role modrinth.user_role, badges bigint)`,
		`DROP TABLE IF EXISTS pg_temp._st_org`,
		`CREATE TEMP TABLE _st_org (org_id int, slug text, name text, color int)`,
		`DROP TABLE IF EXISTS pg_temp._st_team_member`,
		`CREATE TEMP TABLE _st_team_member (team_id int, user_id int, role text, is_owner bool)`,
		`DROP TABLE IF EXISTS pg_temp._st_server`,
		`CREATE TEMP TABLE _st_server (
			project_id int, content_kind modrinth.server_content_kind, content_version_id int, content_project_id int,
			content_version_mr_id text, content_project_mr_id text, recommended_game_version int,
			address text, country text, region modrinth.server_region, languages varchar[], active_version_id int,
			max_players int, active_version_mr_id text)`,
		`DROP TABLE IF EXISTS pg_temp._st_v_pt`,
		`CREATE TEMP TABLE _st_v_pt (version_id int, project_type modrinth.project_type)`,
		`DROP TABLE IF EXISTS pg_temp._st_v_game`,
		`CREATE TEMP TABLE _st_v_game (version_id int, game modrinth.game)`,
		`DROP TABLE IF EXISTS pg_temp._st_v_loader`,
		`CREATE TEMP TABLE _st_v_loader (version_id int, loader_id int)`,
		`DROP TABLE IF EXISTS pg_temp._st_v_gv`,
		`CREATE TEMP TABLE _st_v_gv (version_id int, game_version_id int)`,
		`DROP TABLE IF EXISTS pg_temp._st_p_cat`,
		`CREATE TEMP TABLE _st_p_cat (project_id int, category_id int, is_additional bool)`,
		`DROP TABLE IF EXISTS pg_temp._st_p_link`,
		`CREATE TEMP TABLE _st_p_link (project_id int, platform_id int, url text)`,
		`DROP TABLE IF EXISTS pg_temp._st_p_disc`,
		`CREATE TEMP TABLE _st_p_disc (project_id int, disclosure_kind modrinth.disclosure_kind)`,
		`DROP TABLE IF EXISTS pg_temp._st_server_gv`,
		`CREATE TEMP TABLE _st_server_gv (project_id int, game_version_id int)`,
		`DROP TABLE IF EXISTS pg_temp._st_m_project`,
		`CREATE TEMP TABLE _st_m_project (project_id int, downloads bigint, followers int)`,
		`DROP TABLE IF EXISTS pg_temp._st_m_version`,
		`CREATE TEMP TABLE _st_m_version (version_id int, downloads int)`,
		`DROP TABLE IF EXISTS pg_temp._st_m_server`,
		`CREATE TEMP TABLE _st_m_server (
			project_id int, verified_plays_2w bigint, verified_plays_4w bigint,
			ping_players_online int, ping_players_max int, ping_version_protocol int,
			ping_latency_ms int, pinged_at timestamptz)`,
		`DROP TABLE IF EXISTS pg_temp._st_file`,
		`CREATE TEMP TABLE _st_file (file_id text, version_id int, sha1 text, filename text,
			size bigint, primary_file bool)`,
		`DROP TABLE IF EXISTS pg_temp._st_dep`,
		`CREATE TEMP TABLE _st_dep (version_id int, seq smallint, dep_kind modrinth.dep_kind,
			target_project_id int, target_version_id int, target_mr_id text,
			target_version_mr_id text, file_name text)`,
	}
}

func (s *store) createStaging(ctx context.Context) error {
	for _, st := range stagingStatements() {
		if err := s.exec(ctx, st); err != nil {
			return fmt.Errorf("create staging: %w", err)
		}
	}
	return nil
}

func bulkTable(table string) (string, error) {
	if strings.HasPrefix(table, "_") {
		return stagingTable(table), nil
	}
	if !strings.Contains(table, ".") {
		return "", fmt.Errorf("bulk insert requires a schema-qualified table: %s", table)
	}
	return table, nil
}

func (s *store) bulk(ctx context.Context, table string, cols []string, rows [][]any, chunk int) (err error) {
	if len(rows) == 0 {
		return nil
	}
	table, err = bulkTable(table)
	if err != nil {
		return err
	}
	ownTx := s.tx == nil
	if ownTx {
		if err := s.beginTx(ctx); err != nil {
			return err
		}
		tx := s.tx
		defer func() {
			_ = tx.Rollback()
			s.tx = nil
		}()
	}
	if chunk <= 0 {
		chunk = 500
	}
	head := fmt.Sprintf(`INSERT INTO %s (%s) VALUES `, table, strings.Join(cols, ","))
	for i := 0; i < len(rows); i += chunk {
		end := min(i+chunk, len(rows))
		sub := rows[i:end]
		var args []any
		vals := make([]string, len(sub))
		for j, r := range sub {
			if len(r) != len(cols) {
				return fmt.Errorf("%s: row arity %d != %d", table, len(r), len(cols))
			}
			parts := make([]string, len(cols))
			for k := range cols {
				parts[k] = "$" + strconv.Itoa(j*len(cols)+k+1)
			}
			vals[j] = "(" + strings.Join(parts, ",") + ")"
			args = append(args, r...)
		}
		if _, err := s.tx.ExecContext(ctx, head+strings.Join(vals, ","), args...); err != nil {
			return fmt.Errorf("bulk insert %s: %w", table, err)
		}
	}
	if ownTx {
		return s.tx.Commit()
	}
	return nil
}

func (s *store) stageSnapshots(ctx context.Context) error {
	for _, ref := range []struct{ source, field, ids string }{
		{"_raw_version", "author_mr", "_ids_user"},
		{"_raw_team_member", "user_mr", "_ids_user"},
		{"_raw_project", "team_mr", "_ids_team"},
	} {
		var missing []string
		if err := s.tx.SelectContext(ctx, &missing, `SELECT DISTINCT r.`+ref.field+` FROM `+stagingTable(ref.source)+` r LEFT JOIN `+stagingTable(ref.ids)+` i ON i.mr_id = r.`+ref.field+` WHERE i.mr_id IS NULL LIMIT 10`); err != nil {
			return err
		}
		if len(missing) > 0 {
			return fmt.Errorf("unresolved required %s (%s) in %s: %s", ref.field, ref.ids, ref.source, strings.Join(missing, ", "))
		}
	}
	dedup := []string{
		`DELETE FROM pg_temp._raw_project a USING pg_temp._raw_project b
		 WHERE a.mr_id = b.mr_id AND a.ctid > b.ctid`,
		`DELETE FROM pg_temp._search_disclosure a USING pg_temp._search_disclosure b
		 WHERE a.mr_id = b.mr_id AND a.ctid > b.ctid`,
		`DELETE FROM pg_temp._raw_org a USING pg_temp._raw_org b
		 WHERE a.mr_id = b.mr_id AND a.ctid > b.ctid`,
	}
	for _, q := range dedup {
		if err := s.exec(ctx, q); err != nil {
			return fmt.Errorf("dedup raw tables: %w", err)
		}
	}
	if err := s.recordVocabularyMisses(ctx); err != nil {
		return err
	}
	for _, st := range snapshotStatements() {
		if err := s.exec(ctx, st); err != nil {
			return fmt.Errorf("stage: %w", err)
		}
	}
	for _, pair := range [][2]string{{"_raw_project", "_st_project"}, {"_raw_version", "_st_version"}, {"_raw_org", "_st_org"}, {"_raw_team_member", "_st_team_member"}} {
		var complete bool
		if err := s.tx.QueryRowxContext(ctx, `SELECT (SELECT count(*) FROM `+stagingTable(pair[0])+`) = (SELECT count(*) FROM `+stagingTable(pair[1])+`)`).Scan(&complete); err != nil {
			return err
		}
		if !complete {
			return fmt.Errorf("incomplete snapshot %s: unresolved required identity", pair[1])
		}
	}
	return nil
}

func snapshotStatements() []string {
	return []string{
		`INSERT INTO pg_temp._st_project
		 SELECT p.project_id, t.team_id, rp.slug, rp.name, rp.status::modrinth.project_status,
			 NULLIF(rp.requested_status, '')::modrinth.project_status,
			 rp.monetization::modrinth.monetization_status, rp.gallery_count, rp.color, rp.has_icon,
			 rp.license_url,
			 rp.side_types::modrinth.side_types_review_status, rp.queued_on, l.license_id, o.org_id
		 FROM pg_temp._raw_project rp
		 JOIN pg_temp._ids_project p ON p.mr_id = rp.mr_id
		 JOIN pg_temp._ids_team t ON t.mr_id = rp.team_mr
		 LEFT JOIN pg_temp._ids_org o ON o.mr_id = rp.org_mr
		 LEFT JOIN modrinth.licenses l ON l.spdx_id = rp.license_spdx`,
		`INSERT INTO pg_temp._st_m_project
		 SELECT p.project_id, rp.downloads, rp.followers
		 FROM pg_temp._raw_project rp JOIN pg_temp._ids_project p ON p.mr_id = rp.mr_id`,
		`INSERT INTO pg_temp._st_p_link
		 SELECT p.project_id, lp.platform_id, t.url
		 FROM pg_temp._raw_project rp
		 JOIN pg_temp._ids_project p ON p.mr_id = rp.mr_id
		 CROSS JOIN LATERAL unnest(rp.link_platforms, rp.link_donations, rp.link_urls) AS t(name, donation, url)
		 JOIN modrinth.link_platforms lp ON lp.name = t.name`,
		`INSERT INTO pg_temp._st_p_disc
		 SELECT DISTINCT p.project_id, d::modrinth.disclosure_kind FROM pg_temp._search_disclosure sd
		 JOIN pg_temp._raw_project rp ON rp.mr_id = sd.mr_id
		 JOIN pg_temp._ids_project p ON p.mr_id = rp.mr_id
		 CROSS JOIN LATERAL unnest(sd.kinds) d
		 WHERE sd.kinds IS NOT NULL AND array_length(sd.kinds,1) > 0`,
		`INSERT INTO pg_temp._st_server
		 SELECT p.project_id, rp.server_content_kind::modrinth.server_content_kind, cv.version_id,
			 cp.project_id,
			 NULLIF(rp.server_content_version_mr, ''), NULLIF(rp.server_content_project_mr, ''),
			 g.game_version_id,
			 rp.server_address, rp.server_country,
			 NULLIF(rp.server_region, '')::modrinth.server_region,
			 rp.server_languages::varchar[],
			 ca.version_id, rp.server_max_players, NULLIF(rp.server_active_version_mr, '')
		 FROM pg_temp._raw_project rp
		 JOIN pg_temp._ids_project p ON p.mr_id = rp.mr_id
		 LEFT JOIN pg_temp._ids_version cv ON cv.mr_id = rp.server_content_version_mr
		 LEFT JOIN pg_temp._ids_project cp ON cp.mr_id = rp.server_content_project_mr
		 LEFT JOIN modrinth.game_versions g ON g.label = rp.server_rec_gv
		 LEFT JOIN pg_temp._ids_version ca ON ca.mr_id = rp.server_active_version_mr
		 WHERE rp.has_server`,
		`INSERT INTO pg_temp._st_server_gv
		 SELECT p.project_id, g.game_version_id FROM pg_temp._raw_project rp
		 JOIN pg_temp._ids_project p ON p.mr_id = rp.mr_id
		 CROSS JOIN LATERAL unnest(rp.server_gvs) gv
		 JOIN modrinth.game_versions g ON g.label = gv
		 WHERE rp.server_gvs IS NOT NULL AND array_length(rp.server_gvs,1) > 0`,
		`INSERT INTO pg_temp._st_m_server
		 SELECT p.project_id, rp.server_v2w, rp.server_v4w, rp.server_ping_online,
			 rp.server_ping_max, rp.server_ping_proto, rp.server_ping_latency_ms, rp.server_pinged_at
		 FROM pg_temp._raw_project rp JOIN pg_temp._ids_project p ON p.mr_id = rp.mr_id
		 WHERE rp.has_server`,
		`INSERT INTO pg_temp._st_version
		 SELECT v.version_id, u.user_id, rv.status::modrinth.version_status, rv.name, rv.version_number,
			 rv.version_type::modrinth.version_type, rv.featured, rv.environment::modrinth.environment
		 FROM pg_temp._raw_version rv
		 JOIN pg_temp._ids_version v ON v.mr_id = rv.mr_id
		 JOIN pg_temp._ids_user u ON u.mr_id = rv.author_mr`,
		`INSERT INTO pg_temp._st_m_version
		 SELECT v.version_id, rv.downloads FROM pg_temp._raw_version rv
		 JOIN pg_temp._ids_version v ON v.mr_id = rv.mr_id`,
		`INSERT INTO pg_temp._st_v_pt
		 SELECT DISTINCT v.version_id, pt::modrinth.project_type FROM pg_temp._raw_version rv
		 JOIN pg_temp._ids_version v ON v.mr_id = rv.mr_id
		 CROSS JOIN LATERAL unnest(rv.project_types) pt`,
		`INSERT INTO pg_temp._st_v_game
		 SELECT DISTINCT v.version_id, gm::modrinth.game FROM pg_temp._raw_version rv
		 JOIN pg_temp._ids_version v ON v.mr_id = rv.mr_id
		 CROSS JOIN LATERAL unnest(rv.games) gm`,
		`INSERT INTO pg_temp._st_v_loader
		 SELECT DISTINCT v.version_id, l.loader_id FROM pg_temp._raw_version rv
		 JOIN pg_temp._ids_version v ON v.mr_id = rv.mr_id
		 CROSS JOIN LATERAL unnest(rv.loaders) lc
		 JOIN modrinth.loaders l ON l.code = lc`,
		`INSERT INTO pg_temp._st_v_gv
		 SELECT DISTINCT v.version_id, g.game_version_id FROM pg_temp._raw_version rv
		 JOIN pg_temp._ids_version v ON v.mr_id = rv.mr_id
		 CROSS JOIN LATERAL unnest(rv.game_versions) gv
		 JOIN modrinth.game_versions g ON g.label = gv`,
		`INSERT INTO pg_temp._st_p_cat
		 SELECT DISTINCT p.project_id, cc.category_id, false
		 FROM pg_temp._raw_project rp
		 JOIN pg_temp._ids_project p ON p.mr_id = rp.mr_id
		 CROSS JOIN LATERAL unnest(rp.categories) AS cat(name)
		 JOIN modrinth.categories cc ON cc.name = cat.name
			 AND cc.project_type IN (SELECT t::modrinth.project_type FROM unnest(rp.category_namespaces) AS t)
		 WHERE rp.categories IS NOT NULL AND array_length(rp.categories, 1) > 0`,
		`INSERT INTO pg_temp._st_p_cat
		 SELECT DISTINCT p.project_id, cc.category_id, true
		 FROM pg_temp._raw_project rp
		 JOIN pg_temp._ids_project p ON p.mr_id = rp.mr_id
		 CROSS JOIN LATERAL unnest(rp.addl_categories) AS cat(name)
		 JOIN modrinth.categories cc ON cc.name = cat.name
			 AND cc.project_type IN (SELECT t::modrinth.project_type FROM unnest(rp.category_namespaces) AS t)
		 WHERE rp.addl_categories IS NOT NULL AND array_length(rp.addl_categories, 1) > 0
		   AND NOT (cat.name = ANY(COALESCE(rp.categories, '{}')))`,
		`INSERT INTO pg_temp._st_user
		 SELECT u.user_id, ru.username, ru.role::modrinth.user_role, ru.badges FROM pg_temp._raw_user ru
		 JOIN pg_temp._ids_user u ON u.mr_id = ru.mr_id`,
		`INSERT INTO pg_temp._st_team_member
		 SELECT t.team_id, u.user_id, tm.role, tm.is_owner FROM pg_temp._raw_team_member tm
		 JOIN pg_temp._ids_team t ON t.mr_id = tm.team_mr
		 JOIN pg_temp._ids_user u ON u.mr_id = tm.user_mr`,
		`INSERT INTO pg_temp._st_org
		 SELECT o.org_id, ro.slug, ro.name, ro.color FROM pg_temp._raw_org ro
		 JOIN pg_temp._ids_org o ON o.mr_id = ro.mr_id`,
		`INSERT INTO pg_temp._st_file
		 SELECT f.file_id, v.version_id, f.sha1, f.filename, f.size, f.primary_file
		 FROM pg_temp._raw_version_file f JOIN pg_temp._ids_version v ON v.mr_id = f.version_mr`,
		`INSERT INTO pg_temp._st_dep
		 SELECT v.version_id, d.seq, d.dep_kind::modrinth.dep_kind, tp.project_id, tv.version_id,
			 NULLIF(d.target_project_mr, ''), NULLIF(d.target_version_mr, ''), NULLIF(d.file_name, '')
		 FROM pg_temp._raw_version_dep d
		 JOIN pg_temp._ids_version v ON v.mr_id = d.version_mr
		 LEFT JOIN pg_temp._ids_project tp ON tp.mr_id = d.target_project_mr
		 LEFT JOIN pg_temp._ids_version tv ON tv.mr_id = d.target_version_mr`,
	}
}
