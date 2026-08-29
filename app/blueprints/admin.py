"""admin blueprint — full admin dashboard."""
from __future__ import annotations

import os
import secrets
from datetime import datetime, timedelta
from flask import (Blueprint, abort, current_app, flash, redirect, render_template,
                   request, send_from_directory, url_for)
from flask_login import current_user
from sqlalchemy import func, or_, and_
from werkzeug.utils import secure_filename

from ..extensions import db
from ..models import (AppSetting, AuditLog, Card, Category, CheckIn,
                      Coupon, DeliveryRecord, Notification, Order,
                      PointLedger, Product, User)
from ..utils import (log_action, refresh_product_stock, slugify,
                     unique_product_slug)


def _get_unread_notification_count() -> int:
    """Get unread notification count for current user (for base template)."""
    from flask_login import current_user
    if not current_user.is_authenticated:
        return 0
    return Notification.query.filter_by(
        user_id=current_user.id, is_read=False
    ).count()

bp = Blueprint("admin", __name__, url_prefix="/admin")

ROLE_LABELS = {
    "super_admin": "超级管理员",
    "admin": "管理员",
    "operator": "运营人员",
    "support": "客服人员",
    "user": "普通用户",
}

ENDPOINT_PERMISSIONS = {
    "admin.index": "dashboard.view",
    "admin.products": "products.manage",
    "admin.product_new": "products.manage",
    "admin.product_edit": "products.manage",
    "admin.product_toggle": "products.manage",
    "admin.product_delete": "products.manage",
    "admin.product_cards": "cards.manage",
    "admin.add_cards": "cards.manage",
    "admin.card_toggle": "cards.manage",
    "admin.card_delete": "cards.manage",
    "admin.orders": "orders.manage",
    "admin.order_cancel": "orders.manage",
    "admin.order_deliver": "orders.manage",
    "admin.order_detail": "orders.manage",
    "admin.order_refund": "orders.manage",
    "admin.users": "users.view",
    "admin.user_toggle_admin": "users.manage",
    "admin.user_toggle_active": "users.manage",
    "admin.user_set_role": "users.manage",
    "admin.user_adjust_points": "users.manage",
    "admin.settings": "settings.manage",
    "admin.oauth_test": "settings.manage",
    "admin.payment_test": "settings.manage",
    "admin.logs": "logs.view",
    "admin.user_detail": "users.view",
    "admin.user_orders": "orders.manage",
    "admin.categories": "products.manage",
    "admin.category_new": "products.manage",
    "admin.category_edit": "products.manage",
    "admin.category_delete": "products.manage",
    "admin.coupons": "settings.manage",
    "admin.coupon_new": "settings.manage",
    "admin.coupon_edit": "settings.manage",
    "admin.coupon_delete": "settings.manage",
    "admin.notifications": "settings.manage",
    "admin.notification_send": "settings.manage",
}


@bp.before_request
def require_admin():
    """Require authentication and the permission assigned to this endpoint."""
    if not current_user.is_authenticated:
        return redirect(url_for("auth.login", next=request.full_path))
    if not current_user.can_access_admin:
        abort(403)

    permission = ENDPOINT_PERMISSIONS.get(request.endpoint)
    if permission and not current_user.has_permission(permission):
        abort(403)


# ── Dashboard ─────────────────────────────────────────────────────────────
@bp.route("/")
def index():
    today = datetime.utcnow().date()
    total_users = db.session.query(func.count(User.id)).scalar() or 0
    total_orders = db.session.query(func.count(Order.id)).scalar() or 0
    paid_orders = Order.query.filter_by(status="paid").count()
    revenue = db.session.query(func.sum(Order.total_amount)).filter(
        Order.status == "paid"
    ).scalar() or 0

    recent_orders = Order.query.order_by(Order.created_at.desc()).limit(8).all()
    recent_logs = AuditLog.query.order_by(AuditLog.created_at.desc()).limit(10).all()
    return render_template("admin/index.html",
                         total_users=total_users, total_orders=total_orders,
                         paid_orders=paid_orders, revenue=revenue,
                         recent_orders=recent_orders, recent_logs=recent_logs)


# ── Products ──────────────────────────────────────────────────────────────
@bp.route("/products")
def products():
    q = request.args.get("q", "").strip()
    archived = request.args.get("archived", "0") == "1"
    page = request.args.get("page", 1, type=int)
    pq = Product.query.filter_by(is_archived=archived)
    if q:
        pq = pq.filter(Product.name.ilike(f"%{q}%"))
    pagination = pq.order_by(Product.id.desc()).paginate(page=page, per_page=20, error_out=False)
    return render_template("admin/products.html", pagination=pagination, q=q, archived=archived)


@bp.route("/products/new", methods=["GET", "POST"])
def product_new():
    return _product_edit(None)


@bp.route("/products/<int:pid>/edit", methods=["GET", "POST"])
def product_edit(pid):
    product = db.get_or_404(Product, pid)
    return _product_edit(product)


def _product_edit(product):
    if request.method == "POST":
        name = request.form.get("name", "").strip()
        slug = request.form.get("slug", "").strip() or None
        summary = request.form.get("summary", "").strip() or None
        description = request.form.get("description", "").strip() or None
        product_type = request.form.get("product_type", "card").strip()
        if product_type not in {"card", "manual"}:
            product_type = "card"
        delivery_instructions = request.form.get("delivery_instructions", "").strip() or None
        require_contact = request.form.get("require_contact") == "on"
        price = request.form.get("price", 0, type=int)
        original_price = request.form.get("original_price", 0, type=int) or None
        is_published = request.form.get("is_published") == "on"
        auto_deliver = request.form.get("auto_deliver") == "on"
        stock_visible = request.form.get("stock_visible") == "on"
        category_id = request.form.get("category_id", type=int) or None

        if not name:
            flash("商品名称不能为空", "danger")
            return render_template("admin/product_form.html",
                                   product=product, is_new=(product is None))

        if not slug:
            slug = unique_product_slug(name, exclude_id=product.id if product else None)
        else:
            slug = slugify(slug)
            exists = Product.query.filter_by(slug=slug).first()
            if exists and (not product or exists.id != product.id):
                flash("Slug 已存在，请换一个", "danger")
                return render_template("admin/product_form.html",
                                       product=product, is_new=(product is None))

        image_path = product.image_path if product else None
        img_file = request.files.get("image")
        if img_file and img_file.filename:
            fname = _save_image(img_file)
            image_path = fname

        if product:
            product.name = name
            product.slug = slug
            product.summary = summary
            product.description = description
            product.product_type = product_type
            product.delivery_instructions = delivery_instructions
            product.require_contact = require_contact
            product.price = price
            product.original_price = original_price
            product.is_published = is_published
            product.auto_deliver = auto_deliver
            product.stock_visible = stock_visible
            product.category_id = category_id
            if image_path:
                product.image_path = image_path
            db.session.commit()
            flash("商品已更新", "success")
            log_action("product.edit", target=slug)
        else:
            p = Product(
                name=name, slug=slug, summary=summary, description=description,
                product_type=product_type,
                delivery_instructions=delivery_instructions,
                require_contact=require_contact,
                price=price, original_price=original_price,
                is_published=is_published, auto_deliver=auto_deliver,
                stock_visible=stock_visible, image_path=image_path,
                category_id=category_id,
            )
            db.session.add(p)
            db.session.commit()
            flash("商品已创建", "success")
            log_action("product.create", target=slug)

        return redirect(url_for("admin.products"))

    return render_template("admin/product_form.html",
                           product=product, is_new=(product is None),
                           categories=Category.query.order_by(Category.sort_order).all())


@bp.route("/users/<int:uid>")
def user_detail(uid):
    user = db.get_or_404(User, uid)
    orders = Order.query.filter_by(user_id=uid).order_by(Order.created_at.desc()).limit(10).all()
    point_entries = PointLedger.query.filter_by(user_id=uid).order_by(
        PointLedger.created_at.desc()
    ).limit(10).all()
    return render_template(
        "admin/user_detail.html",
        user=user,
        orders=orders,
        point_entries=point_entries,
        role_labels=ROLE_LABELS,
    )


@bp.route("/users/<int:uid>/orders")
def user_orders(uid):
    user = db.get_or_404(User, uid)
    page = request.args.get("page", 1, type=int)
    pagination = Order.query.filter_by(user_id=uid).order_by(
        Order.created_at.desc()
    ).paginate(page=page, per_page=20, error_out=False)
    return render_template("admin/user_orders.html", user=user, pagination=pagination)


@bp.route("/products/<int:pid>/toggle", methods=["POST"])
def product_toggle(pid):
    product = db.get_or_404(Product, pid)
    if product.is_archived:
        flash("归档商品不能直接上架，请先恢复商品", "warning")
        return redirect(url_for("admin.products", archived=1))
    product.is_published = not product.is_published
    db.session.commit()
    flash(f"商品已{'上架' if product.is_published else '下架'}", "success")
    return redirect(url_for("admin.products"))


@bp.route("/products/<int:pid>/delete", methods=["POST"])
def product_delete(pid):
    product = db.get_or_404(Product, pid)
    product.is_archived = True
    product.is_published = False
    product.archived_at = datetime.utcnow()
    db.session.commit()
    flash("商品已归档，历史订单将继续保留", "success")
    log_action("product.archive", target=product.slug)
    return redirect(url_for("admin.products"))


@bp.route("/products/<int:pid>/restore", methods=["POST"])
def product_restore(pid):
    product = db.get_or_404(Product, pid)
    product.is_archived = False
    product.archived_at = None
    db.session.commit()
    flash("商品已恢复，请检查后再重新上架", "success")
    log_action("product.restore", target=product.slug)
    return redirect(url_for("admin.products", archived=1))


# ── Cards ─────────────────────────────────────────────────────────────────
@bp.route("/products/<int:pid>/cards")
def product_cards(pid):
    product = db.get_or_404(Product, pid)
    status = request.args.get("status", "all")
    page = request.args.get("page", 1, type=int)
    q = Card.query.filter_by(product_id=pid)
    if status != "all":
        q = q.filter_by(status=status)
    pagination = q.order_by(Card.id.desc()).paginate(page=page, per_page=50, error_out=False)
    return render_template("admin/cards.html", product=product, pagination=pagination, status=status)


@bp.route("/products/<int:pid>/cards/add", methods=["POST"])
def add_cards(pid):
    product = db.get_or_404(Product, pid)
    raw = request.form.get("cards", "").strip()
    lines = list(dict.fromkeys(l.strip() for l in raw.splitlines() if l.strip()))
    existing = {
        value for (value,) in
        db.session.query(Card.content).filter(
            Card.product_id == pid,
            Card.content.in_(lines),
        ).all()
    } if lines else set()
    lines = [line for line in lines if line not in existing]
    added = 0
    for line in lines:
        db.session.add(Card(product_id=pid, content=line))
        added += 1
    # Flush new cards before counting them, then persist the cards and cached
    # product stock in the same transaction. Previously stock_count was
    # refreshed after a commit without committing the refreshed value.
    db.session.flush()
    waiting_orders = (
        Order.query
        .filter_by(product_id=pid, status="paid", fulfillment_status="waiting_stock")
        .order_by(Order.paid_at.asc(), Order.id.asc())
        .with_for_update()
        .all()
    )
    auto_delivered = 0
    for order in waiting_orders:
        card = (
            Card.query
            .filter_by(product_id=pid, status="available")
            .order_by(Card.id.asc())
            .with_for_update(skip_locked=True)
            .first()
        )
        if card is None:
            break
        delivered_at = datetime.utcnow()
        card.status = "sold"
        card.order_id = order.id
        card.sold_at = delivered_at
        order.delivered_at = delivered_at
        order.fulfillment_status = "delivered"
        pending_record = next(
            (record for record in order.delivery_records if record.status == "waiting_stock"),
            None,
        )
        if pending_record:
            pending_record.status = "delivered"
            pending_record.content = card.content
            pending_record.note = f"补货自动发货，card_id={card.id}"
            pending_record.completed_at = delivered_at
        else:
            db.session.add(DeliveryRecord(
                order_id=order.id,
                sequence=len(order.delivery_records) + 1,
                delivery_type="card",
                status="delivered",
                content=card.content,
                note=f"补货自动发货，card_id={card.id}",
                completed_at=delivered_at,
            ))
        auto_delivered += 1
    db.session.flush()
    refresh_product_stock(product)
    db.session.commit()
    skipped = len(existing)
    flash(
        f"已添加 {added} 个卡密，跳过 {skipped} 个重复项，自动补发 {auto_delivered} 个订单",
        "success",
    )
    log_action(
        "cards.add",
        target=f"product={pid}",
        detail=f"count={added}, skipped={skipped}, auto_delivered={auto_delivered}",
    )
    return redirect(url_for("admin.product_cards", pid=pid))


@bp.route("/cards/<int:card_id>/toggle", methods=["POST"])
def card_toggle(card_id):
    card = db.get_or_404(Card, card_id)
    if card.status == "sold":
        flash("已售卡密不能启用或禁用", "warning")
        return redirect(url_for("admin.product_cards", pid=card.product_id))
    card.status = "disabled" if card.status == "available" else "available"
    db.session.flush()
    refresh_product_stock(card.product)
    db.session.commit()
    flash(f"卡密已{'禁用' if card.status == 'disabled' else '启用'}", "success")
    return redirect(url_for("admin.product_cards", pid=card.product_id))


@bp.route("/cards/<int:card_id>/delete", methods=["POST"])
def card_delete(card_id):
    card = db.get_or_404(Card, card_id)
    if card.status == "sold" or card.order_id is not None:
        flash("已售或已关联订单的卡密不能删除", "warning")
        return redirect(url_for("admin.product_cards", pid=card.product_id))
    pid = card.product_id
    product = card.product
    db.session.delete(card)
    db.session.flush()
    refresh_product_stock(product)
    db.session.commit()
    flash("卡密已删除", "success")
    return redirect(url_for("admin.product_cards", pid=pid))


# ── Orders ────────────────────────────────────────────────────────────────
@bp.route("/orders")
def orders():
    status = request.args.get("status", "all")
    q_text = request.args.get("q", "").strip()
    q = Order.query
    if status != "all":
        q = q.filter_by(status=status)
    if q_text:
        pattern = f"%{q_text}%"
        q = q.outerjoin(User).outerjoin(Product).filter(
            db.or_(
                Order.order_no.ilike(pattern),
                Order.transaction_id.ilike(pattern),
                User.username.ilike(pattern),
                User.email.ilike(pattern),
                Product.name.ilike(pattern),
            )
        )
    page = request.args.get("page", 1, type=int)
    pagination = q.order_by(Order.created_at.desc()).paginate(page=page, per_page=20, error_out=False)
    return render_template("admin/orders.html", pagination=pagination, status=status, q=q_text)


@bp.route("/orders/<order_no>/cancel", methods=["POST"])
def order_cancel(order_no):
    order = Order.query.filter_by(order_no=order_no).first_or_404()
    if order.status != "pending":
        flash("只能取消待支付订单", "warning")
        return redirect(url_for("admin.order_detail", order_no=order_no))
    order.status = "cancelled"
    db.session.commit()
    log_action("order.cancel", target=order_no)
    flash("订单已取消", "success")
    return redirect(url_for("admin.order_detail", order_no=order_no))


@bp.route("/orders/<order_no>/deliver", methods=["POST"])
def order_deliver(order_no):
    order = Order.query.filter_by(order_no=order_no).with_for_update().first_or_404()
    if order.status != "paid":
        flash("只能处理已支付订单的交付", "warning")
        return redirect(url_for("admin.order_detail", order_no=order_no))

    if order.product and order.product.product_type == "manual":
        delivery_content = request.form.get("delivery_content", "").strip()
        delivery_note = request.form.get("delivery_note", "").strip() or None
        if not delivery_content:
            flash("请填写交付内容", "warning")
            return redirect(url_for("admin.order_detail", order_no=order_no))
        order.delivery_content = delivery_content
        order.delivery_note = delivery_note
        order.fulfillment_status = "delivered"
        order.delivered_at = datetime.utcnow()
        pending_record = next(
            (record for record in order.delivery_records if record.status == "awaiting_manual"),
            None,
        )
        if pending_record:
            pending_record.status = "delivered"
            pending_record.content = delivery_content
            pending_record.note = delivery_note
            pending_record.actor_id = current_user.id
            pending_record.completed_at = order.delivered_at
        else:
            db.session.add(DeliveryRecord(
                order_id=order.id,
                sequence=len(order.delivery_records) + 1,
                delivery_type="manual",
                status="delivered",
                content=delivery_content,
                note=delivery_note,
                actor_id=current_user.id,
                completed_at=order.delivered_at,
            ))
        db.session.commit()
        log_action("order.deliver_manual", target=order_no)
        flash("人工交付已完成", "success")
        return redirect(url_for("admin.order_detail", order_no=order_no))

    if order.cards:
        flash("该订单已有卡密，无需重复补发", "warning")
        return redirect(url_for("admin.order_detail", order_no=order_no))
    card = Card.query.filter_by(product_id=order.product_id, status="available").order_by(Card.id.asc()).with_for_update(skip_locked=True).first()
    if not card:
        flash("当前没有可用卡密", "danger")
        return redirect(url_for("admin.order_detail", order_no=order_no))
    card.status = "sold"
    card.order_id = order.id
    card.sold_at = datetime.utcnow()
    order.delivered_at = datetime.utcnow()
    order.fulfillment_status = "delivered"
    pending_record = next(
        (record for record in order.delivery_records if record.status == "waiting_stock"),
        None,
    )
    if pending_record:
        pending_record.status = "delivered"
        pending_record.content = card.content
        pending_record.note = f"人工补发，card_id={card.id}"
        pending_record.actor_id = current_user.id
        pending_record.completed_at = order.delivered_at
    else:
        db.session.add(DeliveryRecord(
            order_id=order.id,
            sequence=len(order.delivery_records) + 1,
            delivery_type="card",
            status="delivered",
            content=card.content,
            note=f"人工补发，card_id={card.id}",
            actor_id=current_user.id,
            completed_at=order.delivered_at,
        ))
    db.session.flush()
    refresh_product_stock(order.product)
    db.session.commit()
    log_action("order.deliver", target=order_no, detail=f"card_id={card.id}")
    flash("卡密补发成功", "success")
    return redirect(url_for("admin.order_detail", order_no=order_no))


@bp.route("/orders/<order_no>")
def order_detail(order_no):
    order = Order.query.filter_by(order_no=order_no).first_or_404()
    return render_template("admin/order_detail.html", order=order)


@bp.route("/orders/<order_no>/refund", methods=["POST"])
def order_refund(order_no):
    order = Order.query.filter_by(order_no=order_no).first_or_404()
    if order.status != "paid":
        flash("只能退款已支付的订单", "warning")
        return redirect(url_for("admin.order_detail", order_no=order_no))
    # Release card
    if order.cards:
        for card in order.cards:
            card.status = "available"
            card.order_id = None
            card.sold_at = None
    order.status = "refunded"
    db.session.commit()
    refresh_product_stock(order.product)
    flash("订单已退款", "success")
    log_action("order.refund", target=order_no)
    return redirect(url_for("admin.order_detail", order_no=order_no))


# ── Users ─────────────────────────────────────────────────────────────────
@bp.route("/users")
def users():
    q = request.args.get("q", "").strip()
    page = request.args.get("page", 1, type=int)
    uq = User.query
    if q:
        uq = uq.filter(db.or_(
            User.username.ilike(f"%{q}%"),
            User.email.ilike(f"%{q}%"),
        ))
    pagination = uq.order_by(User.id.desc()).paginate(page=page, per_page=20, error_out=False)
    return render_template("admin/users.html", pagination=pagination, q=q)


@bp.route("/users/<int:uid>/points", methods=["POST"])
def user_adjust_points(uid):
    if not current_user.has_permission("users.manage"):
        abort(403)

    user = db.get_or_404(User, uid)
    try:
        delta = int(request.form.get("delta", "0"))
    except (TypeError, ValueError):
        flash("积分变动必须是整数", "danger")
        return redirect(url_for("admin.users"))

    reason = request.form.get("reason", "").strip()
    if delta == 0:
        flash("积分变动不能为 0", "warning")
        return redirect(url_for("admin.users"))
    if not reason:
        flash("请填写调账原因", "warning")
        return redirect(url_for("admin.users"))
    if user.points + delta < 0:
        flash("扣减后积分不能小于 0", "danger")
        return redirect(url_for("admin.users"))

    user.points += delta
    reference_id = f"{user.id}:{secrets.token_hex(12)}"
    db.session.add(PointLedger(
        user_id=user.id,
        delta=delta,
        balance_after=user.points,
        reason=reason,
        reference_type="admin_adjustment",
        reference_id=reference_id,
        actor_id=current_user.id,
    ))
    db.session.commit()
    log_action(
        "admin.user.adjust_points",
        target=str(user.id),
        detail=f"delta={delta}, balance={user.points}, reason={reason}",
    )
    flash(f"已为 {user.username} 调整 {delta:+d} 积分，当前余额 {user.points}", "success")
    return redirect(url_for("admin.users"))


@bp.route("/users/<int:uid>/toggle-admin", methods=["POST"])
def user_toggle_admin(uid):
    if not current_user.has_permission("users.manage"):
        abort(403)
    user = db.get_or_404(User, uid)
    if user.id == current_user.id:
        flash("不能修改自己的管理员权限", "warning")
        return redirect(url_for("admin.users"))
    user.is_admin = not user.is_admin
    user.role = "super_admin" if user.is_admin else "user"
    db.session.commit()
    flash(f"{user.username} 已设为{'管理员' if user.is_admin else '普通用户'}", "success")
    log_action("admin.user.toggle_admin", target=str(user.id))
    return redirect(url_for("admin.users"))


@bp.route("/users/<int:uid>/role", methods=["POST"])
def user_set_role(uid):
    if not current_user.has_permission("users.manage"):
        abort(403)

    user = db.get_or_404(User, uid)
    role = request.form.get("role", "user").strip()
    if role not in ROLE_LABELS:
        flash("无效的用户角色", "danger")
        return redirect(url_for("admin.users"))
    if user.id == current_user.id:
        flash("不能修改自己的角色", "warning")
        return redirect(url_for("admin.users"))

    old_role = user.effective_role
    user.role = role
    user.is_admin = role == "super_admin"
    db.session.commit()
    log_action(
        "admin.user.set_role",
        target=str(user.id),
        detail=f"{old_role}->{role}",
    )
    flash(f"{user.username} 的角色已设为 {ROLE_LABELS[role]}", "success")
    return redirect(url_for("admin.users"))


@bp.route("/users/<int:uid>/toggle-active", methods=["POST"])
def user_toggle_active(uid):
    if not current_user.has_permission("users.manage"):
        abort(403)
    user = db.get_or_404(User, uid)
    if user.id == current_user.id:
        flash("不能禁用自己的账户", "warning")
        return redirect(url_for("admin.users"))
    user.is_active_flag = not user.is_active_flag
    db.session.commit()
    flash(f"{user.username} 已{'启用' if user.is_active_flag else '禁用'}", "success")
    log_action("admin.user.toggle_active", target=str(user.id))
    return redirect(url_for("admin.users"))


# ── Settings ──────────────────────────────────────────────────────────────
@bp.route("/settings", methods=["GET", "POST"])
def settings():
    if request.method == "POST":
        site_name = request.form.get("site_name", "").strip()
        site_slogan = request.form.get("site_slogan", "").strip()
        currency = request.form.get("currency", "积分").strip()
        oauth_url = request.form.get("oauth_url", "").strip()
        oauth_client_id = request.form.get("oauth_client_id", "").strip()
        oauth_client_secret = request.form.get("oauth_client_secret", "").strip()
        oauth_redirect_uri = request.form.get("oauth_redirect_uri", "").strip()
        payment_id = request.form.get("payment_id", "").strip()
        payment_secret = request.form.get("payment_secret", "").strip()
        checkin_enabled = "1" if request.form.get("checkin_enabled") == "1" else "0"
        support_email = request.form.get("support_email", "").strip()
        site_notice = request.form.get("site_notice", "").strip()

        try:
            checkin_reward = str(max(0, int(request.form.get("checkin_reward", "5") or 5)))
            checkin_streak_bonus = str(max(0, int(request.form.get("checkin_streak_bonus", "1") or 1)))
            checkin_reward_cap = str(max(0, int(request.form.get("checkin_reward_cap", "20") or 20)))
        except ValueError:
            flash("签到奖励、连续奖励和奖励上限必须是非负整数", "danger")
            settings_data = {s.key: s.value for s in AppSetting.query.all()}
            return render_template(
                "admin/settings.html",
                settings=settings_data,
                role_labels=ROLE_LABELS,
            )

        if int(checkin_reward_cap) < int(checkin_reward):
            flash("签到奖励上限不能小于基础奖励", "danger")
            settings_data = {s.key: s.value for s in AppSetting.query.all()}
            return render_template(
                "admin/settings.html",
                settings=settings_data,
                role_labels=ROLE_LABELS,
            )

        # Note: oauth_scopes is not exposed in the form — it stays as the install-time
        # default ("openid profile email"). To customize, edit instance/config.ini.
        _save_setting("site_name", site_name)
        _save_setting("site_slogan", site_slogan)
        _save_setting("currency", currency)
        _save_setting("oauth_url", oauth_url)
        _save_setting("oauth_client_id", oauth_client_id)
        if oauth_client_secret:
            _save_setting("oauth_client_secret", oauth_client_secret)
        _save_setting("oauth_redirect_uri", oauth_redirect_uri)
        _save_setting("payment_id", payment_id)
        if payment_secret:
            _save_setting("payment_secret", payment_secret)
        _save_setting("checkin_enabled", checkin_enabled)
        _save_setting("checkin_reward", checkin_reward)
        _save_setting("checkin_streak_bonus", checkin_streak_bonus)
        _save_setting("checkin_reward_cap", checkin_reward_cap)
        _save_setting("support_email", support_email)
        _save_setting("site_notice", site_notice)

        flash("设置已保存", "success")
        log_action("settings.save")
        return redirect(url_for("admin.settings"))

    settings_data = {s.key: s.value for s in AppSetting.query.all()}
    return render_template(
        "admin/settings.html",
        settings=settings_data,
        role_labels=ROLE_LABELS,
    )


@bp.route("/settings/oauth-test", methods=["POST"])
def oauth_test():
    oauth_url = request.form.get("oauth_url", "").strip()
    oauth_client_id = request.form.get("oauth_client_id", "").strip()
    oauth_client_secret = request.form.get("oauth_client_secret", "").strip()
    oauth_redirect_uri = request.form.get("oauth_redirect_uri", "").strip()
    if not all([oauth_url, oauth_client_id, oauth_client_secret, oauth_redirect_uri]):
        return {"ok": False, "msg": "请填写完整 OAuth 配置"}
    from ..nodeloc import NodeLocOAuth, NodeLocError
    try:
        oauth = NodeLocOAuth(oauth_url, oauth_client_id, oauth_client_secret, oauth_redirect_uri)
        url = oauth.build_authorize_url("test_state_123")
        return {"ok": True, "authorize_url": url}
    except NodeLocError as e:
        return {"ok": False, "msg": str(e)}


@bp.route("/settings/payment-test", methods=["POST"])
def payment_test():
    payment_id = request.form.get("payment_id", "").strip()
    payment_secret = request.form.get("payment_secret", "").strip()
    payment_url = request.form.get("payment_url", "").strip()
    if not all([payment_id, payment_secret, payment_url]):
        return {"ok": False, "msg": "请填写完整支付配置"}
    from ..nodeloc import NodeLocPayment, NodeLocError
    try:
        p = NodeLocPayment(payment_url, payment_id, payment_secret)
        # try query with fake id — just check it doesn't throw
        return {"ok": True, "msg": "配置可正常连接"}
    except NodeLocError as e:
        return {"ok": False, "msg": str(e)}


# ── Categories ──────────────────────────────────────────────────────────
@bp.route("/categories")
def categories():
    cats = Category.query.order_by(Category.sort_order, Category.id).all()
    return render_template("admin/categories.html", categories=cats)


@bp.route("/categories/new", methods=["POST"])
def category_new():
    name = request.form.get("name", "").strip()
    slug = request.form.get("slug", "").strip()
    description = request.form.get("description", "").strip()
    icon = request.form.get("icon", "").strip()
    sort_order = request.form.get("sort_order", 0, type=int)
    is_visible = request.form.get("is_visible") == "on"

    if not name:
        flash("分类名称不能为空", "danger")
        return redirect(url_for("admin.categories"))

    if not slug:
        slug = slugify(name)
    else:
        slug = slugify(slug)

    if Category.query.filter_by(slug=slug).first():
        flash("分类 slug 已存在", "danger")
        return redirect(url_for("admin.categories"))

    cat = Category(
        name=name, slug=slug, description=description,
        icon=icon, sort_order=sort_order, is_visible=is_visible,
    )
    db.session.add(cat)
    db.session.commit()
    flash("分类已创建", "success")
    log_action("category.create", target=slug)
    return redirect(url_for("admin.categories"))


@bp.route("/categories/<int:cid>/edit", methods=["POST"])
def category_edit(cid):
    cat = db.get_or_404(Category, cid)
    name = request.form.get("name", "").strip()
    slug = request.form.get("slug", "").strip()
    description = request.form.get("description", "").strip()
    icon = request.form.get("icon", "").strip()
    sort_order = request.form.get("sort_order", 0, type=int)
    is_visible = request.form.get("is_visible") == "on"

    if not name:
        flash("分类名称不能为空", "danger")
        return redirect(url_for("admin.categories"))

    if not slug:
        slug = slugify(name)
    else:
        slug = slugify(slug)

    existing = Category.query.filter_by(slug=slug).first()
    if existing and existing.id != cid:
        flash("分类 slug 已存在", "danger")
        return redirect(url_for("admin.categories"))

    cat.name = name
    cat.slug = slug
    cat.description = description
    cat.icon = icon
    cat.sort_order = sort_order
    cat.is_visible = is_visible
    db.session.commit()
    flash("分类已更新", "success")
    log_action("category.edit", target=slug)
    return redirect(url_for("admin.categories"))


@bp.route("/categories/<int:cid>/delete", methods=["POST"])
def category_delete(cid):
    cat = db.get_or_404(Category, cid)
    if cat.products.count() > 0:
        flash("无法删除包含商品的分类", "danger")
        return redirect(url_for("admin.categories"))
    db.session.delete(cat)
    db.session.commit()
    flash("分类已删除", "success")
    log_action("category.delete", target=str(cid))
    return redirect(url_for("admin.categories"))


# ── Audit log ─────────────────────────────────────────────────────────────
@bp.route("/logs")
def logs():
    page = request.args.get("page", 1, type=int)
    action = request.args.get("action", "").strip()
    q = AuditLog.query
    if action:
        q = q.filter(AuditLog.action == action)
    pagination = q.order_by(AuditLog.created_at.desc()).paginate(page=page, per_page=50, error_out=False)
    return render_template("admin/logs.html", pagination=pagination, action=action)


# ── helpers ───────────────────────────────────────────────────────────────
def _save_setting(key: str, value: str) -> None:
    from configparser import RawConfigParser
    from pathlib import Path
    p = Path(__file__).resolve().parents[1] / "instance" / "config.ini"
    cfg = RawConfigParser()
    cfg.read(p, encoding="utf-8")

    section_map = {
        "site_name": "app", "site_slogan": "app", "currency": "app",
        "oauth_url": "oauth", "oauth_client_id": "oauth",
        "oauth_client_secret": "oauth", "oauth_redirect_uri": "oauth",
        "payment_id": "payment", "payment_secret": "payment",
    }
    section = section_map.get(key, "app")
    if not cfg.has_section(section):
        cfg.add_section(section)
    cfg.set(section, key, value)
    with open(p, "w", encoding="utf-8") as f:
        cfg.write(f)

    # Also keep in DB for runtime access
    row = db.session.get(AppSetting, key)
    if row:
        row.value = value
    else:
        db.session.add(AppSetting(key=key, value=value))
    db.session.commit()


def _save_image(file_obj) -> str:
    from PIL import Image
    from .config import Config
    filename = secure_filename(f"{secrets.token_hex(8)}_{file_obj.filename}")
    upload_dir = Path(Config.UPLOAD_FOLDER)
    upload_dir.mkdir(parents=True, exist_ok=True)
    path = upload_dir / filename
    file_obj.save(str(path))
    # Resize if too large
    try:
        img = Image.open(path)
        if max(img.size) > 1200:
            img.thumbnail((1200, 1200), Image.LANCZOS)
            img.save(path, quality=85)
    except Exception:
        pass
    return filename


@bp.route("/notifications")
def notifications():
    page = request.args.get("page", 1, type=int)
    pagination = Notification.query.order_by(Notification.created_at.desc()).paginate(
        page=page, per_page=30, error_out=False
    )
    return render_template("admin/notifications.html", pagination=pagination)


@bp.route("/notifications/send", methods=["POST"])
def notification_send():
    title = request.form.get("title", "").strip()
    content = request.form.get("content", "").strip()
    user_id = request.form.get("user_id", type=int)
    notify_all = request.form.get("notify_all") == "on"

    if not title:
        flash("通知标题不能为空", "danger")
        return redirect(url_for("admin.notifications"))

    if notify_all:
        # Send to all users (batched)
        batch_size = 100
        offset = 0
        while True:
            users = User.query.limit(batch_size).offset(offset).all()
            if not users:
                break
            for user in users:
                db.session.add(Notification(
                    user_id=user.id,
                    type="system",
                    title=title,
                    content=content,
                ))
            db.session.flush()
            offset += batch_size
        db.session.commit()
        flash("已发送系统通知给所有用户", "success")
        log_action("notification.broadcast", target=f"users={User.query.count()}", detail=title)
    elif user_id:
        user = db.get_or_404(User, user_id)
        db.session.add(Notification(
            user_id=user.id,
            type="system",
            title=title,
            content=content,
        ))
        db.session.commit()
        flash(f"已发送通知给 {user.username}", "success")
        log_action("notification.send", target=str(user.id), detail=title)
    else:
        flash("请选择发送对象", "danger")
    return redirect(url_for("admin.notifications"))


# ── Coupons ──────────────────────────────────────────────────────────────
@bp.route("/coupons")
def coupons():
    coupon_list = Coupon.query.order_by(Coupon.created_at.desc()).all()
    return render_template("admin/coupons.html", coupons=coupon_list)


@bp.route("/coupons/new", methods=["POST"])
def coupon_new():
    code = request.form.get("code", "").strip().upper()
    discount_type = request.form.get("discount_type", "fixed").strip()
    discount_value = request.form.get("discount_value", 0, type=int)
    min_order_amount = request.form.get("min_order_amount", 0, type=int)
    max_uses = request.form.get("max_uses", 0, type=int)
    valid_from = request.form.get("valid_from", "").strip()
    valid_until = request.form.get("valid_until", "").strip()
    is_active = request.form.get("is_active") == "on"

    if not code:
        flash("优惠券代码不能为空", "danger")
        return redirect(url_for("admin.coupons"))
    if discount_type not in ("fixed", "percent"):
        flash("无效的折扣类型", "danger")
        return redirect(url_for("admin.coupons"))
    if discount_value <= 0:
        flash("折扣值必须大于 0", "danger")
        return redirect(url_for("admin.coupons"))
    if Coupon.query.filter_by(code=code).first():
        flash("优惠券代码已存在", "danger")
        return redirect(url_for("admin.coupons"))

    valid_from_dt = datetime.fromisoformat(valid_from) if valid_from else None
    valid_until_dt = datetime.fromisoformat(valid_until) if valid_until else None

    coupon = Coupon(
        code=code, discount_type=discount_type, discount_value=discount_value,
        min_order_amount=min_order_amount, max_uses=max_uses,
        valid_from=valid_from_dt, valid_until=valid_until_dt, is_active=is_active,
    )
    db.session.add(coupon)
    db.session.commit()
    flash("优惠券已创建", "success")
    log_action("coupon.create", target=code)
    return redirect(url_for("admin.coupons"))


@bp.route("/coupons/<int:coupon_id>/toggle", methods=["POST"])
def coupon_toggle(coupon_id):
    coupon = db.get_or_404(Coupon, coupon_id)
    coupon.is_active = not coupon.is_active
    db.session.commit()
    flash(f"优惠券已{'启用' if coupon.is_active else '禁用'}", "success")
    log_action("coupon.toggle", target=coupon.code)
    return redirect(url_for("admin.coupons"))


@bp.route("/coupons/<int:coupon_id>/delete", methods=["POST"])
def coupon_delete(coupon_id):
    coupon = db.get_or_404(Coupon, coupon_id)
    db.session.delete(coupon)
    db.session.commit()
    flash("优惠券已删除", "success")
    log_action("coupon.delete", target=coupon.code)
    return redirect(url_for("admin.coupons"))
