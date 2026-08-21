"""install blueprint — runs once on first访问 to set DB + admin."""
from __future__ import annotations

from configparser import RawConfigParser
from pathlib import Path

from flask import Blueprint, current_app, redirect, render_template, request, url_for

from ..extensions import db
from ..models import User

bp = Blueprint("install", __name__)

INSTANCE_DIR = Path(__file__).resolve().parents[1] / "instance"
CONFIG_PATH = INSTANCE_DIR / "config.ini"


@bp.route("/", methods=["GET", "POST"])
def index():
    if request.method == "POST":
        # --- 1) DB config ---
        db_host = request.form.get("db_host", "").strip()
        db_port = request.form.get("db_port", "3306").strip()
        db_user = request.form.get("db_user", "").strip()
        db_pass = request.form.get("db_pass", "")
        db_name = request.form.get("db_name", "").strip()

        # --- 2) Site info ---
        site_name = request.form.get("site_name", "NodeLoc Store").strip()
        site_slogan = request.form.get("site_slogan", "").strip()

        # --- 3) OAuth ---
        oauth_url = request.form.get("oauth_url", "https://www.nodeloc.com").strip().rstrip("/")
        oauth_client_id = request.form.get("oauth_client_id", "").strip()
        oauth_client_secret = request.form.get("oauth_client_secret", "")
        oauth_redirect_uri = request.form.get("oauth_redirect_uri", "").strip()
        oauth_scopes = request.form.get("oauth_scopes", "openid profile email").strip()

        # --- 4) Payment ---
        payment_id = request.form.get("payment_id", "").strip()
        payment_secret = request.form.get("payment_secret", "")

        # --- 5) Admin ---
        admin_user = request.form.get("admin_user", "").strip()
        admin_email = request.form.get("admin_email", "").strip() or None
        admin_pass = request.form.get("admin_pass", "")

        # Validate required fields
        required = {
            "db_host": db_host, "db_port": db_port, "db_user": db_user,
            "db_name": db_name, "admin_user": admin_user, "admin_pass": admin_pass,
            "oauth_client_id": oauth_client_id, "oauth_client_secret": oauth_client_secret,
            "oauth_redirect_uri": oauth_redirect_uri, "payment_id": payment_id,
            "payment_secret": payment_secret,
        }
        missing = [k for k, v in required.items() if not v]
        if missing:
            return render_template(
                "install/index.html", error=f"以下字段必填: {', '.join(missing)}"
            )
        if len(admin_pass) < 8:
            return render_template("install/index.html", error="管理员密码至少 8 位")

        # Write config.ini
        try:
            _write_config(
                db_host=db_host, db_port=db_port, db_user=db_user,
                db_pass=db_pass, db_name=db_name,
                site_name=site_name, site_slogan=site_slogan,
                oauth_url=oauth_url, oauth_client_id=oauth_client_id,
                oauth_client_secret=oauth_client_secret,
                oauth_redirect_uri=oauth_redirect_uri, oauth_scopes=oauth_scopes,
                payment_id=payment_id, payment_secret=payment_secret,
            )
        except Exception as e:
            return render_template("install/index.html", error=f"配置写入失败: {e}")

        # Reload app with new config and create tables
        from ..extensions import db as _db
        from .. import create_app
        app = create_app()
        with app.app_context():
            _db.create_all()
            if not User.query.filter_by(username=admin_user).first():
                u = User(username=admin_user, email=admin_email, is_admin=True)
                u.set_password(admin_pass)
                _db.session.add(u)
                _db.session.commit()

        _mark_installed()
        return redirect(url_for("store.index"))

    # Build the callback hint URL for the payment field
    return render_template("install/index.html")


# ── helpers ──────────────────────────────────────────────────────────────
def _write_config(**kw):
    cfg = RawConfigParser()
    if CONFIG_PATH.exists():
        cfg.read(CONFIG_PATH, encoding="utf-8")

    for section in ["database", "app", "oauth", "payment"]:
        if not cfg.has_section(section):
            cfg.add_section(section)

    cfg.set("database", "db_host", kw["db_host"])
    cfg.set("database", "db_port", kw["db_port"])
    cfg.set("database", "db_user", kw["db_user"])
    cfg.set("database", "db_pass", kw["db_pass"])
    cfg.set("database", "db_name", kw["db_name"])

    cfg.set("app", "site_name", kw["site_name"])
    cfg.set("app", "site_slogan", kw["site_slogan"])
    cfg.set("app", "secret_key", _gen_secret_key())
    cfg.set("app", "installed", "0")

    cfg.set("oauth", "url", kw["oauth_url"])
    cfg.set("oauth", "client_id", kw["oauth_client_id"])
    cfg.set("oauth", "client_secret", kw["oauth_client_secret"])
    cfg.set("oauth", "redirect_uri", kw["oauth_redirect_uri"])
    cfg.set("oauth", "scopes", kw["oauth_scopes"])

    cfg.set("payment", "id", kw["payment_id"])
    cfg.set("payment", "secret", kw["payment_secret"])

    INSTANCE_DIR.mkdir(parents=True, exist_ok=True)
    with open(CONFIG_PATH, "w", encoding="utf-8") as f:
        cfg.write(f)


def _mark_installed():
    cfg = RawConfigParser()
    cfg.read(CONFIG_PATH, encoding="utf-8")
    cfg.set("app", "installed", "1")
    with open(CONFIG_PATH, "w", encoding="utf-8") as f:
        cfg.write(f)


def _gen_secret_key() -> str:
    import secrets
    return secrets.token_hex(32)
