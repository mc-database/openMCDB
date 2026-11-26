package modrinth

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"golang.org/x/sync/errgroup"
)

type phaseTimer struct {
	names []string
	durs  []time.Duration
}

func newPhaseTimer() *phaseTimer { return &phaseTimer{} }

func (p *phaseTimer) run(name string, work func() error) error {
	start := time.Now()
	defer func() {
		p.names = append(p.names, name)
		p.durs = append(p.durs, time.Since(start))
	}()
	return work()
}

func (p *phaseTimer) String() string {
	var b strings.Builder
	for i, n := range p.names {
		if i > 0 {
			b.WriteString(", ")
		}
		_, _ = fmt.Fprintf(&b, "%s=%s", n, p.durs[i].Round(time.Second))
	}
	return b.String()
}

func RunOnce(ctx context.Context, db *sqlx.DB, cfg CrawlConfig) (err error) {
	cfg, err = NormalizeCrawlConfig(cfg)
	if err != nil {
		return err
	}
	api := newClient(cfg)
	st := newStore(db, cfg)
	pt := newPhaseTimer()
	defer func() {
		log.Printf("openmcdb/modrinth: phase times: %s", pt.String())
		logRequestStats(api)
		for _, kind := range []string{"versions_unavailable", "dependencies_without_target"} {
			if count := st.coverage[kind]; count > 0 {
				log.Printf("openmcdb/modrinth: %s=%d (example version IDs: %s)", kind, count, idPreview(st.samples[kind]))
			}
		}
		if count := st.coverage["environments_missing"]; count > 0 {
			log.Printf("openmcdb/modrinth: %d versions have missing environments; stored as unknown", count)
		}
		if count := st.coverage["unmapped:environment"]; count > 0 {
			log.Printf("openmcdb/modrinth: %d versions have unsupported environments; stored as unknown", count)
		}
	}()

	if err := st.begin(ctx); err != nil {
		return err
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("crawl panic: %v\n%s", r, debug.Stack())
		}
		recordCrawlStats(st.coverage, api, pt)
		if ferr := st.finish(ctx, err); ferr != nil {
			if err == nil {
				err = ferr
			} else {
				err = fmt.Errorf("%w (and: %v)", err, ferr)
			}
		}
	}()

	for _, phase := range []struct {
		name string
		work func() error
	}{
		{"vocabulary", func() error { return loadVocabulary(ctx, api, st) }},
		{"projects", func() error { return crawlProjects(ctx, api, st) }},
		{"versions", func() error { return crawlVersions(ctx, api, st) }},
		{"organizations", func() error { return crawlOrganizations(ctx, api, st) }},
		{"teams", func() error { return crawlTeams(ctx, api, st) }},
		{"users", func() error { return crawlUsers(ctx, api, st) }},
	} {
		if err := pt.run(phase.name, phase.work); err != nil {
			return err
		}
	}
	var platform platformSnapshot
	if err := pt.run("platform_fetch", func() error {
		var err error
		platform, err = fetchPlatform(ctx, api)
		return err
	}); err != nil {
		return err
	}
	if err := pt.run("apply", func() error {
		if err := st.beginTx(ctx); err != nil {
			return err
		}
		if err := st.applyAPIVocabulary(ctx); err != nil {
			return err
		}
		if err := st.applyVocabulary(ctx); err != nil {
			return err
		}
		if err := st.applyIdentities(ctx); err != nil {
			return err
		}
		if err := st.stageSnapshots(ctx); err != nil {
			return err
		}
		if err := st.applyAll(ctx); err != nil {
			return err
		}
		return savePlatform(ctx, st, platform)
	}); err != nil {
		return err
	}
	recordCrawlStats(st.coverage, api, pt)
	return pt.run("commit", func() error { return st.commit(ctx) })
}

func recordCrawlStats(coverage map[string]int, api *client, pt *phaseTimer) {
	paths, attempts := api.RequestStats()
	coverage["http_attempts"] = int(attempts)
	coverage["http_429"] = int(api.RateLimitCount())
	for path, count := range paths {
		coverage["requests:"+path] = int(count)
	}
	for i, name := range pt.names {
		coverage["phase_ms:"+name] = int(pt.durs[i].Milliseconds())
	}
}

func logRequestStats(api *client) {
	if paths, attempts := api.RequestStats(); len(paths) > 0 {
		type kv struct {
			path string
			n    int64
		}
		rows := make([]kv, 0, len(paths))
		for p, n := range paths {
			rows = append(rows, kv{p, n})
		}
		slices.SortFunc(rows, func(a, b kv) int {
			if a.n != b.n {
				return -cmp.Compare(a.n, b.n)
			}
			return cmp.Compare(a.path, b.path)
		})
		var b strings.Builder
		_, _ = fmt.Fprintf(&b, "request counts: ")
		for i, r := range rows {
			if i > 0 {
				b.WriteString(", ")
			}
			_, _ = fmt.Fprintf(&b, "%s=%d", r.path, r.n)
		}
		log.Printf("openmcdb/modrinth: %s (http attempts incl. retries: %d, 429 responses: %d)", b.String(), attempts, api.RateLimitCount())
	}
}

func optStr(v *string, fallback string) string {
	if v == nil {
		return fallback
	}
	return *v
}

func nullableString(v *string) any {
	if v == nil || *v == "" {
		return nil
	}
	return *v
}

func tTime(t apiTime) any { return t.Time }

func chunk(ids []string, size int) [][]string {
	if size <= 0 {
		size = 100
	}
	out := make([][]string, 0, (len(ids)+size-1)/size)
	for i := 0; i < len(ids); i += size {
		end := min(i+size, len(ids))
		out = append(out, ids[i:end])
	}
	return out
}

func loadVocabulary(ctx context.Context, api *client, st *store) error {
	var licenses []apiLicenseRef
	var loaders []apiLoader
	var gvs []apiGameVersion
	var cats []apiCategory
	var platforms []apiLinkPlatform
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		var err error
		licenses, err = api.licenses(gctx)
		if err != nil {
			return fmt.Errorf("licenses: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		loaders, err = api.loaders(gctx)
		if err != nil {
			return fmt.Errorf("loaders: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		gvs, err = api.gameVersions(gctx)
		if err != nil {
			return fmt.Errorf("game_versions: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		cats, err = api.categories(gctx)
		if err != nil {
			return fmt.Errorf("categories: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		platforms, err = api.linkPlatforms(gctx)
		if err != nil {
			return fmt.Errorf("link_platforms: %w", err)
		}
		return nil
	})
	if err := g.Wait(); err != nil {
		return err
	}

	lrows := make([][]any, len(licenses))
	for i, l := range licenses {
		lrows[i] = []any{l.Short, l.Name}
	}
	if err := st.bulk(ctx, "_vocab_license", []string{"spdx", "name"}, lrows, 1000); err != nil {
		return err
	}

	lrows = make([][]any, len(loaders))
	for i, l := range loaders {
		mapped := make([]string, 0, len(l.SupportedProjectTypes))
		for _, pt := range l.SupportedProjectTypes {
			mapped = append(mapped, mapEnum(st.coverage, "projectType", pt, projectType))
		}
		lrows[i] = []any{l.Name, mapped}
	}
	if err := st.bulk(ctx, "_vocab_loader", []string{"code", "types"}, lrows, 1000); err != nil {
		return err
	}

	gvRows := make([][]any, len(gvs))
	for i, g := range gvs {
		gvRows[i] = []any{g.Value, tTime(g.Created), mapEnum(st.coverage, "gameVersionType", optStr(g.Type, ""), gameVersionType), boolPtrDeref(g.Major, false)}
	}
	if err := st.bulk(ctx, "_vocab_gv", []string{"label", "released_at", "vtype", "major"}, gvRows, 1000); err != nil {
		return err
	}

	catRows := make([][]any, len(cats))
	for i, ct := range cats {
		catRows[i] = []any{ct.Name, mapEnum(st.coverage, "projectType", ct.ProjectType, projectType), ct.Header}
	}
	if err := st.bulk(ctx, "_vocab_cat", []string{"name", "ptype", "header"}, catRows, 1000); err != nil {
		return err
	}

	platformRows := make([][]any, len(platforms))
	for i, p := range platforms {
		platformRows[i] = []any{p.Name, p.Donation}
	}
	return st.bulk(ctx, "_vocab_plat", []string{"name", "donation"}, platformRows, 1000)
}

func crawlProjects(ctx context.Context, api *client, st *store) error {
	pids, total, err := fetchSearchPhase(ctx, api, st, st.cfg.MaxProjects)
	st.coverage["search_total"] = total
	st.coverage["search_expected"] = searchResultLimit(total, st.cfg.MaxProjects)
	st.coverage["search_enumerated"] = len(pids)
	if err != nil {
		var coverage *searchCoverageError
		if !errors.As(err, &coverage) {
			return err
		}
		st.coverage["search_inexact"] = 1
		st.coverage["search_shortfall"] = max(0, coverage.Expected-coverage.Enumerated)
		st.coverage["search_duplicates"] = coverage.Duplicates
		st.coverage["search_short_pages"] = coverage.ShortPages
		st.coverage["search_changed_total_pages"] = coverage.ChangedTotalPages
		log.Printf("openmcdb/modrinth: continuing with observed projects: %v; unobserved projects remain untouched", err)
	}
	type projectBatch struct {
		rows    []apiProject
		missing []string
	}
	fetch := func(gctx context.Context, ids []string) (projectBatch, error) {
		rows, err := api.projectsBatch(gctx, ids)
		result := projectBatch{rows: rows}
		if err != nil {
			var coverage *batchCoverageError
			if !errors.As(err, &coverage) || !coverage.onlyMissing() {
				return result, err
			}
			result.missing = coverage.missing
		}
		return result, nil
	}
	if err := fanoutBatches(ctx, st, pids, fetch, func(gctx context.Context, batch projectBatch) error {
		if len(batch.missing) > 0 {
			st.coverage["projects_unavailable"] += len(batch.missing)
			log.Printf("openmcdb/modrinth: skipping projects unavailable from details API: %s; existing project data remains untouched", idPreview(batch.missing))
		}
		return st.stageProjectDetails(gctx, batch.rows)
	}); err != nil {
		return fmt.Errorf("projects: %w", err)
	}
	log.Printf("openmcdb/modrinth: staged %d/%d enumerated projects, skipped %d unsupported servers (search total=%d, cap=%d, facets=%q)", st.coverage["projects"], len(pids), st.coverage["servers_skipped_unsupported"], total, st.cfg.MaxProjects, st.cfg.ScopeFacet)
	return nil
}

func (s *store) stageSearchHits(ctx context.Context, hits []searchHit) error {
	rows := make([][]any, 0, len(hits))
	for _, h := range hits {
		for _, pt := range h.ProjectTypes {
			mapEnum(s.coverage, "projectType", pt, projectType)
		}
		for _, pt := range h.AllProjectTypes {
			mapEnum(s.coverage, "projectType", pt, projectType)
		}
		kinds := make([]string, 0, len(h.DisclosureTypes))
		for _, d := range h.DisclosureTypes {
			if k, ok := mapOptionalValue(s.coverage, "disclosureKind", d, disclosureKind); ok {
				kinds = append(kinds, k)
			}
		}
		rows = append(rows, []any{h.ProjectID, kinds})
	}
	return s.bulk(ctx, "_search_disclosure", []string{"mr_id", "kinds"}, rows, 1000)
}

func (s *store) stageProjectDetails(ctx context.Context, details []apiProject) error {
	cols := []string{
		"mr_id", "team_mr", "org_mr", "slug", "name", "status", "requested_status",
		"monetization", "gallery_count", "color", "has_icon", "license_spdx", "license_name",
		"license_url", "side_types", "queued_on", "published_on", "approved_on",
		"downloads", "followers", "project_types", "category_namespaces", "categories", "addl_categories",
		"link_platforms", "link_donations", "link_urls",
		"has_server",
		"server_address", "server_content_kind", "server_content_version_mr",
		"server_content_project_mr", "server_rec_gv", "server_country", "server_region",
		"server_languages", "server_active_version_mr", "server_max_players", "server_gvs",
		"server_v2w", "server_v4w", "server_ping_online", "server_ping_max",
		"server_ping_proto", "server_ping_latency_ms", "server_pinged_at",
	}
	rows := make([][]any, 0, len(details))
	s.coverage["projects_fetched"] += len(details)
	for _, p := range details {
		status := mapEnum(s.coverage, "projectStatus", p.Status, projectStatus)
		monetizationStatus := mapEnum(s.coverage, "monetization", p.Monetization, monetization)
		reviewStatus := mapEnum(s.coverage, "sideTypesReviewStatus", p.SideTypes, sideTypesReviewStatus)
		requestedStatus := ""
		if p.ReqStatus != nil && *p.ReqStatus != "" {
			requestedStatus = mapEnum(s.coverage, "projectStatus", *p.ReqStatus, projectStatus)
		}
		mappedTypes := make([]string, 0, len(p.ProjectTypes))
		for _, pt := range p.ProjectTypes {
			mappedTypes = append(mappedTypes, mapEnum(s.coverage, "projectType", pt, projectType))
		}
		sc, err := serverFromComponentsWithCoverage(p.MinecraftJavaServer, p.MinecraftServer, s.coverage)
		if errors.Is(err, errUnsupportedServer) {
			s.coverage["projects_skipped_unsupported_server"]++
			s.coverage["servers_skipped_unsupported"]++
			continue
		}
		if err != nil {
			return fmt.Errorf("project %s server components: %w", p.ID, err)
		}
		for _, v := range p.Versions {
			s.remember(s.versionIDs, v)
		}
		s.remember(s.teamIDs, p.TeamID)
		s.remember(s.orgIDs, optStr(p.Org, ""))

		linkPlatforms := make([]string, 0, len(p.LinkURLs))
		linkDonations := make([]bool, 0, len(p.LinkURLs))
		linkURLs := make([]string, 0, len(p.LinkURLs))
		for name, l := range p.LinkURLs {
			linkPlatforms = append(linkPlatforms, name)
			linkDonations = append(linkDonations, l.Donation)
			linkURLs = append(linkURLs, l.URL)
		}

		hasServer := sc != nil
		if hasServer {
			s.coverage["servers"]++
		}
		if sc == nil {
			sc = &serverStateRow{}
		}

		rows = append(rows, []any{
			p.ID, p.TeamID, optStr(p.Org, ""), optStr(p.Slug, ""), p.Name,
			status, requestedStatus,
			monetizationStatus, len(p.Gallery), intPtr(p.Color), p.IconURL != nil,
			p.License.ID, p.License.Name, optStr(p.License.URL, ""),
			reviewStatus, tTimePtr(p.Queued), tTime(p.Published), tTimePtr(p.Approved),
			p.Downloads, p.Followers, mappedTypes, categoryNamespaces(mappedTypes), p.Categories, p.AddlCats,
			linkPlatforms, linkDonations, linkURLs,
			hasServer,
			sc.address, sc.contentKind, sc.contentVersionMR, sc.contentProjectMR, sc.recGV,
			sc.country, sc.region, sc.languages, sc.activeVersionMR, intPtr(sc.maxPlayers), sc.gvs,
			sc.v2w, sc.v4w, sc.pingOnline, sc.pingMax, sc.pingProto, sc.pingLatencyMs, sc.pingedAt,
		})
	}
	if err := s.bulk(ctx, "_raw_project", cols, rows, 500); err != nil {
		return err
	}
	s.coverage["projects"] += len(rows)
	return nil
}

type serverStateRow struct {
	address          string
	contentKind      string
	contentVersionMR string
	contentProjectMR string
	recGV            *string
	country          *string
	region           *string
	languages        []string
	activeVersionMR  *string
	maxPlayers       *int
	gvs              []string
	v2w              *int64
	v4w              *int64
	pingOnline       *int
	pingMax          *int
	pingProto        *int
	pingLatencyMs    *int
	pingedAt         any
}

var errUnsupportedServer = errors.New("minecraft_server without minecraft_java_server is unsupported by Java-only ingestion")

func serverFromComponentsWithCoverage(js *javaServerProjectComponent, shared *serverProjectComponent, coverage map[string]int) (*serverStateRow, error) {
	var region *string
	if shared != nil && shared.Region != nil {
		r, ok := mapOptionalValue(coverage, "serverRegion", *shared.Region, serverRegion)
		if !ok {
			return nil, fmt.Errorf("invalid minecraft_server region %q", *shared.Region)
		}
		region = &r
	}
	if js == nil {
		if shared != nil {
			return nil, errUnsupportedServer
		}
		return nil, nil
	}
	if shared == nil {
		return nil, fmt.Errorf("minecraft_java_server requires a minecraft_server component")
	}
	kind, err := serverContentKind(js.Content.Kind)
	if err != nil {
		recordCoverageValue(coverage, "unmapped", "serverContentKind", js.Content.Kind, 1)
		return nil, err
	}
	row := &serverStateRow{address: js.Address, contentKind: kind}
	switch kind {
	case "vanilla":
		if js.Content.VersionID != nil || js.Content.ProjectID != nil {
			return nil, fmt.Errorf("vanilla server content must not contain modpack version_id or project_id")
		}
		row.recGV = js.Content.RecommendedGameVersion
		row.gvs = slices.Clone(js.Content.SupportedGameVersions)
	case "modpack":
		if js.Content.VersionID == nil || strings.TrimSpace(*js.Content.VersionID) == "" {
			return nil, fmt.Errorf("modpack server content requires a non-empty version_id")
		}
		if js.Content.RecommendedGameVersion != nil || js.Content.SupportedGameVersions != nil {
			return nil, fmt.Errorf("modpack server content must not contain vanilla game version fields")
		}
		row.contentVersionMR = *js.Content.VersionID
		row.contentProjectMR = optStr(js.Content.ProjectID, "")
	}
	if js.VerifiedPlays2w != nil {
		v, err := signedCounter(coverage, "verifiedPlays2w", *js.VerifiedPlays2w)
		if err != nil {
			return nil, err
		}
		row.v2w = &v
	}
	if js.VerifiedPlays4w != nil {
		v, err := signedCounter(coverage, "verifiedPlays4w", *js.VerifiedPlays4w)
		if err != nil {
			return nil, err
		}
		row.v4w = &v
	}
	if js.Ping != nil {
		if js.Ping.When.Valid() {
			row.pingedAt = js.Ping.When.Time
		}
		if js.Ping.Data != nil {
			lat := int(js.Ping.Data.Latency.Duration().Milliseconds())
			online := js.Ping.Data.PlayersOnline
			pmax := js.Ping.Data.PlayersMax
			proto := js.Ping.Data.VersionProtocol
			row.pingOnline = &online
			row.pingMax = &pmax
			row.pingProto = &proto
			row.pingLatencyMs = &lat
		}
	}
	row.country = shared.Country
	row.region = region
	row.languages = make([]string, len(shared.Languages))
	copy(row.languages, shared.Languages)
	slices.Sort(row.languages)
	row.activeVersionMR = shared.ActiveVersion
	row.maxPlayers = shared.MaxPlayers
	return row, nil
}

func signedCounter(coverage map[string]int, field string, raw uint64) (int64, error) {
	if raw > math.MaxInt64 {
		recordCoverageValue(coverage, "invalid", field, strconv.FormatUint(raw, 10), 1)
		return 0, fmt.Errorf("%s value %d exceeds bigint capacity", field, raw)
	}
	return int64(raw), nil
}

func intPtr(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func tTimePtr(t *apiTime) any {
	if t == nil || !t.Valid() {
		return nil
	}
	return t.Time
}

func serverContentKind(s string) (string, error) {
	switch s {
	case "modpack", "vanilla":
		return s, nil
	default:
		return "", fmt.Errorf("invalid minecraft_java_server content kind %q", s)
	}
}

func crawlVersions(ctx context.Context, api *client, st *store) error {
	ids := st.keys(st.versionIDs)
	st.coverage["versions_requested"] = len(ids)
	type versionBatch struct {
		rows    []apiVersion
		missing []string
	}
	fetch := func(gctx context.Context, batch []string) (versionBatch, error) {
		rows, err := api.versionsBatch(gctx, batch)
		result := versionBatch{rows: rows}
		if err != nil {
			var coverage *batchCoverageError
			ok := errors.As(err, &coverage)
			if !ok || !coverage.onlyMissing() {
				return result, err
			}
			result.missing = coverage.missing
		}
		return result, nil
	}
	return fanoutBatches(ctx, st, ids, fetch,
		func(gctx context.Context, batch versionBatch) error {
			if len(batch.missing) > 0 {
				st.coverage["versions_unavailable"] += len(batch.missing)
				st.sampleIDs("versions_unavailable", batch.missing)
			}
			return st.stageVersionDetails(gctx, batch.rows)
		})
}

func (s *store) stageVersionDetails(ctx context.Context, details []apiVersion) error {
	s.coverage["versions"] += len(details)
	fileRows := make([][]any, 0, 1024)
	depRows := make([][]any, 0, 1024)
	vRows := make([][]any, 0, 1024)
	flush := func() error {
		if len(vRows) > 0 {
			if err := s.bulk(ctx, "_raw_version", versionCols(), vRows, 1000); err != nil {
				return err
			}
			vRows = nil
		}
		if len(fileRows) > 0 {
			if err := s.bulk(ctx, "_raw_version_file", []string{"file_id", "version_mr", "sha1", "filename", "size", "primary_file"}, fileRows, 1000); err != nil {
				return err
			}
			fileRows = nil
		}
		if len(depRows) > 0 {
			if err := s.bulk(ctx, "_raw_version_dep", []string{"version_mr", "seq", "dep_kind", "target_project_mr", "target_version_mr", "file_name"}, depRows, 1000); err != nil {
				return err
			}
			depRows = nil
		}
		return nil
	}

	for _, v := range details {
		s.remember(s.userIDs, v.AuthorID)
		mappedTypes := make([]string, 0, len(v.ProjectTypes))
		for _, pt := range v.ProjectTypes {
			mappedTypes = append(mappedTypes, mapEnum(s.coverage, "projectType", pt, projectType))
		}
		games := make([]string, 0, len(v.Games))
		for _, g := range v.Games {
			if gg, ok := mapOptionalValue(s.coverage, "game", g, game); ok {
				games = append(games, gg)
			}
		}
		vRows = append(vRows, []any{
			v.ID, v.ProjectID, v.AuthorID, optStr(v.Name, ""), v.VersionNumber,
			mapEnum(s.coverage, "versionType", v.VersionType, versionType), mapEnum(s.coverage, "versionStatus", v.Status, versionStatus), v.Featured,
			mapEnum(s.coverage, "environment", optStr(v.Environment, ""), environment),
			v.Loaders, games, mappedTypes, v.GameVersions,
			v.Downloads, tTime(v.DatePublished),
		})
		for _, f := range v.Files {
			fileRows = append(fileRows, []any{f.ID, v.ID, f.Hashes["sha1"], f.Filename, f.Size, f.Primary})
		}
		deps := make([]apiDependency, len(v.Dependencies))
		copy(deps, v.Dependencies)
		slices.SortStableFunc(deps, func(a, b apiDependency) int {
			ka := depKind(a.Type) + "\x00" + optStr(a.ProjectID, "") + "\x00" + optStr(a.VersionID, "") + "\x00" + optStr(a.FileName, "")
			kb := depKind(b.Type) + "\x00" + optStr(b.ProjectID, "") + "\x00" + optStr(b.VersionID, "") + "\x00" + optStr(b.FileName, "")
			return strings.Compare(ka, kb)
		})
		if len(deps) > 32768 {
			return fmt.Errorf("version %s has %d dependencies: exceeds smallint sequence capacity", v.ID, len(deps))
		}
		seq := 0
		var targetless bool
		for _, d := range deps {
			kind := mapEnum(s.coverage, "depKind", d.Type, depKind)
			projectID := nullableString(d.ProjectID)
			versionID := nullableString(d.VersionID)
			fileName := nullableString(d.FileName)
			if projectID == nil && versionID == nil && fileName == nil {
				s.coverage["dependencies_without_target"]++
				targetless = true
				continue
			}
			depRows = append(depRows, []any{
				v.ID, int16(seq), kind, projectID, versionID, fileName,
			})
			seq++
		}
		if targetless {
			s.coverage["versions_with_dependencies_without_target"]++
			s.sampleIDs("dependencies_without_target", []string{v.ID})
		}
		if len(fileRows) > 5000 {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	return flush()
}

func versionCols() []string {
	return []string{
		"mr_id", "project_mr", "author_mr", "name", "version_number", "version_type",
		"status", "featured", "environment", "loaders", "games", "project_types",
		"game_versions", "downloads", "published_on",
	}
}

func crawlTeams(ctx context.Context, api *client, st *store) error {
	ids := st.keys(st.teamIDs)
	st.coverage["teams_requested"] = len(ids)
	observed := make(map[string]bool, len(ids))
	stage := func(gctx context.Context, teams [][]apiTeamMember) error {
		rows := make([][]any, 0, 256)
		for _, members := range teams {
			if len(members) == 0 {
				st.coverage["teams_anonymous_groups"]++
				continue
			}
			teamID := members[0].TeamID
			if observed[teamID] {
				continue
			}
			observed[teamID] = true
			st.coverage["teams_observed"]++
			for _, m := range members {
				st.remember(st.userIDs, m.User.ID)
				st.embeddedUsers[m.User.ID] = m.User
				rows = append(rows, []any{m.TeamID, m.User.ID, m.Role, m.IsOwner})
			}
		}
		return st.bulk(gctx, "_raw_team_member", []string{"team_mr", "user_mr", "role", "is_owner"}, rows, 1000)
	}
	err := fanoutBatches(ctx, st, ids, api.teamsBatch, stage)
	st.coverage["teams_recovered"] = 0
	if err == nil {
		missing := make([]string, 0, len(ids)-len(observed))
		for _, id := range ids {
			if !observed[id] {
				missing = append(missing, id)
			}
		}
		st.coverage["teams_retried"] = len(missing)
		before := len(observed)
		err = fanout(ctx, st.cfg.Concurrency, chunk(missing, batchSize/10), func(gctx context.Context, batch []string) error {
			teams, err := api.teamsBatch(gctx, batch)
			if err != nil {
				return err
			}
			st.stageMu.Lock()
			defer st.stageMu.Unlock()
			return stage(gctx, teams)
		})
		st.coverage["teams_recovered"] = len(observed) - before
	}
	st.coverage["teams_unobserved"] = len(ids) - len(observed)
	if st.coverage["teams_retried"] > 0 {
		log.Printf("openmcdb/modrinth: retried %d teams, recovered %d; %d/%d remain unobserved; prior membership observations remain untouched", st.coverage["teams_retried"], st.coverage["teams_recovered"], st.coverage["teams_unobserved"], len(ids))
	}
	return err
}

func crawlOrganizations(ctx context.Context, api *client, st *store) error {
	ids := st.keys(st.orgIDs)
	st.coverage["organizations_requested"] = len(ids)
	return fanoutBatches(ctx, st, ids, api.orgsBatch,
		func(gctx context.Context, orgs []apiOrganization) error {
			rows := make([][]any, 0, len(orgs))
			st.coverage["organizations"] += len(orgs)
			for _, o := range orgs {
				for _, m := range o.Members {
					st.remember(st.userIDs, m.User.ID)
					st.embeddedUsers[m.User.ID] = m.User
				}
				st.remember(st.teamIDs, o.TeamID)
				rows = append(rows, []any{o.ID, o.TeamID, o.Slug, o.Name, intPtr(o.Color)})
			}
			return st.bulk(gctx, "_raw_org", []string{"mr_id", "team_mr", "slug", "name", "color"}, rows, 1000)
		})
}

func crawlUsers(ctx context.Context, api *client, st *store) error {
	ids := st.keys(st.userIDs)
	st.coverage["users_requested"] = len(ids)
	var fetchIDs []string
	var embedded []apiUser
	for _, id := range ids {
		if user, ok := st.embeddedUsers[id]; ok {
			embedded = append(embedded, user)
		} else {
			fetchIDs = append(fetchIDs, id)
		}
	}
	stage := func(gctx context.Context, users []apiUser) error {
		rows := make([][]any, 0, len(users))
		for _, u := range users {
			badges, err := signedCounter(st.coverage, "userBadges", u.Badges)
			if err != nil {
				return fmt.Errorf("user %s: %w", u.ID, err)
			}
			rows = append(rows, []any{u.ID, u.Username, mapEnum(st.coverage, "userRole", u.Role, userRole), badges, tTime(u.Created)})
		}
		st.coverage["users"] += len(users)
		return st.bulk(gctx, "_raw_user", []string{"mr_id", "username", "role", "badges", "created_on"}, rows, 1000)
	}
	if err := stage(ctx, embedded); err != nil {
		return err
	}
	st.coverage["users_embedded"] = len(embedded)
	return fanoutBatches(ctx, st, fetchIDs, api.usersBatch, stage)
}

type platformSnapshot struct {
	stats   apiStatistics
	revenue apiRevenue
}

func fetchPlatform(ctx context.Context, api *client) (platformSnapshot, error) {
	stats, err := api.statistics(ctx)
	if err != nil {
		return platformSnapshot{}, fmt.Errorf("statistics: %w", err)
	}
	revenue, err := api.platformRevenue(ctx)
	if err != nil {
		return platformSnapshot{}, fmt.Errorf("platform revenue: %w", err)
	}
	return platformSnapshot{stats: stats, revenue: revenue}, nil
}

func savePlatform(ctx context.Context, st *store, platform platformSnapshot) error {
	if err := st.exec(ctx, `
		INSERT INTO modrinth.platform_statistics (crawl_id, projects, versions, authors, files,
			revenue_all_time, revenue_all_time_available)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		st.crawlID, i64(platform.stats.Projects), i64(platform.stats.Versions), i64(platform.stats.Authors), i64(platform.stats.Files),
		dec(platform.revenue.AllTime), dec(platform.revenue.AllTimeAvailable)); err != nil {
		return fmt.Errorf("platform statistics: %w", err)
	}
	rows := make([][]any, 0, len(platform.revenue.Data))
	for _, d := range platform.revenue.Data {
		var t time.Time
		if d.Time > 0 {
			t = time.Unix(d.Time, 0).UTC()
		}
		rows = append(rows, []any{st.crawlID, t, dec(d.CreatorRevenue), dec(d.Revenue)})
	}
	return st.bulk(ctx, "modrinth.platform_revenues",
		[]string{"crawl_id", "time", "creator_revenue_usd", "modrinth_revenue_usd"}, rows, 1000)
}

func i64(v *int64) int64 {
	if v == nil {
		return 0
	}
	return *v
}

func dec(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "0"
	}
	return s
}
