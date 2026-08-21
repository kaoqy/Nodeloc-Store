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
    """Read instance/config.ini if it exists; return a dict of overrides."""
    if not CONFIG_PATH.exists():
        return {}
    cp = RawConfigParser()
    cp.read(CONFIG_PATH, encoding="utf-8")
    out: dict = {}
    if cp.has_section("app"):
        for k, v in cp.items("app"):
            out[k] = v
    if cp.has_section("oauth"):
        for k, v in cp.items("oauth"):
            out[f"OAuth_{k}"] = v
    if cp.has_section("payment"):
        for k, v in cp.items("payment"):
            out[f"PAYMENT_{k}"] = v
    if cp.has_section("database"):
        for k, v in cp.items("database"):
            out[f"DB_{k.upper()}"] = v
    return out


def _build_db_uri(values: dict) -> str:
    user = values.get("DB_USER", "")
    pwd = values.get("DB_PASSWORD", "")
    host = values.get("DB_HOST", "")
    port = values.get("DB_PORT", "3306")
    name = values.get("DB_NAME", "")
    if user and host and name:
        return f"mysql+pymysql://{user}:{pwd}@{host}:{port}/{name}?charset=utf8mb4"
    sqlite_path = INSTANCE_DIR / "fallback.db"
    INSTANCE_DIR.mkdir(parents=True, exist_ok=True)
    return f"sqlite:///{sqlite_path}"


def _build_config_dict(values: dict) -> dict:
    return dict(
        # Flask
        SECRET_KEY=os.environ.get("SECRET_KEY") or values.get("secret_key", "change-me-please"),
        # DB
        DB_USER=values.get("DB_USER", ""),
        DB_PASSWORD=values.get("DB_PASSWORD", ""),
        DB_HOST=values.get("DB_HOST", ""),
        DB_PORT=values.get("DB_PORT", "3306"),
        DB_NAME=values.get("DB_NAME", ""),
        SQLALCHEMY_DATABASE_URI=os.environ.get("DATABASE_URL") or _build_db_uri(values),
        # Uploads
        UPLOAD_FOLDER=str(BASE_DIR / "uploads" / "products"),
        MAX_CONTENT_LENGTH=8 * 1024 * 1024,
        # Storefront
        SITE_NAME=values.get("site_name", "NodeLoc Store"),
        SITE_SLOGAN=values.get("site_slogan", "卡密商店 · Powered by NodeLoc"),
        CURRENCY=values.get("currency", "积分"),
        # OAuth
        NODELOC_URL=values.get("OAuth_url", "https://www.nodeloc.com").rstrip("/"),
        NODELOC_CLIENT_ID=values.get("OAuth_client_id", ""),
        NODELOC_CLIENT_SECRET=values.get("OAuth_client_secret", ""),
        NODELOC_REDIRECT_URI=values.get("OAuth_redirect_uri", ""),
        NODELOC_SCOPES=values.get("OAuth_scopes", "openid profile email"),
        # Payment
        PAYMENT_ID=values.get("PAYMENT_id", ""),
        PAYMENT_SECRET=values.get("PAYMENT_secret", ""),
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
