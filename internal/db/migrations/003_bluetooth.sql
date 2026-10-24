CREATE TABLE IF NOT EXISTS bluetooth_devices (
    id            INTEGER PRIMARY KEY,
    project_id    INTEGER NOT NULL REFERENCES projects(id),
    mac           TEXT NOT NULL,
    name          TEXT,
    address_type  TEXT,                 -- public | random
    kind          TEXT,                 -- BlueZ Icon (phone, audio-card, input-mouse...)
    manufacturer  TEXT,
    paired        INTEGER NOT NULL DEFAULT 0,
    detected_type TEXT,                 -- ghicit (cheie icon)
    device_type   TEXT,                 -- ales de utilizator
    custom_name   TEXT,
    location      TEXT,
    notes         TEXT,
    device_id     INTEGER REFERENCES devices(id),   -- același echipament în LAN
    link_status   TEXT DEFAULT 'none',  -- none | inferat | confirmat | respins
    first_seen    TEXT,
    last_seen     TEXT,
    UNIQUE (project_id, mac)
);

CREATE TABLE IF NOT EXISTS bluetooth_observations (
    id        INTEGER PRIMARY KEY,
    scan_id   INTEGER NOT NULL REFERENCES scans(id),
    bt_id     INTEGER NOT NULL REFERENCES bluetooth_devices(id),
    rssi      INTEGER
);

CREATE INDEX IF NOT EXISTS idx_btobs_bt ON bluetooth_observations(bt_id);
