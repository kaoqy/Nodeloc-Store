from __future__ import annotations

from configparser import RawConfigParser
from sqlalchemy import inspect, text

from flask import Blueprint, current_app, redirect, render_template, request, url_for
from flask_login import current_user

from ..extensions import db
from ..models import AppSetting, User
from ..config import INSTANCE_DIR, CONFIG_PATH, apply_to
from ..utils import log_action


bp = Blueprint("install", __name__, url_prefix="/install")


# --------------------------------------------------------------------------- #
# 第一阶段：数据库连接
# --------------------------------------------------------------------------- #
@bp.route("/db", methods=["GET", "POST"])
def db_step():
    """Phase 1: connect to database and create tables."""
    # 如果已经完成安装，直接跳转商店
    if AppSetting.is_installed():
        return redirect(url_for("store.index"))

    # 如果数据库已经配置好（表已存在），跳转到第二阶段
    if AppSetting.is_db_configured():
        # 检查表是否存在
        inspector = inspect(db.engine)
        if "app_settings" in inspector.get_table_names():
            return redirect(url_for("install.setup_step"))

    error = None
    defaults = _load_db_defaults()

    if request.method == "POST":
        db_host = request.form.get("db_host", "").strip()
        db_port = request.form.get("db_port", "3306").strip()
        db_user = request.form.get("db_user", "").strip()
        db_pass = request.form.get("db_pass", "")
        db_name = request.form.get("db_name", "").strip()

        # 基本验证
        missing = []
        if not db_host:
            missing.append("数据库主机")
        if not db_port:
            missing.append("端口")
        if not db_user:
            missing.append("用户名")
        if not db_name:
            missing.append("数据库名")
        if missing:
            error = f"请填写: {', '.join(missing)}"
            return render_template(
                "install/db.html",
                error=error,
                defaults={
                    "db_host": db_host,
                    "db_port": db_port,
                    "db_user": db_user,
                    "db_pass": db_pass,
                    "db_name": db_name,
                },
            )

        # 尝试连接数据库并建表
        try:
            # 写入临时 config.ini（仅 DB 部分）
            _write_db_config(db_host, db_port, db_user, db_pass, db_name)

            # 刷新 app config
            apply_to(current_app)

            # 重新绑定数据库引擎
            from ..extensions import rebind_database
            rebind_database(current_app._get_current_object())

            # 检查数据库连接
            db.session.execute(text("SELECT 1"))

            # 创建所有表（如果不存在）
            db.create_all()

            # 在 app_settings 中记录安装进度
            AppSetting.set("install_step", "db_done")

            current_app.logger.info(
                "install: phase 1 complete, connected to %s@%s:%s/%s",
                db_user, db_host, db_port, db_name
            )

            # 跳转到第二阶段
            return redirect(url_for("install.setup_step"))

        except Exception as e:
            current_app.logger.exception("install: phase 1 failed")
            error = f"数据库连接失败: {e}"
            return render_template(
                "install/db.html",
                error=error,
                defaults={
                    "db_host": db_host,
                    "db_port": db_port,
                    "db_user": db_user,
                    "db_pass": db_pass,
                    "db_name": db_name,
                },
            )

    # GET 请求
    return render_template(
        "install/db.html",
        error=None,
        defaults=defaults,
    )


# --------------------------------------------------------------------------- #
# 第二阶段：OAuth + Payment + 管理员
# --------------------------------------------------------------------------- #
@bp.route("/setup", methods=["GET", "POST"])
def setup_step():
    """Phase 2: configure OAuth, Payment, and create admin user."""
    # 如果已经完成安装，跳转商店
    if AppSetting.is_installed():
        return redirect(url_for("store.index"))

    # 检查是否已完成第一阶段
    if not AppSetting.is_db_configured():
        return redirect(url_for("install.db_step"))

    # 确保数据库表存在
    inspector = inspect(db.engine)
    if "app_settings" not in inspector.get_table_names():
        return redirect(url_for("install.db_step"))

    error = None
    defaults = _load_setup_defaults()

    if request.method == "POST":
        # --- OAuth ---
        oauth_url = request.form.get("oauth_url", "https://www.nodeloc.com").strip().rstrip("/")
        oauth_client_id = request.form.get("oauth_client_id", "").strip()
        oauth_client_secret = request.form.get("oauth_client_secret", "")

        # --- Payment ---
        payment_id = request.form.get("payment_id", "").strip()
        payment_secret = request.form.get("payment_secret", "")

        # --- Admin ---
        admin_user = request.form.get("admin_user", "").strip()
        admin_email = request.form.get("admin_email", "").strip() or None
        admin_pass = request.form.get("admin_pass", "")

        # --- Site ---
        site_name = request.form.get("site_name", "NodeLoc Store").strip()
        site_slogan = request.form.get("site_slogan", "").strip()

        # 验证必填项
        missing = []
        if not oauth_client_id:
            missing.append("OAuth Client ID")
        if not oauth_client_secret:
            missing.append("OAuth Client Secret")
        if not payment_id:
            missing.append("Payment ID")
        if not payment_secret:
            missing.append("Payment Secret")
        if not admin_user:
            missing.append("管理员用户名")
        if not admin_pass:
            missing.append("管理员密码")
        if missing:
            error = f"请填写: {', '.join(missing)}"
            return render_template(
                "install/setup.html",
                error=error,
                defaults={
                    "site_name": site_name,
                    "site_slogan": site_slogan,
                    "oauth_url": oauth_url,
                    "oauth_client_id": oauth_client_id,
                    "oauth_client_secret": oauth_client_secret,
                    "payment_id": payment_id,
                    "payment_secret": payment_secret,
                    "admin_user": admin_user,
                    "admin_email": admin_email or "",
                    "admin_pass": admin_pass,
                },
            )

        if len(admin_pass) < 8:
            error = "管理员密码至少 8 位"
            return render_template(
                "install/setup.html",
                error=error,
                defaults={
                    "site_name": site_name,
                    "site_slogan": site_slogan,
                    "oauth_url": oauth_url,
                    "oauth_client_id": oauth_client_id,
                    "oauth_client_secret": oauth_client_secret,
                    "payment_id": payment_id,
                    "payment_secret": payment_secret,
                    "admin_user": admin_user,
                    "admin_email": admin_email or "",
                    "admin_pass": admin_pass,
                },
            )

        # HTTPS 检查：redirect_uri 必须 HTTPS
        redirect_uri = url_for("auth.oauth_callback", _external=True, _scheme="https")
        if not redirect_uri.startswith("https://"):
            error = "请通过 HTTPS 访问安装页面，或配置反向代理提供 SSL 证书"
            return render_template(
                "install/setup.html",
                error=error,
                defaults={
                    "site_name": site_name,
                    "site_slogan": site_slogan,
                    "oauth_url": oauth_url,
                    "oauth_client_id": oauth_client_id,
                    "oauth_client_secret": oauth_client_secret,
                    "payment_id": payment_id,
                    "payment_secret": payment_secret,
                    "admin_user": admin_user,
                    "admin_email": admin_email or "",
                    "admin_pass": admin_pass,
                },
            )

        # 写入 config.ini（完整配置，包含 OAuth/Payment）
        try:
            _write_full_config(
                site_name=site_name,
                site_slogan=site_slogan,
                oauth_url=oauth_url,
                oauth_client_id=oauth_client_id,
                oauth_client_secret=oauth_client_secret,
                oauth_redirect_uri=redirect_uri,
                oauth_scopes="openid profile email",
                payment_id=payment_id,
                payment_secret=payment_secret,
            )
        except Exception as e:
            current_app.logger.exception("install: phase 2 write config failed")
            error = f"配置写入失败: {e}"
            return render_template(
                "install/setup.html",
                error=error,
                defaults={
                    "site_name": site_name,
                    "site_slogan": site_slogan,
                    "oauth_url": oauth_url,
                    "oauth_client_id": oauth_client_id,
                    "oauth_client_secret": oauth_client_secret,
                    "payment_id": payment_id,
                    "payment_secret": payment_secret,
                    "admin_user": admin_user,
                    "admin_email": admin_email or "",
                    "admin_pass": admin_pass,
                },
            )

        # 刷新 app config（让 OAuth/Payment 配置生效）
        apply_to(current_app)

        # 创建管理员用户
        try:
            existing = User.query.filter_by(username=admin_user).first()
            if existing:
                # 如果已存在但未设置密码，更新密码
                if not existing.has_password():
                    existing.set_password(admin_pass)
                    db.session.commit()
                    current_app.logger.info("install: updated existing admin %s password", admin_user)
            else:
                u = User(username=admin_user, email=admin_email, is_admin=True)
                u.set_password(admin_pass)
                db.session.add(u)
                db.session.commit()
                current_app.logger.info("install: created admin user %s", admin_user)

            # 标记安装完成
            AppSetting.set("install_step", "complete")
            _mark_install_complete()
            apply_to(current_app)

            # 记录审计日志
            log_action("install_complete", target=admin_user, detail=f"管理员 {admin_user} 完成安装")

            current_app.logger.info("install: phase 2 complete, store ready")

            # 跳转到商店首页
            return redirect(url_for("store.index"))

        except Exception as e:
            current_app.logger.exception("install: phase 2 admin creation failed")
            error = f"创建管理员失败: {e}"
            return render_template(
                "install/setup.html",
                error=error,
                defaults={
                    "site_name": site_name,
                    "site_slogan": site_slogan,
                    "oauth_url": oauth_url,
                    "oauth_client_id": oauth_client_id,
                    "oauth_client_secret": oauth_client_secret,
                    "payment_id": payment_id,
                    "payment_secret": payment_secret,
                    "admin_user": admin_user,
                    "admin_email": admin_email or "",
                    "admin_pass": admin_pass,
                },
            )

    # GET 请求
    return render_template(
        "install/setup.html",
        error=None,
        defaults=defaults,
    )


# --------------------------------------------------------------------------- #
# 辅助函数
# --------------------------------------------------------------------------- #
def _load_db_defaults() -> dict:
    """从 config.ini 加载 DB 配置默认值（如果存在）"""
    defaults = {
        "db_host": "localhost",
        "db_port": "3306",
        "db_user": "",
        "db_pass": "",
        "db_name": "nodeloc_store",
    }
    if CONFIG_PATH.exists():
        cp = RawConfigParser()
        cp.read(CONFIG_PATH, encoding="utf-8")
        if cp.has_section("database"):
            for k in defaults.keys():
                if cp.has_option("database", k):
                    defaults[k] = cp.get("database", k)
    return defaults


def _load_setup_defaults() -> dict:
    """从 config.ini 加载配置默认值（如果存在）"""
    defaults = {
        "site_name": "NodeLoc Store",
        "site_slogan": "",
        "oauth_url": "https://www.nodeloc.com",
        "oauth_client_id": "",
        "oauth_client_secret": "",
        "payment_id": "",
        "payment_secret": "",
        "admin_user": "",
        "admin_email": "",
        "admin_pass": "",
    }
    if CONFIG_PATH.exists():
        cp = RawConfigParser()
        cp.read(CONFIG_PATH, encoding="utf-8")
        for section, mapping in [
            ("app", {"site_name": "site_name", "site_slogan": "site_slogan"}),
            ("oauth", {"url": "oauth_url", "client_id": "oauth_client_id"}),
            ("payment", {"id": "payment_id", "secret": "***"}),
        ]:
            if cp.has_section(section):
                for ini_key, default_key in mapping.items():
                    if cp.has_option(section, ini_key):
                        defaults[default_key] = cp.get(section, ini_key)
    return defaults


def _write_db_config(host: str, port: str, user: str, pwd: str, name: str) -> None:
    """写入数据库配置到 config.ini（只写 DB 部分，保留其他已有配置）"""
    cfg = RawConfigParser()
    if CONFIG_PATH.exists():
        cfg.read(CONFIG_PATH, encoding="utf-8")

    for section in ["database", "app", "oauth", "payment"]:
        if not cfg.has_section(section):
            cfg.add_section(section)

    cfg.set("database", "db_host", host)
    cfg.set("database", "db_port", port)
    cfg.set("database", "db_user", user)
    cfg.set("database", "db_pass", pwd)
    cfg.set("database", "db_name", name)

    # 如果 app.installed 还没写，初始化为 0
    if cfg.has_section("app") and not cfg.has_option("app", "installed"):
        cfg.set("app", "installed", "0")
    if not cfg.has_section("app"):
        cfg.add_section("app")
        cfg.set("app", "installed", "0")

    INSTANCE_DIR.mkdir(parents=True, exist_ok=True)
    with open(CONFIG_PATH, "w", encoding="utf-8") as f:
        cfg.write(f)


def _write_full_config(
    site_name: str,
    site_slogan: str,
    oauth_url: str,
    oauth_client_id: str,
    oauth_client_secret: str,
    oauth_redirect_uri: str,
    oauth_scopes: str,
    payment_id: str,
    payment_secret: str,
) -> None:
    """写入完整配置到 config.ini（包含 OAuth/Payment/App）"""
    cfg = RawConfigParser()
    if CONFIG_PATH.exists():
        cfg.read(CONFIG_PATH, encoding="utf-8")

    for section in ["app", "oauth", "payment", "database"]:
        if not cfg.has_section(section):
            cfg.add_section(section)

    cfg.set("app", "site_name", site_name)
    cfg.set("app", "site_slogan", site_slogan)
    cfg.set("app", "installed", "0")  # 由数据库标志控制

    cfg.set("oauth", "url", oauth_url)
    cfg.set("oauth", "client_id", oauth_client_id)
    cfg.set("oauth", "client_secret", oauth_client_secret)
    cfg.set("oauth", "redirect_uri", oauth_redirect_uri)
    cfg.set("oauth", "scopes", oauth_scopes)

    cfg.set("payment", "id", payment_id)
    cfg.set("payment", "secret", payment_secret)

    INSTANCE_DIR.mkdir(parents=True, exist_ok=True)
    with open(CONFIG_PATH, "w", encoding="utf-8") as f:
        cfg.write(f)


def _mark_install_complete() -> None:
    """Persist the file-based installation flag used by the global request gate."""
    cfg = RawConfigParser()
    if CONFIG_PATH.exists():
        cfg.read(CONFIG_PATH, encoding="utf-8")
    if not cfg.has_section("app"):
        cfg.add_section("app")
    cfg.set("app", "installed", "1")
    INSTANCE_DIR.mkdir(parents=True, exist_ok=True)
    with open(CONFIG_PATH, "w", encoding="utf-8") as f:
        cfg.write(f)
