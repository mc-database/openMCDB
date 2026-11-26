package modrinth

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"

	"golang.org/x/sync/errgroup"
)

func fanout[T any](ctx context.Context, n int, work []T, fetch func(context.Context, T) error) error {
	if n <= 0 {
		n = 1
	}
	if len(work) == 0 {
		return nil
	}
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(n)
	for _, w := range work {
		if gctx.Err() != nil {
			break
		}
		g.Go(func() (err error) {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("fetch worker panic: %v\n%s", r, debug.Stack())
				}
			}()
			if err := gctx.Err(); err != nil {
				return err
			}
			return fetch(gctx, w)
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}
	return ctx.Err()
}

type searchPages struct {
	offset int
	limit  int
}

func fetchSearchPhase(ctx context.Context, api *client, st *store, maxProjects int) (pids []string, total int, err error) {
	hits, total, err := enumerateSearch(ctx, api, st.cfg.Concurrency, maxProjects)
	pids = make([]string, len(hits))
	for i, h := range hits {
		pids[i] = h.ProjectID
	}
	if err != nil {
		if _, ok := errors.AsType[*searchCoverageError](err); !ok {
			return pids, total, err
		}
	}
	if stageErr := st.stageSearchHits(ctx, hits); stageErr != nil {
		return pids, total, stageErr
	}
	return pids, total, err
}

func enumerateSearch(ctx context.Context, api *client, concurrency, maxProjects int) ([]searchHit, int, error) {
	var res searchPass
	seen := make(map[string]int)
	for range 2 {
		pass, err := fetchSearchPass(ctx, api, concurrency, maxProjects)
		if err != nil {
			return res.hits, res.total, err
		}
		res.total = max(res.total, pass.total)
		res.expected = searchResultLimit(res.total, maxProjects)
		res.duplicates += pass.duplicates
		res.shortPages += pass.shortPages
		res.changedTotalPages += pass.changedTotalPages
		for _, h := range pass.hits {
			if i, ok := seen[h.ProjectID]; ok {
				res.hits[i] = h
				continue
			}
			if maxProjects > 0 && len(res.hits) >= maxProjects {
				continue
			}
			seen[h.ProjectID] = len(res.hits)
			res.hits = append(res.hits, h)
		}
		if res.complete() {
			return res.hits, res.total, nil
		}
	}
	return res.hits, res.total, &searchCoverageError{
		Expected:          res.expected,
		Enumerated:        len(res.hits),
		ReportedTotal:     res.total,
		Duplicates:        res.duplicates,
		ShortPages:        res.shortPages,
		ChangedTotalPages: res.changedTotalPages,
	}
}

type searchCoverageError struct {
	Expected          int
	Enumerated        int
	ReportedTotal     int
	Duplicates        int
	ShortPages        int
	ChangedTotalPages int
}

func (e *searchCoverageError) Error() string {
	return fmt.Sprintf("modrinth: inexact search enumeration after 2 passes: expected=%d enumerated=%d reported_total=%d duplicates=%d short_pages=%d changed_total_pages=%d", e.Expected, e.Enumerated, e.ReportedTotal, e.Duplicates, e.ShortPages, e.ChangedTotalPages)
}

type searchPass struct {
	hits              []searchHit
	total             int
	expected          int
	duplicates        int
	shortPages        int
	changedTotalPages int
}

func (p searchPass) complete() bool {
	return len(p.hits) == p.expected && p.duplicates == 0 && p.shortPages == 0 && p.changedTotalPages == 0
}

func searchResultLimit(total, maxProjects int) int {
	if maxProjects > 0 && maxProjects < total {
		return maxProjects
	}
	return total
}

func fetchSearchPass(ctx context.Context, api *client, concurrency, maxProjects int) (searchPass, error) {
	var out searchPass
	firstLimit := searchPageSize
	if maxProjects > 0 && maxProjects < firstLimit {
		firstLimit = maxProjects
	}
	first, err := api.searchPage(ctx, 0, firstLimit)
	if err != nil {
		return out, fmt.Errorf("search offset 0: %w", err)
	}
	out.total = first.TotalHits
	if err := validateSearchPage(first, firstLimit); err != nil {
		return out, fmt.Errorf("search offset 0: %w", err)
	}
	out.expected = searchResultLimit(out.total, maxProjects)

	work := make([]searchPages, 0)
	for offset := searchPageSize; offset < out.expected; {
		limit := min(searchPageSize, out.expected-offset)
		work = append(work, searchPages{offset: offset, limit: limit})
		offset += limit
	}
	results := make([]searchResults, len(work))
	err = fanout(ctx, concurrency, work, func(gctx context.Context, pg searchPages) error {
		res, err := api.searchPage(gctx, pg.offset, pg.limit)
		if err != nil {
			return fmt.Errorf("search offset %d: %w", pg.offset, err)
		}
		if err := validateSearchPage(res, pg.limit); err != nil {
			return fmt.Errorf("search offset %d: %w", pg.offset, err)
		}
		results[pg.offset/searchPageSize-1] = res
		return nil
	})

	seen := make(map[string]bool)
	collect := func(res searchResults, wanted int) {
		if len(res.Hits) < wanted {
			out.shortPages++
		}
		if res.TotalHits != out.total {
			out.changedTotalPages++
		}
		for _, h := range res.Hits {
			if seen[h.ProjectID] {
				out.duplicates++
				continue
			}
			seen[h.ProjectID] = true
			out.hits = append(out.hits, h)
		}
	}
	collect(first, min(firstLimit, out.expected))
	for i, res := range results {
		collect(res, work[i].limit)
	}
	return out, err
}

func validateSearchPage(res searchResults, limit int) error {
	if res.TotalHits < 0 {
		return fmt.Errorf("invalid negative total_hits: %d", res.TotalHits)
	}
	if len(res.Hits) > limit {
		return fmt.Errorf("returned %d hits for limit %d", len(res.Hits), limit)
	}
	for _, h := range res.Hits {
		if h.ProjectID == "" {
			return fmt.Errorf("search hit has an empty project_id")
		}
	}
	return nil
}

func fanoutBatches[T any](ctx context.Context, st *store, ids []string,
	fetch func(context.Context, []string) (T, error),
	stage func(context.Context, T) error) error {
	batches := chunk(ids, batchSize)
	return fanout(ctx, st.cfg.Concurrency, batches, func(gctx context.Context, b []string) error {
		result, err := fetch(gctx, b)
		if err != nil {
			return err
		}
		st.stageMu.Lock()
		defer st.stageMu.Unlock()
		return stage(gctx, result)
	})
}
