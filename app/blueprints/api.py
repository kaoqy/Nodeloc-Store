"""api blueprint — JSON APIs (public products, auth-free)."""
from __future__ import annotations

from flask import Blueprint, jsonify

from ..extensions import db
from ..config import is_installed
from ..models import Product

bp = Blueprint("api", __name__)


@bp.route("/health")
def health():
    """Liveness + DB connectivity probe for orchestrators (Docker, k8s)."""
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
    items = [p.to_dict() for p in Product.query.filter_by(is_published=True).all()]
    return jsonify({"products": items, "total": len(items)})


@bp.route("/products/<slug>")
def product(slug):
    p = Product.query.filter_by(slug=slug, is_published=True).first()
    if not p:
        return jsonify({"error": "Not found"}), 404
    return jsonify(p.to_dict())
