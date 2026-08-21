"""Small helpers shared across blueprints."""
from __future__ import annotations

import functools
import re
import secrets
from typing import Callable

from flask import abort, flash, redirect, url_for
from flask_login import current_user
from sqlalchemy import func

from .extensions import db
from .models import AuditLog, Product


# --------------------------------------------------------------------------- #
# Password hashing (PBKDF2 — no external bcrypt needed, ships with stdlib)
# --------------------------------------------------------------------------- #
import hashlib
import hmac as _hmac


_PWD_ITER = 260000
_PWD_ALGO = "pbkdf2_sha256"


def hash_password(password: str) -> str:
    if not password:
        raise ValueError("password is empty")
    salt = secrets.token_bytes(16)
    digest = hashlib.pbkdf2_hmac("sha256", password.encode("utf-8"), salt, _PWD_ITER)
    return f"{_PWD_ALGO}${_PWD_ITER}${salt.hex()}${digest.hex()}"


def verify_password(password: str, stored: str | None) -> bool:
    if not stored or not password:
        return False
    try:
        algo, iters_s, salt_hex, digest_hex = stored.split("$", 3)
        if algo != _PWD_ALGO:
            return False
        iters = int(iters_s)
        salt = bytes.fromhex(salt_hex)
        expected = bytes.fromhex(digest_hex)
    except Exception:
        return False
    candidate = hashlib.pbkdf2_hmac("sha256", password.encode("utf-8"), salt, iters)
    return _hmac.compare_digest(candidate, expected)


# Patch the model so the auth flow is simple.
from .models import User  # noqa: E402


def _user_password_helpers():
    def set_password(self, raw: str) -> None:
        self.password_hash = hash_password(raw)

    def check_password(self, raw: str) -> bool:
        return verify_password(raw, self.password_hash)

    User.set_password = set_password
    User.check_password = check_password


_user_password_helpers()


# --------------------------------------------------------------------------- #
# Decorators
# --------------------------------------------------------------------------- #
def admin_required(view: Callable) -> Callable:
    @functools.wraps(view)
    def wrapper(*args, **kwargs):
        if not current_user.is_authenticated:
            return redirect(url_for("auth.login", next=request_path()))
        if not current_user.is_admin:
            abort(403)
        return view(*args, **kwargs)
    return wrapper


def request_path() -> str:
    from flask import request
    return request.full_path if request.method == "GET" else request.path


# --------------------------------------------------------------------------- #
# Slug helper
# --------------------------------------------------------------------------- #
_SLUG_RE = re.compile(r"[^a-z0-9\u4e00-\u9fff\-]+", re.IGNORECASE)


def slugify(value: str, fallback: str = "item") -> str:
    s = (value or "").strip().lower()
    s = re.sub(r"\s+", "-", s)
    s = _SLUG_RE.sub("", s)
    s = re.sub(r"-+", "-", s).strip("-")
    return s or fallback


def unique_product_slug(name: str, exclude_id: int | None = None) -> str:
    base = slugify(name)
    slug = base
    i = 2
    while True:
        q = Product.query.filter_by(slug=slug)
        if exclude_id is not None:
            q = q.filter(Product.id != exclude_id)
        if not q.first():
            return slug
        slug = f"{base}-{i}"
        i += 1


# --------------------------------------------------------------------------- #
# Inventory helpers
# --------------------------------------------------------------------------- #
def refresh_product_stock(product: Product) -> None:
    from .models import Card
    n = db.session.query(func.count(Card.id)).filter(
        Card.product_id == product.id, Card.status == "available"
    ).scalar() or 0
    product.stock_count = int(n)


# --------------------------------------------------------------------------- #
# Audit
# --------------------------------------------------------------------------- #
def log_action(action: str, *, target: str | None = None, detail: str | None = None) -> None:
    from flask import request
    actor_id = current_user.id if current_user.is_authenticated else None
    db.session.add(AuditLog(
        actor_id=actor_id,
        action=action,
        target=target,
        detail=detail,
        ip=(request.headers.get("X-Forwarded-For") or request.remote_addr) if request else None,
    ))
