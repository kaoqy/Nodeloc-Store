#!/usr/bin/env python3
"""Smoke test: pure stdlib, no third-party deps required.

Run from repo root:
    python3 scripts/smoke_test.py

Validates:
  - NodeLoc OAuth2 callback URL is built correctly
  - Outgoing payment signature uses HMAC-SHA256(SHA256(secret), params)
  - Callback verification uses HMAC-SHA256(secret, params) and round-trips
  - Config: ini → apply_to produces correct SQLALCHEMY_URI and creds
"""
from __future__ import annotations

import hashlib
import hmac
import importlib.util
import shutil
import sys
from configparser import RawConfigParser
from pathlib import Path

REPO = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(REPO))

# Stub third-party deps so we can import app modules in stdlib-only env.
import types
for _mod in ("flask", "flask_sqlalchemy", "flask_login", "flask_wtf",
             "flask_wtf.csrf", "wtforms", "email_validator", "PIL", "werkzeug"):
    sys.modules.setdefault(_mod, types.ModuleType(_mod))

# 'requests' is a real import in nodeloc.py — provide a stub with Session.
if "requests" not in sys.modules:
    _r = types.ModuleType("requests")
    _r.Session = type("Session", (), {"headers": {}})
    sys.modules["requests"] = _r

# ── import via real package path (so dataclass introspection works) ────
from app import config, nodeloc  # noqa: E402

PASS = 0
FAIL = 0


def check(label: str, got, want):
    global PASS, FAIL
    if got == want:
        PASS += 1
        print(f"  ✓ {label}")
    else:
        FAIL += 1
        print(f"  ✗ {label}\n      got:  {got!r}\n      want: {want!r}")


# ── 1) Config round-trip ────────────────────────────────────────────────
print("\n[1] config.ini round-trip")
shutil.rmtree(config.INSTANCE_DIR, ignore_errors=True)
config.INSTANCE_DIR.mkdir(parents=True, exist_ok=True)
cp = RawConfigParser()
for s in ("database", "app", "oauth", "payment"):
    cp.add_section(s)
cp.set("database", "db_host", "db.local")
cp.set("database", "db_port", "3306")
cp.set("database", "db_user", "u")
cp.set("database", "db_pass", "p")
cp.set("database", "db_name", "shop")
cp.set("app", "installed", "1")
cp.set("oauth", "url", "https://www.nodeloc.com")
cp.set("oauth", "client_id", "CID")
cp.set("oauth", "client_secret", "CSC")
cp.set("oauth", "redirect_uri", "https://shop.example.com/auth/oauth/callback")
cp.set("oauth", "scopes", "openid profile email")
cp.set("payment", "id", "pay_xxx")
cp.set("payment", "secret", "sec_xxx")
cp.set("app", "site_name", "我的商店")
cp.set("app", "secret_key", "k" * 32)
with open(config.CONFIG_PATH, "w") as f:
    cp.write(f)


class FakeConfig(dict):
    pass


class FakeApp:
    config = FakeConfig()


config.apply_to(FakeApp())
c = FakeApp.config
check("is_installed()", config.is_installed(), True)
check("SITE_NAME", c.get("SITE_NAME"), "我的商店")
check("NODELOC_URL", c.get("NODELOC_URL"), "https://www.nodeloc.com")
check("NODELOC_CLIENT_ID", c.get("NODELOC_CLIENT_ID"), "CID")
check("NODELOC_REDIRECT_URI", c.get("NODELOC_REDIRECT_URI"),
      "https://shop.example.com/auth/oauth/callback")
check("NODELOC_SCOPES", c.get("NODELOC_SCOPES"), "openid profile email")
check("PAYMENT_ID", c.get("PAYMENT_ID"), "pay_xxx")
check("PAYMENT_SECRET", c.get("PAYMENT_SECRET"), "sec_xxx")
check("SQLALCHEMY_URI", c.get("SQLALCHEMY_DATABASE_URI"),
      "mysql+pymysql://u:p@db.local:3306/shop?charset=utf8mb4")
check("SECRET_KEY", c.get("SECRET_KEY"), "k" * 32)

# ── 2) Outgoing payment signature: HMAC-SHA256(SHA256(secret), params) ─
print("\n[2] Outgoing payment signature")
secret = "tk_abc123"
params = {"amount": 100, "description": "购买VIP", "order_id": "ord_001"}
expected = hmac.new(
    hashlib.sha256(secret.encode()).hexdigest().encode(),
    b"amount=100&description=\xe8\xb4\xad\xe4\xb9\xb0VIP&order_id=ord_001",
    hashlib.sha256,
).hexdigest()
sig = nodeloc._sign_with_hashed_token(params, secret)
check("outgoing HMAC-SHA256(SHA256(secret), params)", sig, expected)

# ── 3) Callback verification: HMAC-SHA256(secret, params) ──────────────
print("\n[3] Callback signature verification")
callback_params = {
    "transaction_id": "txn_xxx",
    "external_reference": "ord_001",
    "amount": "100",
    "platform_fee": "5",
    "merchant_points": "95",
    "status": "completed",
    "paid_at": "2025-10-08T12:00:00Z",
}
expected_cb_sig = hmac.new(
    secret.encode(),
    b"amount=100&external_reference=ord_001&merchant_points=95&paid_at=2025-10-08T12:00:00Z&platform_fee=5&status=completed&transaction_id=txn_xxx",
    hashlib.sha256,
).hexdigest()
callback_params["signature"] = expected_cb_sig
check("callback verifies (round-trip)",
      nodeloc.NodeLocPayment.verify_callback(callback_params, secret), True)
check("callback rejects wrong secret",
      nodeloc.NodeLocPayment.verify_callback(callback_params, "wrong"), False)
check("callback rejects missing signature",
      nodeloc.NodeLocPayment.verify_callback({"amount": "1"}, secret), False)

# ── 4) OAuth URL build ──────────────────────────────────────────────────
print("\n[4] OAuth authorize URL")
oauth = nodeloc.NodeLocOAuth(
    base_url="https://www.nodeloc.com",
    client_id="CID",
    client_secret="CSC",
    redirect_uri="https://shop.example.com/auth/oauth/callback",
    scopes="openid profile email",
)
url = oauth.build_authorize_url(state="xyz")
check("authorize URL", url,
      "https://www.nodeloc.com/oauth-provider/authorize?"
      "client_id=CID&redirect_uri=https%3A%2F%2Fshop.example.com%2Fauth%2Foauth%2Fcallback"
      "&response_type=code&scope=openid+profile+email&state=xyz")
check("is_configured()", oauth.is_configured(), True)
check("unconfigured is_configured()",
      nodeloc.NodeLocOAuth("x", "", "", "y").is_configured(), False)

# ── 5) OAuth token parsing ──────────────────────────────────────────────
print("\n[5] OAuth token parsing")
token = nodeloc.OAuthToken.from_response({
    "access_token": "at_abc",
    "token_type": "Bearer",
    "expires_in": 7200,
    "refresh_token": "rt_xyz",
    "scope": "openid profile email",
})
check("access_token", token.access_token, "at_abc")
check("refresh_token", token.refresh_token, "rt_xyz")
check("scope parsed", token.scope, "openid profile email")
check("expires_at is future", token.expires_at > 0, True)

# ── 6) Payment methods (offline check) ─────────────────────────────────
print("\n[6] Payment client is_configured")
pay = nodeloc.NodeLocPayment("https://www.nodeloc.com", "pay_xxx", "sec_xxx")
check("payment is_configured", pay.is_configured(), True)
check("unconfigured payment",
      nodeloc.NodeLocPayment("x", "", "").is_configured(), False)

# ── Done ────────────────────────────────────────────────────────────────
print(f"\n{'='*40}")
print(f"  PASS: {PASS}    FAIL: {FAIL}")
print(f"{'='*40}\n")
shutil.rmtree(config.INSTANCE_DIR, ignore_errors=True)
sys.exit(0 if FAIL == 0 else 1)
