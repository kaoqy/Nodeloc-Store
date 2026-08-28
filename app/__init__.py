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

    # Keep existing installations compatible with newly introduced account,
    # role and check-in fields. create_all() creates new tables but does not
    # add columns to an existing users table, so apply a small idempotent
    # compatibility migration during startup.
    with app.app_context():
        from sqlalchemy import inspect, text

        inspector = inspect(db.engine)
        if "users" in inspector.get_table_names():
            existing_columns = {column["name"] for column in inspector.get_columns("users")}
            statements = []

            if "role" not in existing_columns:
                statements.append("ALTER TABLE users ADD COLUMN role VARCHAR(32) NOT NULL DEFAULT 'user'")
            if "points" not in existing_columns:
                statements.append("ALTER TABLE users ADD COLUMN points INTEGER NOT NULL DEFAULT 0")
            if "consecutive_checkins" not in existing_columns:
                statements.append("ALTER TABLE users ADD COLUMN consecutive_checkins INTEGER NOT NULL DEFAULT 0")
            if "total_checkins" not in existing_columns:
                statements.append("ALTER TABLE users ADD COLUMN total_checkins INTEGER NOT NULL DEFAULT 0")
            if "last_checkin_date" not in existing_columns:
                statements.append("ALTER TABLE users ADD COLUMN last_checkin_date DATE NULL")

            for statement in statements:
                db.session.execute(text(statement))
            if statements:
                db.session.commit()

            # Preserve the meaning of legacy is_admin records while moving to
            # role-based permissions. SQLite and MySQL/MariaDB support this
            # portable UPDATE statement.
            db.session.execute(text(
                "UPDATE users SET role = 'super_admin' "
                "WHERE is_admin = 1 AND (role IS NULL OR role = '' OR role = 'user')"
            ))
            db.session.commit()

        if "products" in inspector.get_table_names():
            product_columns = {
                column["name"] for column in inspector.get_columns("products")
            }
            product_statements = []
            if "product_type" not in product_columns:
                product_statements.append(
                    "ALTER TABLE products ADD COLUMN product_type VARCHAR(32) NOT NULL DEFAULT 'card'"
                )
            if "delivery_instructions" not in product_columns:
                product_statements.append(
                    "ALTER TABLE products ADD COLUMN delivery_instructions TEXT NULL"
                )
            if "require_contact" not in product_columns:
                product_statements.append(
                    "ALTER TABLE products ADD COLUMN require_contact BOOLEAN NOT NULL DEFAULT 0"
                )
            if "is_archived" not in product_columns:
                product_statements.append(
                    "ALTER TABLE products ADD COLUMN is_archived BOOLEAN NOT NULL DEFAULT 0"
                )
            if "archived_at" not in product_columns:
                product_statements.append(
                    "ALTER TABLE products ADD COLUMN archived_at DATETIME NULL"
                )
            for statement in product_statements:
                db.session.execute(text(statement))
            if product_statements:
                db.session.commit()

        if "orders" in inspector.get_table_names():
            order_columns = {
                column["name"] for column in inspector.get_columns("orders")
            }
            order_statements = []
            if "fulfillment_status" not in order_columns:
                order_statements.append(
                    "ALTER TABLE orders ADD COLUMN fulfillment_status VARCHAR(32) NOT NULL DEFAULT 'pending'"
                )
            if "customer_contact" not in order_columns:
                order_statements.append(
                    "ALTER TABLE orders ADD COLUMN customer_contact VARCHAR(255) NULL"
                )
            if "customer_note" not in order_columns:
                order_statements.append(
                    "ALTER TABLE orders ADD COLUMN customer_note TEXT NULL"
                )
            if "delivery_content" not in order_columns:
                order_statements.append(
                    "ALTER TABLE orders ADD COLUMN delivery_content TEXT NULL"
                )
            if "delivery_note" not in order_columns:
                order_statements.append(
                    "ALTER TABLE orders ADD COLUMN delivery_note TEXT NULL"
                )
            for statement in order_statements:
                db.session.execute(text(statement))
            if order_statements:
                db.session.commit()

        # Ensure newly added tables, including checkins, exist after upgrading.
        db.create_all()

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
    # NodeLoc sends payment notifications from its own server and therefore
    # cannot provide a browser-session CSRF token. Callback authenticity is
    # verified separately using the NodeLoc HMAC signature.
    csrf.exempt(payment_bp)
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
                return redirect(url_for("install.db_step"))

    # Administrative statistics and audit pages contain live operational
    # data. Prevent browsers and reverse proxies from serving stale copies
    # when users revisit or refresh those pages.
    @app.after_request
    def _disable_dynamic_page_cache(response):
        endpoint = request.endpoint or ""
        if endpoint.startswith("admin."):
            response.headers["Cache-Control"] = "no-store, no-cache, must-revalidate, max-age=0"
            response.headers["Pragma"] = "no-cache"
            response.headers["Expires"] = "0"
        return response

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
        return render_template("errors/generic.html", code=404, message="页面不存在"), 404

    @app.errorhandler(403)
    def _forbidden(e):
        return render_template("errors/generic.html", code=403, message="没有权限访问此页面"), 403

    @app.errorhandler(500)
    def _server_error(e):
        app.logger.exception("Unhandled error: %s", e)
        return render_template("errors/generic.html", code=500, message="服务器开小差了"), 500

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
            u = User(
                username=username,
                email=email,
                is_admin=True,
                role="super_admin",
            )
            u.set_password(password)
            db.session.add(u)
            db.session.commit()
            click.echo(f"✔ Admin '{username}' created.")
