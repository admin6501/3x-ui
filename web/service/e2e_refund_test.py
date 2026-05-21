#!/usr/bin/env python3
"""End-to-end verification for the three reseller-related fixes.

1. addClient bills reseller (AccumulateUsage)
2. delClient refunds reseller (RefundUsage)
3. GET /panel/api/inbounds/myQuota returns fresh values
"""
import re, json, sys, sqlite3
import requests

BASE = "http://localhost:2053"


def login(username, password):
    s = requests.Session()
    r = s.get(f"{BASE}/", timeout=5)
    csrf = re.search(r'csrf-token" content="([^"]+)"', r.text).group(1)
    r = s.post(f"{BASE}/login", data={"username": username, "password": password},
               headers={"X-CSRF-Token": csrf}, timeout=5)
    assert r.status_code == 200 and r.json().get("success"), f"login failed: {r.status_code} {r.text}"
    # rotate csrf
    r = s.get(f"{BASE}/panel/inbounds", timeout=5)
    csrf = re.search(r'csrf-token" content="([^"]+)"', r.text).group(1)
    return s, csrf


def db_used(username):
    c = sqlite3.connect("/etc/x-ui/x-ui.db")
    row = c.execute("SELECT traffic_used FROM users WHERE username=?", (username,)).fetchone()
    c.close()
    return row[0] if row else None


def reset_state():
    """Clean db state for repeatable runs."""
    c = sqlite3.connect("/etc/x-ui/x-ui.db")
    cur = c.cursor()
    GIB = 1024**3
    cur.execute("UPDATE users SET traffic_used=0, traffic_quota=? WHERE username='rcapped1'", (2 * GIB,))
    seed = json.dumps({"clients":[{"id":"seed-uuid","email":"seed@e2e","totalGB":0,"expiryTime":0,"enable":True,"limitIp":0,"subId":"s","comment":"","reset":0}], "decryption":"none","fallbacks":[]})
    cur.execute("UPDATE inbounds SET settings=? WHERE tag='test-ib-1'", (seed,))
    cur.execute("DELETE FROM client_traffics WHERE email NOT IN ('seed@e2e')")
    c.commit(); c.close()


def add_client(s, csrf, inbound_id, email, total_bytes, client_id):
    settings = json.dumps({"clients":[{"id": client_id, "email": email, "totalGB": total_bytes,
                                         "expiryTime":0, "enable":True, "limitIp":0,
                                         "subId":"x", "comment":"", "reset":0}]})
    r = s.post(f"{BASE}/panel/api/inbounds/addClient",
               json={"id": inbound_id, "settings": settings},
               headers={"X-CSRF-Token": csrf}, timeout=5)
    return r.status_code, r.json()


def del_client_by_email(s, csrf, inbound_id, email):
    r = s.post(f"{BASE}/panel/api/inbounds/{inbound_id}/delClientByEmail/{email}",
               headers={"X-CSRF-Token": csrf}, json={}, timeout=5)
    return r.status_code, r.json()


def my_quota(s):
    r = s.get(f"{BASE}/panel/api/inbounds/myQuota", timeout=5)
    return r.status_code, r.json()


def run():
    results = []
    # ------ Bug 1: addClient bills reseller ------
    reset_state()
    sess, csrf = login("rcapped1", "reseller123")
    GIB = 1024**3
    print(f"BEFORE addClient: db.used = {db_used('rcapped1')} (expected 0)")
    code, resp = add_client(sess, csrf, 1, "test1@e2e", 1 * GIB, "client-uuid-1")
    print(f"addClient response: HTTP {code}, success={resp.get('success')}, msg={resp.get('msg')}")
    used_after_add = db_used("rcapped1")
    print(f"AFTER addClient: db.used = {used_after_add} (expected {GIB})")
    assert used_after_add == GIB, f"BUG 1 FAIL: used={used_after_add} expected {GIB}"
    results.append("Bug 1 (bill on addClient): PASS")

    # ------ Bug 2: myQuota endpoint reflects fresh value ------
    code, resp = my_quota(sess)
    print(f"myQuota: HTTP {code}, response={resp}")
    assert code == 200 and resp.get("success"), f"BUG 2 FAIL: {resp}"
    obj = resp.get("obj", {})
    assert obj.get("trafficUsed") == GIB, f"BUG 2 FAIL: myQuota.trafficUsed={obj.get('trafficUsed')} expected {GIB}"
    assert obj.get("trafficQuota") == 2 * GIB, f"BUG 2 FAIL: myQuota.trafficQuota={obj.get('trafficQuota')} expected {2*GIB}"
    results.append("Bug 2 (myQuota live read): PASS")

    # ------ Bug 3: delClient refunds reseller ------
    code, resp = del_client_by_email(sess, csrf, 1, "test1@e2e")
    print(f"delClient response: HTTP {code}, success={resp.get('success')}, msg={resp.get('msg')}")
    used_after_del = db_used("rcapped1")
    print(f"AFTER delClient: db.used = {used_after_del} (expected 0; refund 1 GiB)")
    assert used_after_del == 0, f"BUG 3 FAIL: used={used_after_del} expected 0"
    results.append("Bug 3 (refund on delete): PASS")

    # ------ Bug 4: full refund on delete (new auto-reconcile model) ------
    # add 5 GB client (bump quota to 10 GB), simulate 2 GB consumption,
    # delete → reconciler walks remaining clients (none) and sets used=0.
    # In the NEW model, deletion releases the FULL allocation regardless of
    # past consumption. The consumed bytes are the operator's bandwidth
    # cost, not the reseller's debt.
    reset_state()
    c = sqlite3.connect("/etc/x-ui/x-ui.db")
    c.execute("UPDATE users SET traffic_quota=? WHERE username='rcapped1'", (10 * GIB,))
    c.commit(); c.close()
    sess, csrf = login("rcapped1", "reseller123")
    code, resp = add_client(sess, csrf, 1, "test2@e2e", 5 * GIB, "client-uuid-2")
    print(f"addClient(5GB): HTTP {code}, success={resp.get('success')}, msg={resp.get('msg')}")
    assert resp.get("success"), f"addClient failed: {resp}"
    used_after_add = db_used("rcapped1")
    assert used_after_add == 5 * GIB, f"addClient billing wrong: used={used_after_add}"
    # Simulate 2 GiB of traffic on the client
    c = sqlite3.connect("/etc/x-ui/x-ui.db")
    rows = list(c.execute("SELECT email FROM client_traffics WHERE email='test2@e2e'"))
    print(f"client_traffics row for test2@e2e exists: {len(rows) == 1}")
    c.execute("UPDATE client_traffics SET up=?, down=? WHERE email='test2@e2e'", (GIB, GIB))
    c.commit(); c.close()
    print(f"BEFORE delete: reseller.used={db_used('rcapped1')} (=5 GiB)")
    code, resp = del_client_by_email(sess, csrf, 1, "test2@e2e")
    print(f"delClient: HTTP {code}, success={resp.get('success')}, msg={resp.get('msg')}")
    used_after = db_used("rcapped1")
    expected = 0  # full release: no clients remain so used = 0
    print(f"AFTER delete (auto-reconcile): reseller.used={used_after} (expected {expected})")
    assert used_after == expected, f"BUG 4 FAIL: used={used_after} expected {expected}"
    # restore original quota
    c = sqlite3.connect("/etc/x-ui/x-ui.db")
    c.execute("UPDATE users SET traffic_quota=? WHERE username='rcapped1'", (2 * GIB,))
    c.execute("UPDATE users SET traffic_used=0 WHERE username='rcapped1'")
    c.commit(); c.close()
    results.append("Bug 4 (full refund on delete, auto-reconcile model): PASS")

    print("\n" + "=" * 60)
    for r in results: print("  " + r)
    print("=" * 60)
    return 0


if __name__ == "__main__":
    try:
        sys.exit(run())
    except AssertionError as e:
        print(f"\nFAIL: {e}", file=sys.stderr)
        sys.exit(1)
