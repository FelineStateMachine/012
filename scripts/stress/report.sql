-- Stress report: the latest run against the previous run and a baseline
-- on the same CPU, regressions flagged, and trends of the headline
-- benchmarks. Run by scripts/stress/report.sh after load.sql, with
-- variables threshold (fraction, e.g. 0.10), min_delta_ns (smaller
-- changes are noise) and baseline (a run_id, or '' for the oldest run that
-- covered most of the same benchmarks).

.mode box
.maxrows 400

.print
.print # Runs
SELECT age, run_id, strftime(ts, '%Y-%m-%d %H:%M') AS ts, git_sha, dirty, cpu, go_version, benchmarks
FROM runs ORDER BY age LIMIT 10;

-- The runs compared: the latest, the one before it on the same CPU and
-- OS, and the baseline.
CREATE OR REPLACE TEMP TABLE picked AS
WITH latest AS (SELECT * FROM runs WHERE age = 1),
same AS (SELECT r.* FROM runs r, latest l WHERE r.cpu = l.cpu AND r.goos = l.goos)
SELECT
    (SELECT run_id FROM latest) AS latest,
    (SELECT run_id FROM same WHERE age > 1 ORDER BY age LIMIT 1) AS previous,
    -- By default the oldest run that covered most of what the latest did.
    coalesce(nullif(getvariable('baseline'), ''),
        (SELECT run_id FROM same WHERE benchmarks >= 0.9 * (SELECT benchmarks FROM latest) ORDER BY age DESC LIMIT 1)) AS baseline;

CREATE OR REPLACE TEMP TABLE compared AS
WITH l AS (SELECT r.* FROM bench r, picked p WHERE r.run_id = p.latest)
SELECT l.pkg, l.name, kind, o.ns_op AS before_ns, l.ns_op AS after_ns,
       (l.ns_op - o.ns_op) / o.ns_op AS change,
       l.allocs_op - o.allocs_op AS allocs_change,
       CASE
           WHEN (l.ns_op - o.ns_op) / o.ns_op > getvariable('threshold')
                AND l.ns_op - o.ns_op > getvariable('min_delta_ns') THEN 'REGRESSION'
           WHEN (o.ns_op - l.ns_op) / o.ns_op > getvariable('threshold')
                AND o.ns_op - l.ns_op > getvariable('min_delta_ns') THEN 'faster'
           ELSE ''
       END AS flag
FROM l
JOIN (
    SELECT r.*, 'previous' AS kind FROM bench r, picked p WHERE r.run_id = p.previous
    UNION ALL
    SELECT r.*, 'baseline' AS kind FROM bench r, picked p WHERE r.run_id = p.baseline AND p.baseline <> p.latest
) o USING (pkg, name);

.print
.print # Latest against the previous run on this machine (changes past the threshold)
SELECT pkg, name, fmt_ns(before_ns) AS before, fmt_ns(after_ns) AS after,
       printf('%+.1f%%', change * 100) AS change, allocs_change, flag
FROM compared WHERE kind = 'previous' AND flag <> ''
ORDER BY flag DESC, change DESC;

.print
.print # Latest against the baseline
SELECT pkg, name, fmt_ns(before_ns) AS before, fmt_ns(after_ns) AS after,
       printf('%+.1f%%', change * 100) AS change, allocs_change, flag
FROM compared WHERE kind = 'baseline' AND flag <> ''
ORDER BY flag DESC, change DESC;

.print
.print # Headline benchmarks over the last 8 runs (time per op)
CREATE OR REPLACE TEMP TABLE headline AS
SELECT r.name, r.run_id, u.age, fmt_ns(r.ns_op) AS t
FROM bench r JOIN runs u USING (run_id)
WHERE u.age <= 8 AND regexp_matches(r.name,
    '^(Edit/(chain|fanin|running|volatile)|RecalcAll/(dense|fanin)|Frame/(dense-8192x256|longtext-8192x500)/200x60|Keystroke/(arrow/dense-8192x256|extend-data/dense-8192x256)/80x24|Import/(owid|airport)|JEV/1000|Open/dense-8192x256|Save/dense-8192x256)');
PIVOT headline ON age USING first(t) GROUP BY name ORDER BY name;

.print
.print # Regressions
SELECT kind, count(*) FILTER (WHERE flag = 'REGRESSION') AS regressions,
       count(*) FILTER (WHERE flag = 'faster') AS faster, count(*) AS compared
FROM compared GROUP BY kind ORDER BY kind DESC;
