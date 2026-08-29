"""Flask extensions — kept in a single module to avoid circular imports."""
from __future__ import annotations

from flask_sqlalchemy import SQLAlchemy
from flask_login import LoginManager
from flask_wtf.csrf import CSRFProtect
from sqlalchemy import create_engine


db = SQLAlchemy()
login_manager = LoginManager()
csrf = CSRFProtect()


def rebind_database(app):
    """Recreate the default engine after the installer changes the DB URI.

    Flask-SQLAlchemy creates its engines during ``init_app``. Updating
    ``SQLALCHEMY_DATABASE_URI`` later does not update the existing engine, and
    disposing that engine only closes its pool. The installer must therefore
    replace the cached default engine before creating tables.
    """
    db.session.remove()

    engines = db._app_engines[app]
    old_engine = engines.get(None)

    options = dict(app.config.get("SQLALCHEMY_ENGINE_OPTIONS") or {})
    options["url"] = app.config["SQLALCHEMY_DATABASE_URI"]
    db._apply_driver_defaults(options, app)
    new_engine = create_engine(**options)
    engines[None] = new_engine

    if old_engine is not None:
        old_engine.dispose()

    return new_engine

login_manager.login_view = "auth.login"
login_manager.login_message = "请先登录后再继续。"
login_manager.login_message_category = "warning"
