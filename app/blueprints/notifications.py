"""notifications blueprint — user notifications."""
from __future__ import annotations

from flask import Blueprint, redirect, render_template, request, url_for
from flask_login import current_user, login_required

from ..extensions import db
from ..models import Notification

bp = Blueprint("notifications", __name__, url_prefix="/notifications")


@bp.route("/")
@login_required
def index():
    page = max(1, int(request.args.get("page", 1)))
    pagination = Notification.query.filter_by(
        user_id=current_user.id
    ).order_by(
        Notification.created_at.desc()
    ).paginate(page=page, per_page=20, error_out=False)
    
    unread_count = Notification.query.filter_by(
        user_id=current_user.id, is_read=False
    ).count()
    
    return render_template(
        "user/notifications.html",
        pagination=pagination,
        unread_count=unread_count,
    )


@bp.route("/<int:nid>/read", methods=["POST"])
@login_required
def mark_read(nid):
    notification = Notification.query.filter_by(
        id=nid, user_id=current_user.id
    ).first_or_404()
    notification.is_read = True
    db.session.commit()
    if notification.link:
        return redirect(notification.link)
    return redirect(url_for("notifications.index"))


@bp.route("/mark-all-read", methods=["POST"])
@login_required
def mark_all_read():
    Notification.query.filter_by(
        user_id=current_user.id, is_read=False
    ).update({"is_read": True})
    db.session.commit()
    return redirect(url_for("notifications.index"))
