"""store blueprint — public storefront pages."""
from __future__ import annotations

from flask import Blueprint, render_template, request, url_for
from sqlalchemy import or_, desc

from ..extensions import db
from ..models import Product

bp = Blueprint("store", __name__)


@bp.route("/")
def index():
    q = request.args.get("q", "").strip()
    page = max(1, request.args.get("page", 1, type=int))
    per_page = 12

    products_query = Product.query.filter_by(is_published=True, is_archived=False)
    if q:
        products_query = products_query.filter(
            or_(Product.name.ilike(f"%{q}%"), Product.summary.ilike(f"%{q}%"))
        )
    products_query = products_query.order_by(desc(Product.sort_order), desc(Product.id))
    pagination = products_query.paginate(page=page, per_page=per_page, error_out=False)

    return render_template("store/index.html", pagination=pagination, q=q)


@bp.route("/product/<slug>")
def product_detail(slug):
    product = Product.query.filter_by(
        slug=slug, is_published=True, is_archived=False
    ).first_or_404()
    return render_template("store/product_detail.html", product=product)


@bp.route("/search")
def search():
    """Alias for store index with query parameter."""
    return index()
