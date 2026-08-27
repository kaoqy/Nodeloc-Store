"""payment blueprint — create order, NodeLoc redirect, callback, order pages."""
from __future__ import annotations

import uuid
from datetime import datetime
from decimal import Decimal, InvalidOperation
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


@bp.route("/callback", methods=["GET", "POST"])
def callback():
    """Handle NodeLoc browser redirects and server-side payment callbacks."""
    # NodeLoc integrations may submit the callback as query parameters,
    # form data, or JSON. Normalize all supported transports before checking
    # the signature and locating the order.
    params = request.values.to_dict(flat=True)
    if request.is_json:
        payload = request.get_json(silent=True)
        if isinstance(payload, dict):
            params.update({str(key): value for key, value in payload.items()})

    # Accept common signature header variants without weakening verification.
    if not params.get("signature"):
        header_signature = (
            request.headers.get("X-NodeLoc-Signature")
            or request.headers.get("X-Signature")
        )
        if header_signature:
            params["signature"] = header_signature.strip()
    payment = _payment()

    if payment.is_configured() and not NodeLocPayment.verify_callback(params, payment.secret_key):
        flash("支付回调签名验证失败", "danger")
        return redirect(url_for("store.index"))

    transaction_id = params.get("transaction_id")
    order_no = str(
        params.get("external_reference")
        or params.get("order_id")
        or params.get("order_no")
        or params.get("out_trade_no")
        or ""
    ).strip()
    amount = params.get("amount")
    status = str(
        params.get("status")
        or params.get("payment_status")
        or params.get("trade_status")
        or ""
    ).strip().lower()
    platform_fee = params.get("platform_fee")
    merchant_points = params.get("merchant_points")
    paid_at_str = params.get("paid_at")

    order = Order.query.filter_by(order_no=order_no).with_for_update().first()
    if not order:
        flash("订单不存在", "danger")
        return redirect(url_for("store.index"))

    if amount is not None:
        try:
            callback_amount = Decimal(str(amount))
            order_amount = Decimal(str(order.total_amount))
        except (InvalidOperation, TypeError, ValueError):
            log_action("payment.invalid_amount", target=order_no, detail="invalid callback amount")
            flash("支付回调金额格式错误", "danger")
            return redirect(url_for("payment.order_detail", order_no=order.order_no))
        if callback_amount != order_amount:
            log_action(
                "payment.amount_mismatch",
                target=order_no,
                detail=f"expected={order_amount}, received={callback_amount}",
            )
            flash("支付回调金额与订单不一致", "danger")
            return redirect(url_for("payment.order_detail", order_no=order.order_no))

    if order.status == "paid" and order.delivered_at is not None:
        return render_template("payment/done.html", order=order)

    if status in {
        "completed", "complete", "paid", "success", "succeeded",
        "successful", "approved", "finished", "trade_success",
    }:
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
        _fulfill_order_db(order, commit=False)
        db.session.commit()
        log_action("payment.completed", target=order_no, detail=f"amount={amount}")
        return render_template("payment/done.html", order=order)
    elif status in {"failed", "cancelled", "canceled", "expired"}:
        order.status = "failed"
        db.session.commit()
        flash(f"支付失败: {status}", "warning")
        return redirect(url_for("store.product_detail", slug=order.product.slug))
    else:
        log_action("payment.pending", target=order_no, detail=f"status={status or 'unknown'}")
        flash("支付结果仍在处理中，请稍后刷新订单", "info")
        return redirect(url_for("payment.order_detail", order_no=order.order_no))


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


def _fulfill_order_db(order: Order, *, commit: bool = True):
    """Mark order paid and assign a card to the user."""
    if order.delivered_at is not None:
        return
    order.status = "paid"

    card = (
        Card.query
        .filter_by(product_id=order.product_id, status="available")
        .order_by(Card.id.asc())
        .with_for_update(skip_locked=True)
        .first()
    )
    if card:
        card.status = "sold"
        card.order_id = order.id
        card.sold_at = datetime.utcnow()
        order.delivered_at = datetime.utcnow()
    else:
        log_action("payment.stock_warning", target=order.order_no, detail="no card available")

    db.session.flush()
    refresh_product_stock(order.product)
    if commit:
        db.session.commit()
