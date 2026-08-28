"""auth blueprint — register / login / logout / OAuth."""
from __future__ import annotations

from flask import Blueprint, current_app, flash, redirect, render_template, request, session, url_for
from flask_login import login_user, logout_user

from ..extensions import db
from ..models import OAuthIdentity, User
from ..nodeloc import NodeLocOAuth, NodeLocError, new_state
from ..utils import verify_password

bp = Blueprint("auth", __name__)


# ── Register ──────────────────────────────────────────────────────────────
@bp.route("/register", methods=["GET", "POST"])
def register():
    if request.method == "POST":
        username = request.form.get("username", "").strip()
        email = request.form.get("email", "").strip() or None
        password = request.form.get("password", "")
        password2 = request.form.get("password2", "")

        if not username or not password:
            flash("用户名和密码不能为空", "danger")
            return render_template("auth/register.html")
        if password != password2:
            flash("两次密码不一致", "danger")
            return render_template("auth/register.html")
        if len(password) < 8:
            flash("密码至少 8 位", "danger")
            return render_template("auth/register.html")
        if User.query.filter_by(username=username).first():
            flash("用户名已被注册", "danger")
            return render_template("auth/register.html")
        if email and User.query.filter_by(email=email).first():
            flash("邮箱已被注册", "danger")
            return render_template("auth/register.html")

        user = User(username=username, email=email)
        user.set_password(password)
        db.session.add(user)
        db.session.commit()
        login_user(user)
        flash("注册成功，欢迎！", "success")
        return redirect(url_for("store.index"))
    return render_template("auth/register.html")


# ── Login ─────────────────────────────────────────────────────────────────
@bp.route("/login", methods=["GET", "POST"])
def login():
    if request.method == "POST":
        login_field = request.form.get("login", "").strip()  # username or email
        password = request.form.get("password", "")
        remember = request.form.get("remember") == "on"
        next_url = request.args.get("next", url_for("store.index"))

        user = User.query.filter_by(username=login_field).first()
        if not user:
            user = User.query.filter_by(email=login_field).first()
        if not user or not user.check_password(password):
            flash("用户名 / 邮箱或密码错误", "danger")
            return render_template("auth/login.html")
        if not user.is_active:
            flash("账号已被禁用", "warning")
            return render_template("auth/login.html")
        login_user(user, remember=remember)
        user.last_login_at = db.func.now()
        db.session.commit()
        return redirect(next_url)
    return render_template("auth/login.html")


@bp.route("/logout")
def logout():
    logout_user()
    return redirect(url_for("store.index"))


# ── OAuth ─────────────────────────────────────────────────────────────────
@bp.route("/oauth/initiate")
def oauth_initiate():
    oauth = _oauth()
    if not oauth.is_configured():
        flash("NodeLoc OAuth 未配置，请联系管理员", "warning")
        return redirect(url_for("auth.login"))
    state = new_state()
    session.permanent = True
    session["oauth_state"] = state
    session.modified = True
    return redirect(oauth.build_authorize_url(state))


@bp.route("/oauth/callback")
def oauth_callback():
    oauth = _oauth()
    error = request.args.get("error")
    if error:
        flash(f"授权失败: {error}", "danger")
        return redirect(url_for("auth.login"))

    state = request.args.get("state")
    code = request.args.get("code")
    if not state or state != session.pop("oauth_state", None):
        flash("OAuth 状态验证失败，请重试", "danger")
        return redirect(url_for("auth.login"))

    if not code:
        flash("OAuth 回调缺少授权码，请重新授权", "danger")
        return redirect(url_for("auth.login"))

    try:
        token = oauth.exchange_code(code)
        nl_user = oauth.fetch_userinfo(token)
    except NodeLocError as e:
        flash(f"NodeLoc 连接失败: {e}", "danger")
        return redirect(url_for("auth.login"))

    # Look up or create user
    user = _find_or_create_oauth_user(nl_user, token)
    login_user(user)

    # If account not bound, redirect to bind page
    if not user.has_password() and not user.oauth_uid:
        return redirect(url_for("user.bind_oauth"))

    flash(f"欢迎回来，{user.username}！", "success")
    return redirect(url_for("store.index"))


# ── Helpers ───────────────────────────────────────────────────────────────
def _oauth() -> NodeLocOAuth:
    return NodeLocOAuth(
        base_url=current_app.config["NODELOC_URL"],
        client_id=current_app.config["NODELOC_CLIENT_ID"],
        client_secret=current_app.config["NODELOC_CLIENT_SECRET"],
        redirect_uri=current_app.config["NODELOC_REDIRECT_URI"],
    )


def _find_or_create_oauth_user(nl_user, token) -> User:
    provider_uid = str(nl_user.id)
    identity = OAuthIdentity.query.filter_by(
        provider="nodeloc", provider_uid=provider_uid
    ).first()
    user = identity.user if identity else None

    # Migration compatibility: adopt a legacy binding only when its exact
    # provider/UID pair matches. Never bind by username or email.
    if not user:
        user = User.query.filter_by(
            oauth_provider="nodeloc", oauth_uid=provider_uid
        ).first()
        if user:
            identity = OAuthIdentity(
                user_id=user.id,
                provider="nodeloc",
                provider_uid=provider_uid,
            )
            db.session.add(identity)

    if not user:
        # Brand-new user
        base = (nl_user.username or f"user{nl_user.id}")[:64]
        suffix = 0
        candidate = base
        while User.query.filter_by(username=candidate).first():
            suffix += 1
            candidate = f"{base}_{suffix}"
        user = User(username=candidate)
        db.session.add(user)
        db.session.flush()  # get id
        identity = OAuthIdentity(
            user_id=user.id,
            provider="nodeloc",
            provider_uid=provider_uid,
        )
        db.session.add(identity)

    identity.username = nl_user.username
    identity.display_name = nl_user.name
    identity.avatar_url = nl_user.avatar_url
    identity.scope = token.scope or ""
    identity.access_token = token.access_token
    identity.refresh_token = token.refresh_token

    # Keep legacy columns synchronized during the compatibility window.
    user.oauth_provider = "nodeloc"
    user.oauth_uid = provider_uid
    user.oauth_username = nl_user.username
    user.oauth_name = nl_user.name
    user.oauth_avatar = nl_user.avatar_url
    user.oauth_trust_level = nl_user.trust_level
    user.oauth_scope = token.scope or ""
    user.oauth_has_email = "email" in (token.scope or "").split()
    user.oauth_access_token = token.access_token
    user.oauth_refresh_token = token.refresh_token
    user.oauth_token_expires_at = None  # not needed for read-only profile
    user.last_login_at = db.func.now()
    db.session.commit()
    return user
