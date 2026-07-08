ALTER TABLE evaluation_run
    ADD COLUMN input_snapshot_ref VARCHAR(200) NULL AFTER trace_id;
