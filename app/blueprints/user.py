"""user blueprint — profile, change password, bind OAuth."""
from __future__ import annotations

from flask import Blueprint, current_app, flash, redirect, render_template, request, url_for
from flask_login import current_user, login_required

from ..extensions import db
from ..models import Order
from ..nodeloc import NodeLocOAuth, NodeLocError
from ..utils import log_action

bp = Blueprint("user", __name__)


@bp.route("/profile")
@login_required
def profile():
    return render_template("user/profile.html")


@bp.route("/profile/edit", methods=["GET", "POST"])
@login_required
def edit_profile():
    if request.method == "POST":
        new_username = request.form.get("username", "").strip()
        new_email = request.form.get("email", "").strip() or None
        if not new_username:
            flash("用户名不能为空", "danger")
            return render_template("user/edit_profile.html")
        existing = db.session.query(db.exists().where(
            db.and_(db.text("id != :uid"), db.text("username = :u"))
        )).params(uid=current_user.id, u=new_username).scalar()
        if existing:
            flash("用户名已被使用", "danger")
            return render_template("user/edit_profile.html")
        if new_email:
            existing_email = db.session.query(db.exists().where(
                db.and_(db.text("id != :uid"), db.text("email = :e"))
            )).params(uid=current_user.id, e=new_email).scalar()
            if existing_email:
                flash("邮箱已被使用", "danger")
                return render_template("user/edit_profile.html")
        current_user.username = new_username
        current_user.email = new_email
        db.session.commit()
        flash("资料已更新", "success")
        log_action("profile.edit", target=str(current_user.id))
        return redirect(url_for("user.profile"))
    return render_template("user/edit_profile.html")


@bp.route("/change-password", methods=["GET", "POST"])
@login_required
def change_password():
    if request.method == "POST":
        old_pw = request.form.get("old_password", "")
        new_pw = request.form.get("new_password", "")
        new_pw2 = request.form.get("new_password2", "")
        if not current_user.check_password(old_pw):
            flash("当前密码错误", "danger")
            return render_template("user/change_password.html")
        if new_pw != new_pw2:
            flash("两次新密码不一致", "danger")
            return render_template("user/change_password.html")
        if len(new_pw) < 8:
            flash("新密码至少 8 位", "danger")
            return render_template("user/change_password.html")
        current_user.set_password(new_pw)
        db.session.commit()
        flash("密码已修改，请重新登录", "success")
        return redirect(url_for("auth.login"))
    return render_template("user/change_password.html")


@bp.route("/bind-oauth", methods=["GET", "POST"])
@login_required
def bind_oauth():
    oauth = _oauth()
    if request.method == "POST":
        code = request.form.get("code", "").strip()
        state = request.form.get("state", "").strip()
        stored_state = request.session.get("oauth_bind_state", "")
        if not code or state != stored_state:
            flash("OAuth 状态无效，请重试", "danger")
            return redirect(url_for("user.bind_oauth"))

        try:
            token = oauth.exchange_code(code)
            nl_user = oauth.fetch_userinfo(token)
        except NodeLocError as e:
            flash(f"绑定失败: {e}", "danger")
            return redirect(url_for("user.bind_oauth"))

        # Check if another user already bound this NodeLoc account
        existing = db.session.query(db.exists().where(
            db.and_(
                db.text("oauth_provider = 'nodeloc'"),
                db.text("oauth_uid = :uid"),
                db.text("id != :me"),
            )
        )).params(uid=str(nl_user.id), me=current_user.id).scalar()
        if existing:
            flash("该 NodeLoc 账号已绑定到其他账户", "warning")
            return redirect(url_for("user.profile"))

        current_user.oauth_provider = "nodeloc"
        current_user.oauth_uid = str(nl_user.id)
        current_user.oauth_username = nl_user.username
        current_user.oauth_name = nl_user.name
        current_user.oauth_avatar = nl_user.avatar_url
        current_user.oauth_trust_level = nl_user.trust_level
        current_user.oauth_scope = token.scope or ""
        current_user.oauth_has_email = "email" in (token.scope or "").split()
        current_user.oauth_access_token = token.access_token
        current_user.oauth_refresh_token = token.refresh_token
        db.session.commit()
        flash("NodeLoc 账号已绑定", "success")
        return redirect(url_for("user.profile"))

    state = request.session["oauth_bind_state"] = secrets.token_urlsafe(32)
    auth_url = oauth.build_authorize_url(state)
    return render_template("user/bind_oauth.html", auth_url=auth_url)


@bp.route("/orders")
@login_required
def orders():
    page = request.args.get("page", 1, type=int)
    pagination = Order.query.filter_by(user_id=current_user.id).order_by(
        Order.created_at.desc()
    ).paginate(page=page, per_page=20, error_out=False)
    return render_template("user/orders.html", pagination=pagination)


@bp.route("/orders/<order_no>")
@login_required
def order_detail(order_no):
    order = Order.query.filter_by(order_no=order_no, user_id=current_user.id).first_or_404()
    return render_template("payment/order_detail.html", order=order)


# ── helpers ──────────────────────────────────────────────────────────────
import secrets


def _oauth() -> NodeLocOAuth:
    return NodeLocOAuth(
        base_url=current_app.config["NODELOC_URL"],
        client_id=current_app.config["NODELOC_CLIENT_ID"],
        client_secret=current_app.config["NODELOC_CLIENT_SECRET"],
        redirect_uri=current_app.config["NODELOC_REDIRECT_URI"],
    )
