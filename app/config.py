"""
Application configuration.

Settings are loaded from instance/config.ini (created by the install wizard).
Use `reload_config()` to re-read after the install wizard writes new settings.
"""
from __future__ import annotations

import os
from configparser import RawConfigParser
from pathlib import Path

BASE_DIR = Path(__file__).resolve().parent.parent
INSTANCE_DIR = BASE_DIR / "instance"
CONFIG_PATH = INSTANCE_DIR / "config.ini"


def _read_ini() -> dict:
    """Read instance/config.ini if it exists; return a flat dict of
    section.key pairs (no prefixing). Callers know which section each key
    lives in.
    """
    if not CONFIG_PATH.exists():
        return {}
    cp = RawConfigParser()
    cp.read(CONFIG_PATH, encoding="utf-8")
    out: dict = {}
    for section in ("app", "oauth", "payment", "database"):
        if not cp.has_section(section):
            continue
        for k, v in cp.items(section):
            out[k] = v
    return out


def _build_db_uri(values: dict) -> str:
    user = values.get("db_user", "")
    pwd = values.get("db_pass", "")
    host = values.get("db_host", "")
    port = values.get("db_port", "3306")
    name = values.get("db_name", "")
    if user and host and name:
        return f"mysql+pymysql://{user}:{pwd}@{host}:{port}/{name}?charset=utf8mb4"
    sqlite_path = INSTANCE_DIR / "fallback.db"
    INSTANCE_DIR.mkdir(parents=True, exist_ok=True)
    return f"sqlite:///{sqlite_path}"


def _build_config_dict(values: dict) -> dict:
    return dict(
        # Flask
        SECRET_KEY=os.environ.get("SECRET_KEY") or values.get("secret_key", "change-me-please"),
        # Storefront
        SITE_NAME=values.get("site_name", "NodeLoc Store"),
        SITE_SLOGAN=values.get("site_slogan", "卡密商店 · Powered by NodeLoc"),
        CURRENCY=values.get("currency", "积分"),
        # OAuth (ini keys: url / client_id / client_secret / redirect_uri / scopes / allow_http)
        NODELOC_URL=values.get("url", "https://www.nodeloc.com").rstrip("/"),
        NODELOC_CLIENT_ID=values.get("client_id", ""),
        NODELOC_CLIENT_SECRET=values.get("client_secret", ""),
        NODELOC_REDIRECT_URI=values.get("redirect_uri", ""),
        NODELOC_SCOPES=values.get("scopes", "openid profile email"),
        ALLOW_HTTP=values.get("allow_http", "0") == "1",
        # Payment (ini keys: id / secret)
        PAYMENT_ID=values.get("id", ""),
        PAYMENT_SECRET=values.get("secret", ""),
        # DB (read directly from values via _build_db_uri)
        SQLALCHEMY_DATABASE_URI=os.environ.get("DATABASE_URL") or _build_db_uri(values),
    )


class Config:
    # Defaults — overridden at create_app() time via apply_to()
    SECRET_KEY = "change-me-please"
    SQLALCHEMY_DATABASE_URI = "sqlite:///:memory:"
    SQLALCHEMY_TRACK_MODIFICATIONS = False
    SQLALCHEMY_ENGINE_OPTIONS = {"pool_pre_ping": True, "pool_recycle": 280}
    UPLOAD_FOLDER = str(BASE_DIR / "uploads" / "products")
    MAX_CONTENT_LENGTH = 8 * 1024 * 1024
    SITE_NAME = "NodeLoc Store"
    SITE_SLOGAN = "卡密商店 · Powered by NodeLoc"
    CURRENCY = "积分"
    NODELOC_URL = "https://www.nodeloc.com"
    NODELOC_CLIENT_ID = ""
    NODELOC_CLIENT_SECRET = ""
    NODELOC_REDIRECT_URI = ""
    NODELOC_SCOPES = "openid profile email"
    ALLOW_HTTP = False
    PAYMENT_ID = ""
    PAYMENT_SECRET = ""
    PERMANENT_SESSION_LIFETIME = 60 * 60 * 24 * 7
    SESSION_COOKIE_HTTPONLY = True
    SESSION_COOKIE_SAMESITE = "Lax"
    PREFERRED_URL_SCHEME = "https"


def apply_to(app) -> None:
    """Read config.ini fresh and push values onto the Flask app config."""
    values = _read_ini()
    for k, v in _build_config_dict(values).items():
        app.config[k] = v


def is_installed() -> bool:
    if not CONFIG_PATH.exists():
        return False
    cp = RawConfigParser()
    cp.read(CONFIG_PATH, encoding="utf-8")
    return cp.has_option("app", "installed") and cp.get("app", "installed") == "1"
