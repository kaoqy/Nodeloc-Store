"""api blueprint — JSON APIs (public products, auth-free)."""
from __future__ import annotations

from flask import Blueprint, jsonify

from ..models import Product

bp = Blueprint("api", __name__)


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
