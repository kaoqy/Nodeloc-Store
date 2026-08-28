"""payment blueprint — create order, NodeLoc redirect, callback, order pages."""
from __future__ import annotations

import uuid
from datetime import datetime
from decimal import Decimal, InvalidOperation
from flask import Blueprint, current_app, flash, redirect, render_template, request, url_for
from flask_login import current_user, login_required

from ..extensions import db
from ..models import Card, DeliveryRecord, Order, Product
from ..nodeloc import NodeLocPayment, NodeLocError, NodeLocOAuth
from ..utils import log_action, refresh_product_stock

bp = Blueprint("payment", __name__)


@bp.route("/create/<int:product_id>", methods=["POST"])
@login_required
def create(product_id):
    product = db.get_or_404(Product, product_id)
    if not product.is_published or product.is_archived:
        flash("该商品已下架", "warning")
        return redirect(url_for("store.index"))

    if product.product_type == "card" and product.stock_count < 1:
        flash("库存不足", "warning")
        return redirect(url_for("store.product_detail", slug=product.slug))

    customer_contact = request.form.get("customer_contact", "").strip() or None
    customer_note = request.form.get("customer_note", "").strip() or None
    if product.require_contact and not customer_contact:
        flash("请填写交付所需的联系方式", "warning")
        return redirect(url_for("store.product_detail", slug=product.slug))

    order_no = f"ord_{uuid.uuid4().hex[:20]}"
    order = Order(
        order_no=order_no,
        user_id=current_user.id,
        product_id=product.id,
        quantity=1,
        unit_price=product.price,
        total_amount=product.price,
        status="pending",
        fulfillment_status="pending",
        customer_contact=customer_contact,
        customer_note=customer_note,
    )
    db.session.add(order)
    db.session.commit()

    payment = _payment()
    if not payment.is_configured():
        order.status = "payment_error"
        order.fulfillment_status = "blocked"
        db.session.commit()
        log_action(
            "payment.configuration_missing",
            target=order.order_no,
            detail="order blocked before payment initiation",
        )
        flash("支付服务暂未配置，订单未支付且不会发货，请联系管理员", "danger")
        return redirect(url_for("payment.order_detail", order_no=order.order_no))

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

        order.status = "payment_error"
        order.fulfillment_status = "blocked"
        db.session.commit()
        log_action(
            "payment.url_missing",
            target=order.order_no,
            detail=f"transaction_id={order.transaction_id or 'missing'}",
        )
        flash("支付平台未返回有效支付地址，订单未支付且不会发货", "danger")
        return redirect(url_for("payment.order_detail", order_no=order.order_no))
    except NodeLocError as e:
        order.status = "failed"
        db.session.commit()
        flash(f"支付发起失败: {e}", "danger")
        return redirect(url_for("store.product_detail", slug=product.slug))


@bp.route("/callback", methods=["POST"])
@bp.route("/notify", methods=["POST"])
def callback():
    params = request.values.to_dict(flat=True)
    if request.is_json:
        payload = request.get_json(silent=True)
        if isinstance(payload, dict):
            params.update({str(key): value for key, value in payload.items()})

    if not params.get("signature"):
        header_signature = (
            request.headers.get("X-NodeLoc-Signature")
            or request.headers.get("X-Signature")
        )
        if header_signature:
            params["signature"] = header_signature.strip()
    payment = _payment()

    if not payment.is_configured():
        log_action("payment.callback_blocked", detail="payment gateway is not configured")
        return "payment gateway not configured", 503

    if not NodeLocPayment.verify_callback(params, payment.secret_key):
        flash("支付回调签名验证失败", "danger")
        log_action("payment.invalid_signature")
        return "invalid signature", 400

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
        log_action("payment.order_missing", target=order_no)
        return "order not found", 404

    if amount is None:
        log_action("payment.invalid_amount", target=order_no, detail="callback amount missing")
        return "amount required", 400
    try:
        callback_amount = Decimal(str(amount))
        order_amount = Decimal(str(order.total_amount))
    except (InvalidOperation, TypeError, ValueError):
        log_action("payment.invalid_amount", target=order_no, detail="invalid callback amount")
        return "invalid amount", 400
    if callback_amount != order_amount:
        log_action(
            "payment.amount_mismatch",
            target=order_no,
            detail=f"expected={order_amount}, received={callback_amount}",
        )
        return "amount mismatch", 400

    if order.status == "paid":
        if order.transaction_id != transaction_id:
            log_action(
                "payment.transaction_mismatch",
                target=order_no,
                detail=f"expected={order.transaction_id}, received={transaction_id}",
            )
            return "transaction mismatch", 409
        return "success", 200

    if status in {
        "completed", "complete", "paid", "success", "succeeded",
        "successful", "approved", "finished", "trade_success",
    }:
        if not transaction_id:
            log_action("payment.transaction_missing", target=order_no)
            return "transaction id required", 400
        if order.transaction_id and order.transaction_id != transaction_id:
            log_action(
                "payment.transaction_mismatch",
                target=order_no,
                detail=f"expected={order.transaction_id}, received={transaction_id}",
            )
            return "transaction mismatch", 409
        order.transaction_id = transaction_id
        try:
            if platform_fee is not None:
                order.platform_fee = int(platform_fee)
            if merchant_points is not None:
                order.merchant_points = int(merchant_points)
        except (TypeError, ValueError):
            db.session.rollback()
            log_action("payment.invalid_fee", target=order_no)
            return "invalid fee", 400
        if paid_at_str:
            try:
                order.paid_at = datetime.fromisoformat(paid_at_str.replace("Z", "+00:00"))
            except Exception:
                pass
        _fulfill_order_db(order, commit=False)
        db.session.commit()
        log_action("payment.completed", target=order_no, detail=f"amount={amount}")
        return "success", 200
    elif status in {"failed", "cancelled", "canceled", "expired"}:
        if order.status != "paid":
            order.status = "failed"
        db.session.commit()
        log_action("payment.failed", target=order_no, detail=f"status={status}")
        return "success", 200
    else:
        log_action("payment.pending", target=order_no, detail=f"status={status or 'unknown'}")
        return "unsupported status", 400


@bp.route("/return", methods=["GET"])
def browser_return():
    order_no = str(
        request.args.get("external_reference")
        or request.args.get("order_id")
        or request.args.get("order_no")
        or request.args.get("out_trade_no")
        or ""
    ).strip()
    if not order_no:
        flash("支付返回缺少订单号，请在订单中心查看结果", "warning")
        return redirect(url_for("store.index"))

    order = Order.query.filter_by(order_no=order_no).first()
    if not order:
        flash("订单不存在", "danger")
        return redirect(url_for("store.index"))

    if current_user.is_authenticated and order.user_id == current_user.id:
        if order.status == "paid":
            return render_template("payment/done.html", order=order)
        flash("支付结果尚未由服务端确认，请稍后刷新", "info")
        return redirect(url_for("payment.order_detail", order_no=order.order_no))

    flash("请登录后查看订单支付结果", "info")
    return redirect(url_for("auth.login", next=url_for("payment.order_detail", order_no=order.order_no)))


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


def _fulfill_order_db(order: Order, *, commit: bool = True):
    if order.delivered_at is not None:
        return
    order.status = "paid"

    if order.product.product_type == "manual":
        order.fulfillment_status = "awaiting_manual"
        db.session.flush()
        if not order.delivery_records:
            db.session.add(DeliveryRecord(
                order_id=order.id,
                sequence=1,
                delivery_type="manual",
                status="awaiting_manual",
            ))
        if commit:
            db.session.commit()
        return

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
        order.fulfillment_status = "delivered"
        db.session.add(DeliveryRecord(
            order_id=order.id,
            sequence=len(order.delivery_records) + 1,
            delivery_type="card",
            status="delivered",
            content=card.content,
            note=f"card_id={card.id}",
            completed_at=order.delivered_at,
        ))
    else:
        order.fulfillment_status = "waiting_stock"
        if not order.delivery_records:
            db.session.add(DeliveryRecord(
                order_id=order.id,
                sequence=1,
                delivery_type="card",
                status="waiting_stock",
                note="等待库存补充后自动发货",
            ))
        log_action("payment.stock_warning", target=order.order_no, detail="no card available")

    db.session.flush()
    refresh_product_stock(order.product)
    if commit:
        db.session.commit()
