"""store blueprint — public storefront pages."""
from __future__ import annotations

from flask import Blueprint, render_template, request, url_for
from sqlalchemy import or_, desc

from ..extensions import db
from ..models import Category, Product

bp = Blueprint("store", __name__)


@bp.route("/")
def index():
    q = request.args.get("q", "").strip()
    category = request.args.get("category", "").strip()
    page = max(1, request.args.get("page", 1, type=int))
    per_page = 12

    products_query = Product.query.filter_by(is_published=True, is_archived=False)
    
    if q:
        products_query = products_query.filter(
            or_(Product.name.ilike(f"%{q}%"), Product.summary.ilike(f"%{q}%"))
        )
    
    if category:
        cat = Category.query.filter_by(slug=category).first()
        if cat:
            products_query = products_query.filter_by(category_id=cat.id)
    
    products_query = products_query.order_by(desc(Product.sort_order), desc(Product.id))
    pagination = products_query.paginate(page=page, per_page=per_page, error_out=False)

    categories = Category.query.filter_by(is_visible=True).order_by(Category.sort_order).all()
    
    from ..models import AppSetting
    site_notice = AppSetting.get("site_notice", "")
    
    return render_template(
        "store/index.html",
        pagination=pagination,
        q=q,
        current_category=category,
        categories=categories,
        site_notice=site_notice,
    )


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


@bp.route("/category/<slug>")
def category(slug):
    cat = Category.query.filter_by(slug=slug, is_visible=True).first_or_404()
    page = max(1, request.args.get("page", 1, type=int))
    per_page = 12
    
    products_query = Product.query.filter_by(
        is_published=True, is_archived=False, category_id=cat.id
    ).order_by(desc(Product.sort_order), desc(Product.id))
    
    pagination = products_query.paginate(page=page, per_page=per_page, error_out=False)
    categories = Category.query.filter_by(is_visible=True).order_by(Category.sort_order).all()
    
    return render_template(
        "store/index.html",
        pagination=pagination,
        q="",
        current_category=slug,
        categories=categories,
        category=cat,
    )
