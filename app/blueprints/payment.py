"""payment blueprint — create order, NodeLoc redirect, callback, order pages."""
from __future__ import annotations

import uuid
from datetime import datetime
from flask import Blueprint, current_app, flash, redirect, render_template, request, url_for
from flask_login import current_user, login_required

from ..extensions import db
from ..models import Card, Order, Product
from ..nodeloc import NodeLocPayment, NodeLocError, NodeLocOAuth
from ..utils import log_action, refresh_product_stock

bp = Blueprint("payment", __name__)


@bp.route("/create/<int:product_id>", methods=["POST"])
@login_required
def create(product_id):
    product = Product.query.get_or_404(product_id)
    if not product.is_published:
        flash("该商品已下架", "warning")
        return redirect(url_for("store.index"))

    if product.stock_count < 1:
        flash("库存不足", "warning")
        return redirect(url_for("store.product_detail", slug=product.slug))

    # Create pending order
    order_no = f"ord_{uuid.uuid4().hex[:20]}"
    order = Order(
        order_no=order_no,
        user_id=current_user.id,
        product_id=product.id,
        quantity=1,
        unit_price=product.price,
        total_amount=product.price,
        status="pending",
    )
    db.session.add(order)
    db.session.commit()

    # Initiate NodeLoc payment
    payment = _payment()
    if not payment.is_configured():
        # Demo / no-payment mode: directly fulfill
        return _fulfill_order(order)

    try:
        resp = payment.create_payment(
            amount=product.price,
            description=f"{product.name} × 1",
            order_id=order_no,
        )
        order.transaction_id = resp.get("transaction_id")
        db.session.commit()
        payment_url = resp.get("payment_url")
        if payment_url:
            return redirect(payment_url)
        # No URL returned → treat as instant completion (e.g. zero-price)
        return _fulfill_order(order)
    except NodeLocError as e:
        order.status = "failed"
        db.session.commit()
        flash(f"支付发起失败: {e}", "danger")
        return redirect(url_for("store.product_detail", slug=product.slug))


@bp.route("/callback")
def callback():
    """Browser GET redirect from NodeLoc after payment."""
    params = dict(request.args)
    sig = params.pop("signature", None)
    payment = _payment()

    if payment.is_configured() and not NodeLocPayment.verify_callback(params, payment.secret_key):
        flash("支付回调签名验证失败", "danger")
        return redirect(url_for("store.index"))

    transaction_id = params.get("transaction_id")
    order_no = params.get("external_reference")
    amount = params.get("amount")
    status = params.get("status")
    platform_fee = params.get("platform_fee")
    merchant_points = params.get("merchant_points")
    paid_at_str = params.get("paid_at")

    order = Order.query.filter_by(order_no=order_no).first()
    if not order:
        flash("订单不存在", "danger")
        return redirect(url_for("store.index"))

    if order.status == "paid":
        return render_template("payment/done.html", order=order)

    if status == "completed":
        order.status = "paid"
        order.transaction_id = transaction_id
        if platform_fee is not None:
            order.platform_fee = int(platform_fee)
        if merchant_points is not None:
            order.merchant_points = int(merchant_points)
        if paid_at_str:
            try:
                order.paid_at = datetime.fromisoformat(paid_at_str.replace("Z", "+00:00"))
            except Exception:
                pass
        db.session.commit()
        _fulfill_order_db(order)
        log_action("payment.completed", target=order_no, detail=f"amount={amount}")
        return render_template("payment/done.html", order=order)
    else:
        order.status = "failed"
        db.session.commit()
        flash(f"支付失败: {status}", "warning")
        return redirect(url_for("store.product_detail", slug=order.product.slug))


@bp.route("/order/<order_no>")
@login_required
def order_detail(order_no):
    order = Order.query.filter_by(order_no=order_no, user_id=current_user.id).first_or_404()
    return render_template("payment/order_detail.html", order=order)


def _payment() -> NodeLocPayment:
    return NodeLocPayment(
        base_url=current_app.config["NODELOC_URL"],
        payment_id=current_app.config["PAYMENT_ID"],
        secret_key=current_app.config["PAYMENT_SECRET"],
    )


def _fulfill_order(order: Order) -> ...:
    return _fulfill_order_db(order) or render_template("payment/done.html", order=order)


def _fulfill_order_db(order: Order):
    """Mark order paid and assign a card to the user."""
    if order.status == "paid":
        return
    order.status = "paid"
    order.delivered_at = datetime.utcnow()

    card = Card.query.filter_by(product_id=order.product_id, status="available").first()
    if card:
        card.status = "sold"
        card.order_id = order.id
        card.sold_at = datetime.utcnow()
    else:
        log_action("payment.stock_warning", target=order.order_no, detail="no card available")

    db.session.commit()
    refresh_product_stock(order.product)
