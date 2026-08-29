"""API blueprint — public and admin JSON APIs."""
from __future__ import annotations

from flask import Blueprint, jsonify, request
from sqlalchemy import func, or_, desc

from ..extensions import db
from ..config import is_installed
from ..models import Product, Order, User, PointLedger

bp = Blueprint("api", __name__)


@bp.route("/health")
def health():
    """Liveness + DB connectivity probe for orchestrators."""
    from sqlalchemy import text as sa_text
    payload = {"status": "ok", "installed": is_installed()}
    try:
        db.session.execute(sa_text("SELECT 1"))
        payload["db"] = "ok"
    except Exception as e:
        payload["db"] = "error"
        payload["db_error"] = str(e)[:200]
        payload["status"] = "degraded"
    code = 200 if payload["status"] == "ok" else 503
    return jsonify(payload), code


@bp.route("/products")
def products():
    q = request.args.get("q", "").strip()
    page = max(1, request.args.get("page", 1, type=int))
    per_page = min(50, request.args.get("per_page", 12, type=int))
    
    query = Product.query.filter_by(is_published=True, is_archived=False)
    if q:
        query = query.filter(
            or_(Product.name.ilike(f"%{q}%"), Product.summary.ilike(f"%{q}%"))
        )
    
    query = query.order_by(desc(Product.sort_order), desc(Product.id))
    pagination = query.paginate(page=page, per_page=per_page, error_out=False)
    
    return jsonify({
        "products": [p.to_dict() for p in pagination.items],
        "total": pagination.total,
        "page": pagination.page,
        "pages": pagination.pages,
        "has_next": pagination.has_next,
        "has_prev": pagination.has_prev,
    })


@bp.route("/products/<slug>")
def product(slug):
    p = Product.query.filter_by(slug=slug, is_published=True, is_archived=False).first()
    if not p:
        return jsonify({"error": "Not found"}), 404
    return jsonify(p.to_dict())


@bp.route("/stats")
def stats():
    """Public storefront statistics."""
    total_products = Product.query.filter_by(is_published=True, is_archived=False).count()
    total_orders = Order.query.filter_by(status="paid").count()
    total_users = User.query.filter_by(is_active_flag=True).count()
    
    return jsonify({
        "products": total_products,
        "orders": total_orders,
        "users": total_users,
    })
