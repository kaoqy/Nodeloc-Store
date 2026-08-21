"""install blueprint — runs once on first访问 to set DB + admin."""
from __future__ import annotations

from configparser import RawConfigParser

from flask import (Blueprint, current_app, redirect, render_template, request,
                   url_for)
from flask_login import current_user

from ..extensions import db
from ..models import User
from ..utils import log_action
from ..config import INSTANCE_DIR, CONFIG_PATH, apply_to, is_installed

bp = Blueprint("install", __name__)


# Default values used to render the wizard.  On GET they fall back to these
# if no config.ini exists.  On POST error, they're filled from the user's
# submitted values so they can fix + retry without retyping everything.
_DEFAULT_DEFAULTS = {
    "db_host": "localhost", "db_port": "3306", "db_name": "nodeloc_store",
    "db_user": "", "db_pass": "",
    "site_name": "NodeLoc Store", "site_slogan": "",
    "oauth_url": "https://www.nodeloc.com", "oauth_client_id": "",
    "oauth_client_secret": "",
    "payment_id": "", "payment_secret": "",
    "admin_user": "", "admin_email": "", "admin_pass": "",
}


def _form_defaults() -> dict:
    """Read the current form values from the request, falling back to the
    most recent partial-install state in instance/config.ini, then to the
    hardcoded defaults."""
    out = dict(_DEFAULT_DEFAULTS)
    if CONFIG_PATH.exists():
        cp = RawConfigParser()
        cp.read(CONFIG_PATH, encoding="utf-8")
        def _g(sec, key, fallback=""):
            return cp.get(sec, key) if cp.has_option(sec, key) else fallback
        out["db_host"]   = _g("database", "db_host",   out["db_host"])
        out["db_port"]   = _g("database", "db_port",   out["db_port"])
        out["db_name"]   = _g("database", "db_name",   out["db_name"])
        out["db_user"]   = _g("database", "db_user",   out["db_user"])
        out["db_pass"]   = _g("database", "db_pass",   out["db_pass"])
        out["site_name"]     = _g("app",    "site_name",     out["site_name"])
        out["site_slogan"]   = _g("app",    "site_slogan",   out["site_slogan"])
        out["oauth_url"]     = _g("oauth",  "url",           out["oauth_url"])
        out["oauth_client_id"]       = _g("oauth", "client_id",       out["oauth_client_id"])
        out["oauth_client_secret"]   = _g("oauth", "client_secret",   out["oauth_client_secret"])
        out["payment_id"]     = _g("payment", "id",     out["payment_id"])
        out["payment_secret"] = _g("payment", "secret", out["payment_secret"])
    return out


@bp.route("/", methods=["GET", "POST"])
def index():
    # ===== POST =============================================================
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
        oauth_scopes = "openid profile email"
        user_redirect = request.form.get("oauth_redirect_uri", "").strip()

        if not user_redirect:
            oauth_redirect_uri = url_for("auth.oauth_callback", _external=True, _scheme="https")
        else:
            oauth_redirect_uri = user_redirect

        # --- 4) Payment ---
        payment_id = request.form.get("payment_id", "").strip()
        payment_secret = request.form.get("payment_secret", "")

        # --- 5) Admin ---
        admin_user = request.form.get("admin_user", "").strip()
        admin_email = request.form.get("admin_email", "").strip() or None
        admin_pass = request.form.get("admin_pass", "")

        # Build submitted defaults (for re-render on error)
        submitted = dict(_DEFAULT_DEFAULTS)
        submitted.update({
            "db_host": db_host, "db_port": db_port, "db_name": db_name,
            "db_user": db_user, "db_pass": db_pass,
            "site_name": site_name, "site_slogan": site_slogan,
            "oauth_url": oauth_url, "oauth_client_id": oauth_client_id,
            "oauth_client_secret": oauth_client_secret,
            "payment_id": payment_id, "payment_secret": payment_secret,
            "admin_user": admin_user, "admin_email": admin_email or "",
            "admin_pass": admin_pass,
        })

        # Validate required fields
        required = {
            "db_host": db_host, "db_port": db_port, "db_user": db_user,
            "db_name": db_name, "admin_user": admin_user, "admin_pass": admin_pass,
            "oauth_client_id": oauth_client_id, "oauth_client_secret": oauth_client_secret,
            "oauth_redirect_uri": oauth_redirect_uri,
            "payment_id": payment_id, "payment_secret": payment_secret,
        }
        missing = [k for k, v in required.items() if not v]
        if missing:
            return render_template(
                "install/index.html",
                error=f"以下字段必填: {', '.join(missing)}",
                defaults=submitted,
                partial_install=CONFIG_PATH.exists() and not is_installed(),
            )
        if len(admin_pass) < 8:
            return render_template(
                "install/index.html",
                error="管理员密码至少 8 位",
                defaults=submitted,
                partial_install=CONFIG_PATH.exists() and not is_installed(),
            )

        # HTTPS check (required for OAuth callbacks).
        # Behind a TLS-terminating proxy, Flask may see http:// internally;
        # auto-upgrade to https:// instead of failing.
        if not oauth_redirect_uri.startswith("https://"):
            if oauth_redirect_uri.startswith("http://"):
                oauth_redirect_uri = oauth_redirect_uri.replace("http://", "https://", 1)
            else:
                return render_template(
                    "install/index.html",
                    error="Redirect URI 必须使用 HTTPS。请通过 OpenResty / Caddy / Nginx 反代并配置 SSL 证书。",
                    defaults=submitted,
                    partial_install=CONFIG_PATH.exists() and not is_installed(),
                )

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
            current_app.logger.exception("install: _write_config failed")
            return render_template(
                "install/index.html",
                error=f"配置写入失败: {e}",
                defaults=submitted,
                partial_install=False,
            )

        # CRITICAL: refresh app config NOW so SQLAlchemy uses the freshly
        # written MySQL URI instead of the in-memory SQLite fallback.
        apply_to(current_app)
        try:
            db.engine.dispose()
        except Exception:
            pass

        # Build tables
        try:
            db.create_all()
        except Exception as e:
            current_app.logger.exception("install: db.create_all failed")
            return render_template(
                "install/index.html",
                error=f"数据库连接失败: {e}",
                defaults=submitted,
                partial_install=True,
            )

        # Create admin user
        if not User.query.filter_by(username=admin_user).first():
            u = User(username=admin_user, email=admin_email, is_admin=True)
            u.set_password(admin_pass)
            db.session.add(u)
            db.session.commit()

        _mark_installed()
        current_app.logger.info("Install completed: admin=%s", admin_user)
        return redirect(url_for("store.index"))

    # ===== GET ==============================================================
    partial = CONFIG_PATH.exists() and not is_installed()
    return render_template(
        "install/index.html",
        partial_install=partial,
        defaults=_form_defaults(),
    )


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
