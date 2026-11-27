"""Freshen the CoreScope e2e fixture and seed generated observer_metrics.

Runs inside python:3.13-alpine (stdlib only). Usage:
    python -I dev_seed.py <pristine-fixture.db> <output.db>

Everything seeded here is generated from a fixed RNG seed; no real data.
"""
import math
import random
import shutil
import sqlite3
import sys
import time
from datetime import datetime, timedelta, timezone

SRC, DST = sys.argv[1], sys.argv[2]
STEP = 300
DAYS = 7
N_OBSERVERS = 3
SEED = 2428

shutil.copyfile(SRC, DST)
db = sqlite3.connect(DST)
now = int(time.time())

# --- Shift every timestamp so the newest transmission is "now" (same idea as
# CoreScope's tools/freshen-fixture.sh; one delta keeps relative ordering).
(newest,) = db.execute("SELECT MAX(first_seen) FROM transmissions").fetchone()
newest_epoch = int(datetime.strptime(newest, "%Y-%m-%dT%H:%M:%SZ").replace(tzinfo=timezone.utc).timestamp())
delta = now - newest_epoch


def shift_text(table, col):
    exists = db.execute("SELECT 1 FROM sqlite_master WHERE type='table' AND name=?", (table,)).fetchone()
    if not exists:
        return
    rows = db.execute(f"SELECT rowid, {col} FROM {table} WHERE {col} IS NOT NULL").fetchall()
    out = []
    for rowid, val in rows:
        t = datetime.fromisoformat(val.replace("Z", "+00:00")) + timedelta(seconds=delta)
        out.append((t.astimezone(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"), rowid))
    db.executemany(f"UPDATE {table} SET {col}=? WHERE rowid=?", out)


for table, col in [
    ("nodes", "last_seen"), ("nodes", "first_seen"),
    ("transmissions", "first_seen"),
    ("observers", "last_seen"), ("observers", "first_seen"), ("observers", "last_packet_at"),
    ("neighbor_edges", "last_seen"),
]:
    shift_text(table, col)
db.execute("UPDATE observations SET timestamp = timestamp + ? WHERE timestamp > 0", (delta,))
db.execute("UPDATE observations SET timestamp = CAST(strftime('%s', (SELECT first_seen FROM transmissions WHERE id = transmission_id)) AS INTEGER) WHERE timestamp = 0 OR timestamp IS NULL")
db.execute("UPDATE observers SET inactive = 0 WHERE inactive = 1")

# --- observer_metrics for the busiest observers.
db.execute(
    """CREATE TABLE IF NOT EXISTS observer_metrics (
        observer_id TEXT NOT NULL, timestamp TEXT NOT NULL, noise_floor REAL,
        tx_air_secs INTEGER, rx_air_secs INTEGER, recv_errors INTEGER, battery_mv INTEGER,
        packets_sent INTEGER, packets_recv INTEGER, PRIMARY KEY (observer_id, timestamp))"""
)
db.execute("DELETE FROM observer_metrics")
top = db.execute(
    """SELECT o.id, COUNT(*) AS n FROM observations x JOIN observers o ON o.rowid = x.observer_idx
       GROUP BY o.id ORDER BY n DESC, o.id LIMIT ?""", (N_OBSERVERS,)
).fetchall()

end = now - now % STEP
points = DAYS * 86400 // STEP
reboot_at = int(points * 3.5 / DAYS)          # middle of day 4
recharge_at = int(points * 4.6 / DAYS)
decay_pts = int(points * 3.0 / DAYS)          # 4.2 V -> 3.6 V over three days
rows = []
for k, (oid, _) in enumerate(top):
    rng = random.Random(SEED + k)
    tx = rx = ps = pr = 0
    for i in range(points + 1):
        ts = end - (points - i) * STEP
        hour = (ts % 86400) / 3600.0
        day = math.sin(2 * math.pi * (hour - 9) / 24)     # daily cycle in [-1, 1]
        noise = -112 + 3.0 * day + rng.gauss(0, 0.8) + 2 * k
        busy = 0.5 + 0.5 * math.sin(2 * math.pi * (hour - 15) / 24)
        tx_frac = 0.005 + 0.025 * busy * rng.uniform(0.6, 1.0)   # 0.5 - 3 %
        rx_frac = 0.03 + 0.07 * busy * rng.uniform(0.6, 1.0)     # 3 - 10 %
        if i == reboot_at:
            tx = rx = ps = pr = 0
        else:
            tx += round(STEP * tx_frac)
            rx += round(STEP * rx_frac)
            ps += rng.randint(2, 12)
            pr += rng.randint(15, 60)
        if i < recharge_at:
            frac = min(i, decay_pts) / decay_pts
            volts = 4.2 - 0.6 * frac * (1.0 if i < reboot_at else 0.9)
            volts = max(volts, 3.6)
        else:
            rel = (i - recharge_at) / max(1, points - recharge_at)
            volts = 4.2 - 0.25 * rel
        mv = int(volts * 1000 + rng.gauss(0, 6))
        errs = rng.randint(0, 3) if rng.random() < 0.3 else 0
        rows.append((oid, datetime.fromtimestamp(ts, timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
                     round(noise, 1), tx, rx, errs, mv, ps, pr))
db.executemany("INSERT INTO observer_metrics VALUES (?,?,?,?,?,?,?,?,?)", rows)
db.commit()
db.execute("VACUUM")
db.close()
for oid, n in top:
    print(f"seeded observer {oid} ({n} observations): {points + 1} samples")
