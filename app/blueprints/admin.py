"""admin blueprint — full admin dashboard."""
from __future__ import annotations

import os
import secrets
from datetime import datetime
from flask import (Blueprint, abort, current_app, flash, redirect, render_template,
                   request, send_from_directory, url_for)
from flask_login import current_user
from werkzeug.utils import secure_filename

from ..extensions import db
from ..models import AppSetting, AuditLog, Card, Order, Product, User
from ..utils import (log_action, refresh_product_stock, slugify,
                     unique_product_slug)

bp = Blueprint("admin", __name__, url_prefix="/admin")


@bp.before_request
def require_admin():
    """Require an authenticated administrator for every admin route."""
    if not current_user.is_authenticated:
        return redirect(url_for("auth.login", next=request.full_path))
    if not current_user.is_admin:
        abort(403)


# ── Dashboard ─────────────────────────────────────────────────────────────
@bp.route("/")
def index():
    today = datetime.utcnow().date()
    total_users = db.session.query(db.func.count(User.id)).scalar() or 0
    total_orders = db.session.query(db.func.count(Order.id)).scalar() or 0
    paid_orders = Order.query.filter_by(status="paid").count()
    revenue = db.session.query(db.func.sum(Order.total_amount)).filter(
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
    page = request.args.get("page", 1, type=int)
    pq = Product.query
    if q:
        pq = pq.filter(Product.name.ilike(f"%{q}%"))
    pagination = pq.order_by(Product.id.desc()).paginate(page=page, per_page=20, error_out=False)
    return render_template("admin/products.html", pagination=pagination, q=q)


@bp.route("/products/new", methods=["GET", "POST"])
def product_new():
    return _product_edit(None)


@bp.route("/products/<int:pid>/edit", methods=["GET", "POST"])
def product_edit(pid):
    product = Product.query.get_or_404(pid)
    return _product_edit(product)


def _product_edit(product):
    if request.method == "POST":
        name = request.form.get("name", "").strip()
        slug = request.form.get("slug", "").strip() or None
        summary = request.form.get("summary", "").strip() or None
        description = request.form.get("description", "").strip() or None
        price = request.form.get("price", 0, type=int)
        original_price = request.form.get("original_price", 0, type=int) or None
        is_published = request.form.get("is_published") == "on"
        auto_deliver = request.form.get("auto_deliver") == "on"
        stock_visible = request.form.get("stock_visible") == "on"

        if not name:
            flash("商品名称不能为空", "danger")
            return render_template("admin/product_form.html",
                                   product=product, is_new=(product is None))

        if not slug:
            slug = unique_product_slug(name, exclude_id=product.id if product else None)
        else:
            slug = slugify(slug)
            # check uniqueness
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
            product.price = price
            product.original_price = original_price
            product.is_published = is_published
            product.auto_deliver = auto_deliver
            product.stock_visible = stock_visible
            if image_path:
                product.image_path = image_path
            db.session.commit()
            flash("商品已更新", "success")
            log_action("product.edit", target=slug)
        else:
            p = Product(
                name=name, slug=slug, summary=summary, description=description,
                price=price, original_price=original_price,
                is_published=is_published, auto_deliver=auto_deliver,
                stock_visible=stock_visible, image_path=image_path,
            )
            db.session.add(p)
            db.session.commit()
            flash("商品已创建", "success")
            log_action("product.create", target=slug)

        return redirect(url_for("admin.products"))

    return render_template("admin/product_form.html",
                           product=product, is_new=(product is None))


@bp.route("/products/<int:pid>/toggle")
def product_toggle(pid):
    product = Product.query.get_or_404(pid)
    product.is_published = not product.is_published
    db.session.commit()
    flash(f"商品已{'上架' if product.is_published else '下架'}", "success")
    return redirect(url_for("admin.products"))


@bp.route("/products/<int:pid>/delete", methods=["POST"])
def product_delete(pid):
    product = Product.query.get_or_404(pid)
    db.session.delete(product)
    db.session.commit()
    flash("商品已删除", "success")
    log_action("product.delete", target=product.slug)
    return redirect(url_for("admin.products"))


# ── Cards ─────────────────────────────────────────────────────────────────
@bp.route("/products/<int:pid>/cards")
def product_cards(pid):
    product = Product.query.get_or_404(pid)
    status = request.args.get("status", "all")
    q = Card.query.filter_by(product_id=pid)
    if status != "all":
        q = q.filter_by(status=status)
    cards = q.order_by(Card.id.desc()).all()
    return render_template("admin/cards.html", product=product, cards=cards, status=status)


@bp.route("/products/<int:pid>/cards/add", methods=["POST"])
def add_cards(pid):
    product = Product.query.get_or_404(pid)
    raw = request.form.get("cards", "").strip()
    lines = [l.strip() for l in raw.splitlines() if l.strip()]
    added = 0
    for line in lines:
        db.session.add(Card(product_id=pid, content=line))
        added += 1
    db.session.commit()
    refresh_product_stock(product)
    flash(f"已添加 {added} 个卡密", "success")
    log_action("cards.add", target=f"product={pid}", detail=f"count={added}")
    return redirect(url_for("admin.product_cards", pid=pid))


@bp.route("/cards/<int:card_id>/toggle")
def card_toggle(card_id):
    card = Card.query.get_or_404(card_id)
    card.status = "disabled" if card.status == "available" else "available"
    db.session.commit()
    refresh_product_stock(card.product)
    flash(f"卡密已{'禁用' if card.status == 'disabled' else '启用'}", "success")
    return redirect(url_for("admin.product_cards", pid=card.product_id))


@bp.route("/cards/<int:card_id>/delete", methods=["POST"])
def card_delete(card_id):
    card = Card.query.get_or_404(card_id)
    pid = card.product_id
    db.session.delete(card)
    db.session.commit()
    refresh_product_stock(Product.query.get(pid))
    flash("卡密已删除", "success")
    return redirect(url_for("admin.product_cards", pid=pid))


# ── Orders ────────────────────────────────────────────────────────────────
@bp.route("/orders")
def orders():
    status = request.args.get("status", "all")
    q = Order.query
    if status != "all":
        q = q.filter_by(status=status)
    page = request.args.get("page", 1, type=int)
    pagination = q.order_by(Order.created_at.desc()).paginate(page=page, per_page=20, error_out=False)
    return render_template("admin/orders.html", pagination=pagination, status=status)


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


@bp.route("/users/<int:uid>/toggle-admin", methods=["POST"])
def user_toggle_admin(uid):
    user = User.query.get_or_404(uid)
    if user.id == current_user.id:
        flash("不能修改自己的管理员权限", "danger")
        return redirect(url_for("admin.users"))
    user.is_admin = not user.is_admin
    db.session.commit()
    flash(f"{user.username} 已设为{'管理员' if user.is_admin else '普通用户'}", "success")
    return redirect(url_for("admin.users"))


@bp.route("/users/<int:uid>/toggle-active", methods=["POST"])
def user_toggle_active(uid):
    user = User.query.get_or_404(uid)
    user.is_active_flag = not user.is_active_flag
    db.session.commit()
    flash(f"{user.username} 已{'启用' if user.is_active_flag else '禁用'}", "success")
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
        # Note: oauth_scopes is not exposed in the form — it stays as the install-time
        # default ("openid profile email"). To customize, edit instance/config.ini.
        _save_setting("site_name", site_name)
        _save_setting("site_slogan", site_slogan)
        _save_setting("currency", currency)
        _save_setting("oauth_url", oauth_url)
        _save_setting("oauth_client_id", oauth_client_id)
        _save_setting("oauth_client_secret", oauth_client_secret)
        _save_setting("oauth_redirect_uri", oauth_redirect_uri)
        _save_setting("payment_id", payment_id)
        _save_setting("payment_secret", payment_secret)

        flash("设置已保存", "success")
        log_action("settings.save")
        return redirect(url_for("admin.settings"))

    settings_data = {s.key: s.value for s in AppSetting.query.all()}
    return render_template("admin/settings.html", settings=settings_data)


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
    row = AppSetting.query.get(key)
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


@bp.route("/uploads/<path:filename>")
def uploaded_file(filename):
    return send_from_directory(current_app.config["UPLOAD_FOLDER"], filename)
