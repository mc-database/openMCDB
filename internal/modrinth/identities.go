package modrinth

import (
	"context"
	"fmt"
)

func (s *store) applyIdentities(ctx context.Context) error {
	if err := s.exec(ctx,
		`INSERT INTO modrinth.projects (mr_id, published_on, approved_on, first_seen_crawl, last_seen_crawl)
		 SELECT mr_id, published_on, approved_on, `+sqlInt(s.crawlID)+`, `+sqlInt(s.crawlID)+`
		 FROM pg_temp._raw_project ON CONFLICT (mr_id) DO NOTHING`); err != nil {
		return fmt.Errorf("identity projects: %w", err)
	}
	if err := s.exec(ctx,
		`UPDATE modrinth.projects SET last_seen_crawl=`+sqlInt(s.crawlID)+` WHERE mr_id IN (SELECT mr_id FROM pg_temp._raw_project)`); err != nil {
		return err
	}
	if err := s.exec(ctx, `
		INSERT INTO modrinth.teams (mr_id, first_seen_crawl, last_seen_crawl)
		SELECT mr_id, `+sqlInt(s.crawlID)+`, `+sqlInt(s.crawlID)+` FROM (
			SELECT DISTINCT team_mr AS mr_id FROM pg_temp._raw_project
			UNION
			SELECT DISTINCT team_mr FROM pg_temp._raw_team_member
			UNION
			SELECT DISTINCT team_mr FROM pg_temp._raw_org
		) d
		ON CONFLICT (mr_id) DO NOTHING`); err != nil {
		return fmt.Errorf("identity teams: %w", err)
	}
	if err := s.exec(ctx,
		`UPDATE modrinth.teams SET last_seen_crawl=`+sqlInt(s.crawlID)+` WHERE mr_id IN (SELECT team_mr FROM pg_temp._raw_project UNION SELECT team_mr FROM pg_temp._raw_team_member UNION SELECT team_mr FROM pg_temp._raw_org)`); err != nil {
		return err
	}
	if err := s.exec(ctx, `
		INSERT INTO modrinth.organisations (mr_id, team_id, first_seen_crawl, last_seen_crawl)
		SELECT o.mr_id, t.team_id, `+sqlInt(s.crawlID)+`, `+sqlInt(s.crawlID)+`
		FROM pg_temp._raw_org o JOIN modrinth.teams t ON t.mr_id = o.team_mr
		ON CONFLICT (mr_id) DO NOTHING`); err != nil {
		return fmt.Errorf("identity orgs: %w", err)
	}
	if err := s.exec(ctx,
		`UPDATE modrinth.organisations SET last_seen_crawl=`+sqlInt(s.crawlID)+` WHERE mr_id IN (SELECT mr_id FROM pg_temp._raw_org)`); err != nil {
		return err
	}
	if err := s.exec(ctx,
		`INSERT INTO modrinth.app_users (mr_id, created_on, first_seen_crawl, last_seen_crawl)
		 SELECT mr_id, created_on, `+sqlInt(s.crawlID)+`, `+sqlInt(s.crawlID)+`
		 FROM pg_temp._raw_user ON CONFLICT (mr_id) DO NOTHING`); err != nil {
		return fmt.Errorf("identity users: %w", err)
	}
	if err := s.exec(ctx,
		`UPDATE modrinth.app_users SET last_seen_crawl=`+sqlInt(s.crawlID)+` WHERE mr_id IN (SELECT mr_id FROM pg_temp._raw_user)`); err != nil {
		return err
	}
	if err := s.exec(ctx, `
		INSERT INTO modrinth.versions (mr_id, project_id, published_on, first_seen_crawl, last_seen_crawl)
		SELECT v.mr_id, p.project_id, v.published_on, `+sqlInt(s.crawlID)+`, `+sqlInt(s.crawlID)+`
		FROM pg_temp._raw_version v JOIN modrinth.projects p ON p.mr_id = v.project_mr
		ON CONFLICT (mr_id) DO NOTHING`); err != nil {
		return fmt.Errorf("identity versions: %w", err)
	}
	if err := s.exec(ctx,
		`UPDATE modrinth.versions SET last_seen_crawl=`+sqlInt(s.crawlID)+` WHERE mr_id IN (SELECT mr_id FROM pg_temp._raw_version)`); err != nil {
		return err
	}

	idTables := []struct{ name, query string }{
		{"_ids_project", `SELECT mr_id, project_id FROM modrinth.projects WHERE mr_id IN (
		 SELECT mr_id FROM pg_temp._raw_project UNION SELECT target_project_mr FROM pg_temp._raw_version_dep UNION SELECT server_content_project_mr FROM pg_temp._raw_project)`},
		{"_ids_team", `SELECT mr_id, team_id FROM modrinth.teams WHERE mr_id IN (SELECT team_mr FROM pg_temp._raw_project UNION SELECT team_mr FROM pg_temp._raw_team_member UNION SELECT team_mr FROM pg_temp._raw_org)`},
		{"_ids_org", `SELECT mr_id, org_id FROM modrinth.organisations WHERE mr_id IN (SELECT mr_id FROM pg_temp._raw_org UNION SELECT org_mr FROM pg_temp._raw_project)`},
		{"_ids_user", `SELECT mr_id, user_id FROM modrinth.app_users WHERE mr_id IN (SELECT mr_id FROM pg_temp._raw_user UNION SELECT author_mr FROM pg_temp._raw_version UNION SELECT user_mr FROM pg_temp._raw_team_member)`},
		{"_ids_version", `SELECT mr_id, version_id FROM modrinth.versions WHERE mr_id IN (
		 SELECT mr_id FROM pg_temp._raw_version UNION SELECT target_version_mr FROM pg_temp._raw_version_dep UNION SELECT server_content_version_mr FROM pg_temp._raw_project UNION SELECT server_active_version_mr FROM pg_temp._raw_project)`},
	}
	for _, ids := range idTables {
		if err := s.exec(ctx, `CREATE TEMP TABLE `+ids.name+` AS `+ids.query); err != nil {
			return fmt.Errorf("build %s: %w", ids.name, err)
		}
		if err := s.exec(ctx, `CREATE UNIQUE INDEX ON `+stagingTable(ids.name)+` (mr_id)`); err != nil {
			return err
		}
		if err := s.exec(ctx, `ANALYZE `+stagingTable(ids.name)); err != nil {
			return err
		}
	}
	if err := s.exec(ctx, `CREATE TEMP TABLE _observed_team AS SELECT DISTINCT t.team_id FROM pg_temp._raw_team_member m JOIN pg_temp._ids_team t ON t.mr_id = m.team_mr`); err != nil {
		return err
	}
	return nil
}
