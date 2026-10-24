-- Un MAC poate răspunde pentru mai multe IP-uri (repetor Wi-Fi cu MAC-NAT,
-- proxy ARP, echipament cu mai multe adrese). Atunci fiecare IP devine un
-- dispozitiv separat, cu ip_key = IP; în rest ip_key rămâne ''.
-- SQLite nu poate schimba o constrângere UNIQUE, așa că refacem tabela
-- (migrate() rulează cu foreign_keys dezactivat; id-urile se păstrează).
CREATE TABLE devices_new (
    id           INTEGER PRIMARY KEY,
    project_id   INTEGER NOT NULL REFERENCES projects(id),
    mac          TEXT NOT NULL,
    ip_key       TEXT NOT NULL DEFAULT '',
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
    UNIQUE (project_id, mac, ip_key)
);

INSERT INTO devices_new(id, project_id, mac, ip_key, last_ip, hostname, vendor, detected_type, os_guess,
                        custom_name, device_type, location, notes, color, first_seen, last_seen)
SELECT id, project_id, mac, '', last_ip, hostname, vendor, detected_type, os_guess,
       custom_name, device_type, location, notes, color, first_seen, last_seen
FROM devices;

DROP TABLE devices;
ALTER TABLE devices_new RENAME TO devices;
