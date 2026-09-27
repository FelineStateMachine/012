-- The latest run against the last release's recorded runs, with a
-- threshold that allows for noise. Run by scripts/stress/report.sh after
-- load.sql, with variables threshold, min_delta_ns and release_tags, a
-- list of 'tag commit' strings, newest release first (from git), or a
-- single run id in release_run.
--
-- The release's runs are every clean run of the newest tagged commit that
-- has one on the latest run's CPU and OS, other than the latest run
-- itself; its samples are pooled. A benchmark regresses when its median
-- is slower than the release's by more than threshold + noise and by more
-- than min_delta_ns, where noise is the larger spread,
-- (max - min) / median, of its samples in either side.

CREATE OR REPLACE TEMP TABLE release_tags AS
SELECT split_part(t, ' ', 1) AS tag, split_part(t, ' ', 2) AS commit, i AS ord
FROM (SELECT unnest(getvariable('release_tags')) AS t, generate_subscripts(getvariable('release_tags'), 1) AS i)
WHERE t <> '';

CREATE OR REPLACE TEMP TABLE release_pick AS
WITH latest AS (SELECT * FROM runs WHERE age = 1),
cand AS (
    SELECT t.tag, t.ord, r.run_id
    FROM release_tags t
    JOIN runs r ON starts_with(t.commit, r.git_sha) AND length(r.git_sha) >= 7
    JOIN latest l ON r.cpu = l.cpu AND r.goos = l.goos
    WHERE NOT r.dirty AND r.run_id <> l.run_id AND r.git_sha <> l.git_sha
),
tag AS (
    SELECT tag FROM cand ORDER BY ord LIMIT 1
)
SELECT run_id, tag FROM cand WHERE getvariable('release_run') = '' AND tag = (SELECT tag FROM tag)
UNION ALL
SELECT run_id, run_id AS tag FROM runs WHERE run_id = getvariable('release_run');

CREATE OR REPLACE TEMP TABLE release_compared AS
WITH l AS (SELECT r.* FROM bench r JOIN runs u USING (run_id) WHERE u.age = 1),
rel AS (
    SELECT r.pkg, r.name, count(*) AS samples, median(r.ns_op) AS ns_op,
           CASE WHEN median(r.ns_op) > 0 THEN (max(r.ns_op) - min(r.ns_op)) / median(r.ns_op) ELSE 0 END AS spread
    FROM results r JOIN release_pick p USING (run_id)
    GROUP BY r.pkg, r.name
)
SELECT l.pkg, l.name, rel.ns_op AS before_ns, l.ns_op AS after_ns,
       (l.ns_op - rel.ns_op) / rel.ns_op AS change,
       greatest(l.spread, rel.spread) AS noise,
       rel.samples AS release_samples, l.samples AS latest_samples,
       CASE
           WHEN (l.ns_op - rel.ns_op) / rel.ns_op > getvariable('threshold') + greatest(l.spread, rel.spread)
                AND l.ns_op - rel.ns_op > getvariable('min_delta_ns') THEN 'REGRESSION'
           WHEN (rel.ns_op - l.ns_op) / rel.ns_op > getvariable('threshold') + greatest(l.spread, rel.spread)
                AND rel.ns_op - l.ns_op > getvariable('min_delta_ns') THEN 'faster'
           ELSE ''
       END AS flag
FROM l JOIN rel USING (pkg, name);

.print
.print # Latest against the last release (threshold plus each benchmark's noise)
SELECT (SELECT any_value(tag) FROM release_pick) AS release,
       (SELECT count(*) FROM release_pick) AS release_runs,
       count(*) AS compared,
       count(*) FILTER (WHERE flag = 'REGRESSION') AS regressions,
       count(*) FILTER (WHERE flag = 'faster') AS faster,
       printf('%.0f%%', getvariable('threshold') * 100) AS threshold
FROM release_compared;

SELECT pkg, name, fmt_ns(before_ns) AS release, fmt_ns(after_ns) AS latest,
       printf('%+.1f%%', change * 100) AS change,
       printf('%.1f%%', noise * 100) AS noise,
       release_samples || '/' || latest_samples AS samples, flag
FROM release_compared WHERE flag <> ''
ORDER BY flag DESC, change DESC;
