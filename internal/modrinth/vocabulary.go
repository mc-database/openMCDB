package modrinth

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
)

func (s *store) applyAPIVocabulary(ctx context.Context) error {
	stmts := []struct {
		name  string
		query string
	}{
		{"licenses", `
			INSERT INTO modrinth.licenses (spdx_id, name, is_custom, first_seen_crawl, last_seen_crawl)
			SELECT spdx, name, false, ` + sqlInt(s.crawlID) + `, ` + sqlInt(s.crawlID) + ` FROM pg_temp._vocab_license
			ON CONFLICT (spdx_id) DO UPDATE SET last_seen_crawl = EXCLUDED.last_seen_crawl`},
		{"loaders", `
			INSERT INTO modrinth.loaders (code, supported_project_types, first_seen_crawl, last_seen_crawl)
			SELECT code, types::modrinth.project_type[], ` + sqlInt(s.crawlID) + `, ` + sqlInt(s.crawlID) + ` FROM pg_temp._vocab_loader
			ON CONFLICT (code) DO UPDATE SET
				supported_project_types = EXCLUDED.supported_project_types,
				last_seen_crawl = EXCLUDED.last_seen_crawl`},
		{"game versions", `
			INSERT INTO modrinth.game_versions (label, released_at, version_type, major, first_seen_crawl, last_seen_crawl)
			SELECT label, COALESCE(released_at, NOW()), vtype::modrinth.game_version_type, major, ` + sqlInt(s.crawlID) + `, ` + sqlInt(s.crawlID) + `
			FROM pg_temp._vocab_gv
			ON CONFLICT (label) DO UPDATE SET
				released_at = EXCLUDED.released_at, version_type = EXCLUDED.version_type,
				major = EXCLUDED.major, last_seen_crawl = EXCLUDED.last_seen_crawl`},
		{"categories", `
			INSERT INTO modrinth.categories (name, project_type, header, first_seen_crawl, last_seen_crawl)
			SELECT name, ptype::modrinth.project_type, header, ` + sqlInt(s.crawlID) + `, ` + sqlInt(s.crawlID) + ` FROM pg_temp._vocab_cat
			ON CONFLICT (name, project_type) DO UPDATE SET
				header = EXCLUDED.header, is_from_api = true, last_seen_crawl = EXCLUDED.last_seen_crawl`},
		{"link platforms", `
			INSERT INTO modrinth.link_platforms (name, donation, first_seen_crawl, last_seen_crawl)
			SELECT name, donation, ` + sqlInt(s.crawlID) + `, ` + sqlInt(s.crawlID) + ` FROM pg_temp._vocab_plat
			ON CONFLICT (name) DO UPDATE SET last_seen_crawl = EXCLUDED.last_seen_crawl`},
	}
	for _, stmt := range stmts {
		if err := s.exec(ctx, stmt.query); err != nil {
			return fmt.Errorf("vocab %s: %w", stmt.name, err)
		}
	}
	return nil
}

func (s *store) applyVocabulary(ctx context.Context) error {
	if err := s.exec(ctx, `
		INSERT INTO modrinth.categories (name, project_type, header, is_from_api, first_seen_crawl, last_seen_crawl)
		SELECT DISTINCT cat.name, ns.ptype::modrinth.project_type, '', false, `+sqlInt(s.crawlID)+`, `+sqlInt(s.crawlID)+`
		FROM pg_temp._raw_project rp
		CROSS JOIN LATERAL unnest(rp.categories || rp.addl_categories) AS cat(name)
		CROSS JOIN LATERAL unnest(rp.category_namespaces) AS ns(ptype)
		WHERE NULLIF(cat.name, '') IS NOT NULL
			AND ns.ptype IN ('mod', 'modpack', 'resourcepack', 'shader', 'minecraft_java_server')
			AND NOT EXISTS (SELECT 1 FROM modrinth.categories cc WHERE cc.name = cat.name AND cc.is_from_api
				AND cc.project_type IN (SELECT t::modrinth.project_type FROM unnest(rp.category_namespaces) t))
		ON CONFLICT (name, project_type) DO UPDATE SET last_seen_crawl = EXCLUDED.last_seen_crawl`); err != nil {
		return fmt.Errorf("vocab observed categories: %w", err)
	}
	if err := s.exec(ctx,
		`INSERT INTO modrinth.licenses (spdx_id, name, is_custom, first_seen_crawl, last_seen_crawl)
		 SELECT license_spdx, license_name, true, `+sqlInt(s.crawlID)+`, `+sqlInt(s.crawlID)+`
		 FROM (SELECT DISTINCT ON (license_spdx) license_spdx, license_name FROM pg_temp._raw_project ORDER BY license_spdx, license_name) d
		 WHERE NULLIF(license_spdx, '') IS NOT NULL
		 ON CONFLICT (spdx_id) DO UPDATE SET last_seen_crawl = EXCLUDED.last_seen_crawl`); err != nil {
		return fmt.Errorf("vocab licenses: %w", err)
	}
	if err := s.exec(ctx,
		`INSERT INTO modrinth.loaders (code, supported_project_types, first_seen_crawl, last_seen_crawl)
		 SELECT lc, '{}', `+sqlInt(s.crawlID)+`, `+sqlInt(s.crawlID)+`
		 FROM (SELECT DISTINCT lc FROM pg_temp._raw_version, unnest(loaders) lc) d
		 ON CONFLICT (code) DO UPDATE SET last_seen_crawl = EXCLUDED.last_seen_crawl`); err != nil {
		return fmt.Errorf("vocab loaders: %w", err)
	}
	if err := s.exec(ctx, `
		INSERT INTO modrinth.game_versions (label, released_at, version_type, major, first_seen_crawl, last_seen_crawl)
		SELECT label, NOW(), 'unknown', false, `+sqlInt(s.crawlID)+`, `+sqlInt(s.crawlID)+` FROM (
			SELECT DISTINCT gv AS label FROM pg_temp._raw_version, unnest(game_versions) gv
			UNION
			SELECT DISTINCT gv FROM pg_temp._raw_project, unnest(server_gvs) gv
			UNION
			SELECT DISTINCT server_rec_gv FROM pg_temp._raw_project WHERE server_rec_gv IS NOT NULL
		) d WHERE NULLIF(label, '') IS NOT NULL
		ON CONFLICT (label) DO UPDATE SET last_seen_crawl = EXCLUDED.last_seen_crawl`); err != nil {
		return fmt.Errorf("vocab game_versions: %w", err)
	}
	if err := s.exec(ctx,
		`INSERT INTO modrinth.link_platforms (name, donation, first_seen_crawl, last_seen_crawl)
		 SELECT n, dn, `+sqlInt(s.crawlID)+`, `+sqlInt(s.crawlID)+`
		 FROM (SELECT DISTINCT n, dn FROM pg_temp._raw_project, unnest(link_platforms, link_donations) t(n, dn)) d
		 ON CONFLICT (name) DO UPDATE SET last_seen_crawl = EXCLUDED.last_seen_crawl`); err != nil {
		return fmt.Errorf("vocab link_platforms: %w", err)
	}
	return nil
}

const (
	vocabularyMissSampleLimit = 10
	vocabularyMissValueLimit  = 120
)

type vocabularyMiss struct {
	Kind    string `db:"kind"`
	Value   string `db:"value"`
	Count   int    `db:"count"`
	Sampled bool   `db:"sampled"`
}

func (s *store) recordVocabularyMisses(ctx context.Context) error {
	var misses []vocabularyMiss
	if err := s.tx.SelectContext(ctx, &misses, vocabularyMissesSQL()); err != nil {
		return fmt.Errorf("count vocabulary misses: %w", err)
	}
	for _, miss := range misses {
		s.recordVocabularyMiss(miss)
		if miss.Sampled {
			log.Printf("openmcdb/modrinth: vocabulary miss %s %q (%d observations)", miss.Kind, miss.Value, miss.Count)
		}
	}
	return nil
}

func (s *store) recordVocabularyMiss(miss vocabularyMiss) {
	prefix := "vocabulary_misses_" + miss.Kind
	s.coverage[prefix] += miss.Count
	if !miss.Sampled {
		s.coverage[prefix+":other"] += miss.Count
		return
	}
	rawPrefix := prefix + ":raw:"
	value := []rune(miss.Value)
	if len(value) > vocabularyMissValueLimit {
		value = value[:vocabularyMissValueLimit]
	}
	key := rawPrefix + strconv.Quote(string(value))
	if _, ok := s.coverage[key]; !ok {
		var samples int
		for key := range s.coverage {
			if strings.HasPrefix(key, rawPrefix) {
				samples++
			}
		}
		if samples >= vocabularyMissSampleLimit {
			s.coverage[prefix+":other"] += miss.Count
			return
		}
	}
	s.coverage[key] += miss.Count
}

func vocabularyMissesSQL() string {
	return fmt.Sprintf(`WITH missing AS (
		SELECT 'categories' AS kind,
			COALESCE(cat.name, '<null>') || ' [' || COALESCE(array_to_string(rp.project_types, ','), '') || ']' AS value
		FROM pg_temp._raw_project rp CROSS JOIN LATERAL unnest(rp.categories) AS cat(name)
		WHERE NOT EXISTS (SELECT 1 FROM modrinth.categories cc WHERE cc.name = cat.name
			AND cc.project_type IN (SELECT t::modrinth.project_type FROM unnest(rp.category_namespaces) t))
		UNION ALL
		SELECT 'additional_categories',
			COALESCE(cat.name, '<null>') || ' [' || COALESCE(array_to_string(rp.project_types, ','), '') || ']'
		FROM pg_temp._raw_project rp CROSS JOIN LATERAL unnest(rp.addl_categories) AS cat(name)
		WHERE NOT (cat.name = ANY(COALESCE(rp.categories, '{}')))
			AND NOT EXISTS (SELECT 1 FROM modrinth.categories cc WHERE cc.name = cat.name
				AND cc.project_type IN (SELECT t::modrinth.project_type FROM unnest(rp.category_namespaces) t))
		UNION ALL
		SELECT 'loaders', COALESCE(lc, '<null>')
		FROM pg_temp._raw_version rv CROSS JOIN LATERAL unnest(rv.loaders) lc
		WHERE NOT EXISTS (SELECT 1 FROM modrinth.loaders l WHERE l.code = lc)
		UNION ALL
		SELECT 'game_versions', COALESCE(gv, '<null>')
		FROM pg_temp._raw_version rv CROSS JOIN LATERAL unnest(rv.game_versions) gv
		WHERE NOT EXISTS (SELECT 1 FROM modrinth.game_versions g WHERE g.label = gv)
		UNION ALL
		SELECT 'server_game_versions', COALESCE(gv, '<null>')
		FROM pg_temp._raw_project rp CROSS JOIN LATERAL unnest(rp.server_gvs) gv
		WHERE rp.has_server AND NOT EXISTS (SELECT 1 FROM modrinth.game_versions g WHERE g.label = gv)
		UNION ALL
		SELECT 'recommended_game_versions', rp.server_rec_gv
		FROM pg_temp._raw_project rp
		WHERE rp.has_server AND rp.server_rec_gv IS NOT NULL
			AND NOT EXISTS (SELECT 1 FROM modrinth.game_versions g WHERE g.label = rp.server_rec_gv)
		UNION ALL
		SELECT 'licenses', COALESCE(rp.license_spdx, '<null>')
		FROM pg_temp._raw_project rp
		WHERE NOT EXISTS (SELECT 1 FROM modrinth.licenses l WHERE l.spdx_id = rp.license_spdx)
		UNION ALL
		SELECT 'link_platforms', COALESCE(lp, '<null>')
		FROM pg_temp._raw_project rp CROSS JOIN LATERAL unnest(rp.link_platforms) lp
		WHERE NOT EXISTS (SELECT 1 FROM modrinth.link_platforms l WHERE l.name = lp)
	), counts AS (
		SELECT kind, value, count(*) AS count FROM missing GROUP BY kind, value
	), ranked AS (
		SELECT kind, value, count, row_number() OVER (PARTITION BY kind ORDER BY count DESC, value) AS rank
		FROM counts
	)
	SELECT kind, CASE WHEN rank <= %[1]d THEN left(value, %[2]d) ELSE '' END AS value,
		rank <= %[1]d AS sampled, sum(count) AS count
	FROM ranked GROUP BY kind, sampled, 2 ORDER BY kind, sampled DESC, value`,
		vocabularyMissSampleLimit, vocabularyMissValueLimit)
}
