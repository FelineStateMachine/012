-- Loads the stress run records (JSONL written by internal/stress/benchrec)
-- into a table named results. The same columns are the stress_results
-- table in ClickHouse (deploy/observability/clickhouse/init.sql).
--
-- Set the file first: SET VARIABLE results = '.deps/stress/results/runs.jsonl';

CREATE OR REPLACE TEMP TABLE results AS
SELECT *
FROM read_json(getvariable('results'),
    format = 'newline_delimited',
    columns = {
        run_id: 'VARCHAR', ts: 'TIMESTAMP', git_sha: 'VARCHAR', git_dirty: 'BOOLEAN',
        go_version: 'VARCHAR', goos: 'VARCHAR', goarch: 'VARCHAR', cpu: 'VARCHAR', host: 'VARCHAR',
        pkg: 'VARCHAR', name: 'VARCHAR', procs: 'INTEGER', n: 'BIGINT',
        ns_op: 'DOUBLE', b_op: 'DOUBLE', allocs_op: 'DOUBLE', mb_s: 'DOUBLE',
        metrics: 'MAP(VARCHAR, DOUBLE)'
    });

-- Durations in readable units.
CREATE OR REPLACE TEMP MACRO fmt_ns(ns) AS
    CASE
        WHEN ns IS NULL THEN ''
        WHEN ns >= 1e9 THEN printf('%.2f s', ns / 1e9)
        WHEN ns >= 1e6 THEN printf('%.2f ms', ns / 1e6)
        WHEN ns >= 1e3 THEN printf('%.1f us', ns / 1e3)
        ELSE printf('%.0f ns', ns)
    END;

-- Runs, newest first, numbered: 1 is the latest.
CREATE OR REPLACE TEMP TABLE runs AS
SELECT run_id, min(ts) AS ts, any_value(git_sha) AS git_sha, bool_or(git_dirty) AS dirty,
       any_value(cpu) AS cpu, any_value(goos) AS goos, any_value(go_version) AS go_version,
       count(*) AS benchmarks,
       row_number() OVER (ORDER BY min(ts) DESC) AS age
FROM results
GROUP BY run_id;
