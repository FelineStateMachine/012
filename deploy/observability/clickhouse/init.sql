-- Stress run records, one row per benchmark per run: the same columns as
-- the JSONL written by internal/stress/benchrec (and read by DuckDB in
-- scripts/stress/load.sql). Loaded by scripts/stress/clickhouse-load.sh;
-- loading a run twice is harmless (ReplacingMergeTree, query with FINAL).
CREATE DATABASE IF NOT EXISTS stress;

CREATE TABLE IF NOT EXISTS stress.results
(
    run_id     String,
    ts         DateTime64(3, 'UTC'),
    git_sha    LowCardinality(String),
    git_dirty  Bool,
    go_version LowCardinality(String),
    goos       LowCardinality(String),
    goarch     LowCardinality(String),
    cpu        LowCardinality(String),
    host       LowCardinality(String),
    pkg        LowCardinality(String),
    name       String,
    procs      UInt16,
    n          UInt64,
    ns_op      Float64,
    b_op       Float64,
    allocs_op  Float64,
    mb_s       Float64,
    metrics    Map(String, Float64)
)
ENGINE = ReplacingMergeTree
ORDER BY (cpu, name, ts, run_id);

-- The latest run against the one before it on the same CPU, per
-- benchmark: what the Grafana regressions table and make stress-report
-- (DuckDB) both show.
CREATE VIEW IF NOT EXISTS stress.latest_vs_previous AS
WITH ranked AS
(
    SELECT run_id, cpu, row_number() OVER (PARTITION BY cpu ORDER BY started DESC) AS age
    FROM (SELECT run_id, cpu, min(ts) AS started FROM stress.results FINAL GROUP BY run_id, cpu)
)
SELECT l.cpu AS cpu, l.pkg AS pkg, l.name AS name,
       p.ns_op AS before_ns, l.ns_op AS after_ns,
       (l.ns_op - p.ns_op) / p.ns_op AS change
FROM stress.results AS l FINAL
JOIN ranked AS kl ON kl.run_id = l.run_id AND kl.cpu = l.cpu
JOIN stress.results AS p FINAL ON p.cpu = l.cpu AND p.pkg = l.pkg AND p.name = l.name
JOIN ranked AS kp ON kp.run_id = p.run_id AND kp.cpu = p.cpu
WHERE kl.age = 1 AND kp.age = 2;
