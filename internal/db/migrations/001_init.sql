CREATE TABLE IF NOT EXISTS projects (
    id           INTEGER PRIMARY KEY,
    name         TEXT NOT NULL,
    address      TEXT,
    notes        TEXT,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS scans (
    id           INTEGER PRIMARY KEY,
    project_id   INTEGER NOT NULL REFERENCES projects(id),
    type         TEXT NOT NULL,
    subnet       TEXT,
    interface    TEXT,
    gateway      TEXT,
    dns          TEXT,
    method       TEXT,
    status       TEXT NOT NULL DEFAULT 'running',
    progress     INTEGER NOT NULL DEFAULT 0,
    message      TEXT,
    started_at   TEXT NOT NULL,
    finished_at  TEXT
);

CREATE TABLE IF NOT EXISTS devices (
    id           INTEGER PRIMARY KEY,
    project_id   INTEGER NOT NULL REFERENCES projects(id),
    mac          TEXT NOT NULL,
    last_ip      TEXT,
    hostname     TEXT,
    vendor       TEXT,
    detected_type TEXT,
    os_guess     TEXT,
    custom_name  TEXT,
    device_type  TEXT,
    location     TEXT,
    notes        TEXT,
    color        TEXT,
    first_seen   TEXT,
    last_seen    TEXT,
    UNIQUE (project_id, mac)
);

CREATE TABLE IF NOT EXISTS device_observations (
    id           INTEGER PRIMARY KEY,
    scan_id      INTEGER NOT NULL REFERENCES scans(id),
    device_id    INTEGER NOT NULL REFERENCES devices(id),
    ip           TEXT,
    open_ports   TEXT,
    services     TEXT,
    latency_ms   REAL
);

CREATE TABLE IF NOT EXISTS survey_points (
    id           INTEGER PRIMARY KEY,
    project_id   INTEGER NOT NULL REFERENCES projects(id),
    map_id       INTEGER REFERENCES maps(id),
    label        TEXT,
    x            REAL NOT NULL,
    y            REAL NOT NULL,
    created_at   TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS wifi_networks (
    id           INTEGER PRIMARY KEY,
    project_id   INTEGER NOT NULL REFERENCES projects(id),
    ssid         TEXT,
    bssid        TEXT NOT NULL,
    band         TEXT,
    channel      INTEGER,
    frequency    INTEGER,
    security     TEXT,
    vendor       TEXT,
    ap_device_id INTEGER REFERENCES devices(id),
    ap_link_status TEXT DEFAULT 'none',
    label        TEXT,
    location     TEXT,
    notes        TEXT,
    first_seen   TEXT,
    last_seen    TEXT,
    UNIQUE (project_id, bssid)
);

CREATE TABLE IF NOT EXISTS wifi_observations (
    id           INTEGER PRIMARY KEY,
    scan_id      INTEGER NOT NULL REFERENCES scans(id),
    wifi_id      INTEGER NOT NULL REFERENCES wifi_networks(id),
    signal_dbm   INTEGER,
    survey_point_id INTEGER REFERENCES survey_points(id)
);

CREATE TABLE IF NOT EXISTS maps (
    id           INTEGER PRIMARY KEY,
    project_id   INTEGER NOT NULL REFERENCES projects(id),
    name         TEXT NOT NULL,
    scene_json   TEXT NOT NULL,
    background   TEXT,
    version      INTEGER NOT NULL DEFAULT 1,
    updated_at   TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_devobs_scan ON device_observations(scan_id);
CREATE INDEX IF NOT EXISTS idx_devobs_dev ON device_observations(device_id);
CREATE INDEX IF NOT EXISTS idx_wifiobs_wifi ON wifi_observations(wifi_id);
CREATE INDEX IF NOT EXISTS idx_wifiobs_sp ON wifi_observations(survey_point_id);
