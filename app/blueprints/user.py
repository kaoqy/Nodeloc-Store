"""user blueprint — profile, change password, bind OAuth."""
from __future__ import annotations

from datetime import date, timedelta

from flask import Blueprint, current_app, flash, redirect, render_template, request, session, url_for
from flask_login import current_user, login_required
from sqlalchemy.exc import IntegrityError

from ..extensions import db
from ..models import AppSetting, CheckIn, Order
from ..nodeloc import NodeLocOAuth, NodeLocError
from ..utils import log_action

bp = Blueprint("user", __name__)


@bp.route("/profile")
@login_required
def profile():
    recent_checkins = CheckIn.query.filter_by(user_id=current_user.id).order_by(
        CheckIn.checkin_date.desc()
    ).limit(14).all()
    today = date.today()
    checked_in_today = current_user.last_checkin_date == today
    return render_template(
        "user/profile.html",
        recent_checkins=recent_checkins,
        checked_in_today=checked_in_today,
    )


@bp.route("/check-in", methods=["POST"])
@login_required
def check_in():
    """Award the configured daily reward once per local calendar day."""
    if AppSetting.get("checkin_enabled", "1") != "1":
        flash("签到功能当前未开启", "warning")
        return redirect(url_for("user.profile"))

    today = date.today()
    if current_user.last_checkin_date == today:
        flash("今天已经签到过了，明天再来吧", "info")
        return redirect(url_for("user.profile"))

    try:
        base_reward = max(0, int(AppSetting.get("checkin_reward", "5") or 5))
        streak_bonus = max(0, int(AppSetting.get("checkin_streak_bonus", "1") or 1))
        reward_cap = max(base_reward, int(AppSetting.get("checkin_reward_cap", "20") or 20))
    except (TypeError, ValueError):
        base_reward, streak_bonus, reward_cap = 5, 1, 20

    if current_user.last_checkin_date == today - timedelta(days=1):
        streak = current_user.consecutive_checkins + 1
    else:
        streak = 1

    reward = min(reward_cap, base_reward + max(0, streak - 1) * streak_bonus)
    record = CheckIn(
        user_id=current_user.id,
        checkin_date=today,
        reward_points=reward,
        consecutive_days=streak,
    )
    current_user.points += reward
    current_user.consecutive_checkins = streak
    current_user.total_checkins += 1
    current_user.last_checkin_date = today
    db.session.add(record)

    try:
        db.session.commit()
    except IntegrityError:
        db.session.rollback()
        flash("今天已经签到过了", "info")
        return redirect(url_for("user.profile"))

    log_action(
        "user.checkin",
        target=str(current_user.id),
        detail=f"reward={reward}, streak={streak}",
    )
    flash(f"签到成功，获得 {reward} 积分，已连续签到 {streak} 天", "success")
    return redirect(url_for("user.profile"))


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
        stored_state = session.get("oauth_bind_state", "")
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

    state = secrets.token_urlsafe(32)
    session["oauth_bind_state"] = state
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
