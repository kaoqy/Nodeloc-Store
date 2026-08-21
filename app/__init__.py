"""Flask application factory."""
from __future__ import annotations

import logging
from pathlib import Path

# Keep these lightweight imports at module level; everything Flask-specific
# is deferred into create_app() so other app.* modules can be imported in a
# stdlib-only environment (e.g. scripts/smoke_test.py).

from .config import Config, apply_to, is_installed


def create_app() -> "Flask":
    # Defer Flask imports so this module can be imported in stdlib-only envs.
    from datetime import datetime
    from flask import Flask, redirect, render_template, request, url_for
    from .extensions import csrf, db, login_manager
    app = Flask(
        __name__,
        instance_path=str(Path(__file__).resolve().parent.parent / "instance"),
        instance_relative_config=False,
    )
    # Pull defaults from Config (engine opts, session, scheme, ...)
    app.config.from_object(Config)
    # Overlay instance/config.ini values (freshly read)
    apply_to(app)

    Path(app.instance_path).mkdir(parents=True, exist_ok=True)
    (Path(app.instance_path) / "uploads").mkdir(parents=True, exist_ok=True)

    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s [%(levelname)s] %(name)s: %(message)s",
    )

    db.init_app(app)
    login_manager.init_app(app)
    csrf.init_app(app)

    from .models import User  # noqa: WPS433

    @login_manager.user_loader
    def load_user(user_id: str):
        try:
            return db.session.get(User, int(user_id))
        except (ValueError, TypeError):
            return None

    # ---------- Blueprints ----------
    from .blueprints.install import bp as install_bp
    from .blueprints.auth import bp as auth_bp
    from .blueprints.store import bp as store_bp
    from .blueprints.user import bp as user_bp
    from .blueprints.payment import bp as payment_bp
    from .blueprints.admin import bp as admin_bp
    from .blueprints.api import bp as api_bp

    app.register_blueprint(install_bp)
    app.register_blueprint(auth_bp, url_prefix="/auth")
    app.register_blueprint(store_bp)
    app.register_blueprint(user_bp, url_prefix="/user")
    app.register_blueprint(payment_bp, url_prefix="/payment")
    app.register_blueprint(admin_bp, url_prefix="/admin")
    app.register_blueprint(api_bp, url_prefix="/api")

    # Reload config on every request (cheap, in-memory) so changes
    # from /admin/settings take effect immediately.
    @app.before_request
    def _refresh_config():
        apply_to(app)

    # ---------- Install gate ----------
    @app.before_request
    def _install_gate():
        if not is_installed():
            allowed = ("install.", "static")
            endpoint = request.endpoint or ""
            if not any(endpoint.startswith(p) for p in allowed):
                return redirect(url_for("install.index"))

    # ---------- Context ----------
    @app.context_processor
    def _inject_globals():
        from datetime import datetime as _dt
        return {
            "site_name": app.config.get("SITE_NAME", "NodeLoc Store"),
            "site_slogan": app.config.get("SITE_SLOGAN", ""),
            "currency": app.config.get("CURRENCY", "积分"),
            "current_year": _dt.utcnow().year,
            "now": _dt.utcnow,
        }

    # ---------- Error handlers ----------
    @app.errorhandler(404)
    def _not_found(e):
        return render_template("errors/404.html"), 404

    @app.errorhandler(403)
    def _forbidden(e):
        return render_template("errors/403.html"), 403

    @app.errorhandler(500)
    def _server_error(e):
        app.logger.exception("Unhandled error: %s", e)
        return render_template("errors/500.html"), 500

    register_cli(app)
    return app


def register_cli(app: Flask) -> None:
    import click
    from .models import User

    @app.cli.command("init-db")
    def init_db():
        """Create database tables (used by entrypoint when DB is up)."""
        with app.app_context():
            db.create_all()
            click.echo("✔ Tables ensured.")

    @app.cli.command("create-admin")
    @click.option("--username", required=True)
    @click.option("--email", required=False, default=None)
    @click.option("--password", required=True)
    def create_admin(username, email, password):
        with app.app_context():
            if User.query.filter_by(username=username).first():
                click.echo("User already exists")
                return
            u = User(username=username, email=email, is_admin=True)
            u.set_password(password)
            db.session.add(u)
            db.session.commit()
            click.echo(f"✔ Admin '{username}' created.")
