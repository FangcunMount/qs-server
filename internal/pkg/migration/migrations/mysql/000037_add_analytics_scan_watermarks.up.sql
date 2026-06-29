CREATE TABLE IF NOT EXISTS analytics_scan_watermarks (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
    source_name VARCHAR(64) NOT NULL,
    org_id BIGINT NOT NULL DEFAULT 0,
    last_seen_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
    last_seen_time DATETIME(3) NULL,
    scan_window_start DATETIME(3) NULL,
    scan_window_end DATETIME(3) NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'idle',
    last_error TEXT NULL,
    created_at DATETIME(3) NOT NULL,
    updated_at DATETIME(3) NOT NULL,
    deleted_at DATETIME(3) NULL,
    UNIQUE KEY uk_source_org (source_name, org_id),
    KEY idx_status_updated_at (status, updated_at)
);
