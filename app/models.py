"""SQLAlchemy ORM models."""
from __future__ import annotations

from datetime import datetime
from decimal import Decimal
from typing import Optional

from flask_login import UserMixin
from sqlalchemy import (
    BigInteger, Boolean, DateTime, ForeignKey, Integer, Numeric,
    String, Text, UniqueConstraint, Index, func
)
from sqlalchemy.orm import Mapped, mapped_column, relationship

from .extensions import db


# --------------------------------------------------------------------------- #
# Users
# --------------------------------------------------------------------------- #
class User(UserMixin, db.Model):
    __tablename__ = "users"

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    username: Mapped[str] = mapped_column(String(64), unique=True, nullable=False, index=True)
    email: Mapped[Optional[str]] = mapped_column(String(190), unique=True, nullable=True, index=True)
    password_hash: Mapped[Optional[str]] = mapped_column(String(255), nullable=True)

    # OAuth binding
    oauth_provider: Mapped[Optional[str]] = mapped_column(String(32), nullable=True)
    oauth_uid: Mapped[Optional[str]] = mapped_column(String(64), nullable=True, index=True)
    oauth_username: Mapped[Optional[str]] = mapped_column(String(64), nullable=True)
    oauth_name: Mapped[Optional[str]] = mapped_column(String(64), nullable=True)
    oauth_avatar: Mapped[Optional[str]] = mapped_column(String(500), nullable=True)
    oauth_trust_level: Mapped[Optional[int]] = mapped_column(Integer, nullable=True)
    oauth_scope: Mapped[Optional[str]] = mapped_column(String(255), nullable=True)
    oauth_has_email: Mapped[bool] = mapped_column(Boolean, default=False, nullable=False)
    oauth_access_token: Mapped[Optional[str]] = mapped_column(String(255), nullable=True)
    oauth_refresh_token: Mapped[Optional[str]] = mapped_column(String(255), nullable=True)
    oauth_token_expires_at: Mapped[Optional[datetime]] = mapped_column(DateTime, nullable=True)

    is_admin: Mapped[bool] = mapped_column(Boolean, default=False, nullable=False)
    is_active_flag: Mapped[bool] = mapped_column("is_active", Boolean, default=True, nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow, nullable=False)
    last_login_at: Mapped[Optional[datetime]] = mapped_column(DateTime, nullable=True)

    orders: Mapped[list["Order"]] = relationship("Order", back_populates="user", lazy="dynamic")

    # Flask-Login needs `is_active` to be a property by that exact name.
    @property
    def is_active(self) -> bool:  # type: ignore[override]
        return bool(self.is_active_flag)

    def has_password(self) -> bool:
        return bool(self.password_hash)

    def oauth_scope_list(self) -> list[str]:
        if not self.oauth_scope:
            return []
        return [s for s in self.oauth_scope.replace(",", " ").split() if s]

    def has_oauth_scope(self, scope: str) -> bool:
        return scope in self.oauth_scope_list()

    def __repr__(self) -> str:
        return f"<User {self.id} {self.username}>"


# --------------------------------------------------------------------------- #
# Catalog
# --------------------------------------------------------------------------- #
class Product(db.Model):
    __tablename__ = "products"

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    slug: Mapped[str] = mapped_column(String(120), unique=True, nullable=False, index=True)
    name: Mapped[str] = mapped_column(String(120), nullable=False)
    summary: Mapped[Optional[str]] = mapped_column(String(255), nullable=True)
    description: Mapped[Optional[str]] = mapped_column(Text, nullable=True)
    image_path: Mapped[Optional[str]] = mapped_column(String(255), nullable=True)

    # Price stored in integer "points" (NodeLoc currency unit).
    price: Mapped[int] = mapped_column(Integer, nullable=False)
    original_price: Mapped[Optional[int]] = mapped_column(Integer, nullable=True)

    # Inventory / visibility
    stock_visible: Mapped[bool] = mapped_column(Boolean, default=True, nullable=False)
    stock_count: Mapped[int] = mapped_column(Integer, default=0, nullable=False)  # cached card count
    auto_deliver: Mapped[bool] = mapped_column(Boolean, default=True, nullable=False)
    is_published: Mapped[bool] = mapped_column(Boolean, default=True, nullable=False)
    sort_order: Mapped[int] = mapped_column(Integer, default=0, nullable=False)

    created_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow, nullable=False)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime, default=datetime.utcnow, onupdate=datetime.utcnow, nullable=False
    )

    cards: Mapped[list["Card"]] = relationship(
        "Card", back_populates="product", lazy="dynamic",
        primaryjoin="and_(Card.product_id==Product.id, Card.status=='available')",
    )

    def to_dict(self) -> dict:
        return {
            "id": self.id,
            "slug": self.slug,
            "name": self.name,
            "summary": self.summary,
            "description": self.description,
            "image_path": self.image_path,
            "price": self.price,
            "original_price": self.original_price,
            "stock_count": self.stock_count,
            "is_published": self.is_published,
        }

    def __repr__(self) -> str:
        return f"<Product {self.id} {self.name}>"


# --------------------------------------------------------------------------- #
# Card keys (a.k.a. license keys / vouchers)
# --------------------------------------------------------------------------- #
class Card(db.Model):
    __tablename__ = "cards"

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True)
    product_id: Mapped[int] = mapped_column(Integer, ForeignKey("products.id"), nullable=False, index=True)
    content: Mapped[str] = mapped_column(Text, nullable=False)  # may be multi-line "key\tvalue"
    status: Mapped[str] = mapped_column(String(16), default="available", nullable=False, index=True)
    # statuses: available | sold | reserved | disabled
    order_id: Mapped[Optional[int]] = mapped_column(BigInteger, ForeignKey("orders.id"), nullable=True)
    sold_at: Mapped[Optional[datetime]] = mapped_column(DateTime, nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow, nullable=False)

    product: Mapped[Product] = relationship("Product", back_populates="cards")
    order: Mapped[Optional["Order"]] = relationship("Order", back_populates="cards")

    __table_args__ = (
        Index("ix_cards_product_status", "product_id", "status"),
    )


# --------------------------------------------------------------------------- #
# Orders
# --------------------------------------------------------------------------- #
class Order(db.Model):
    __tablename__ = "orders"

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True)
    order_no: Mapped[str] = mapped_column(String(64), unique=True, nullable=False, index=True)
    user_id: Mapped[int] = mapped_column(Integer, ForeignKey("users.id"), nullable=False, index=True)
    product_id: Mapped[int] = mapped_column(Integer, ForeignKey("products.id"), nullable=False, index=True)

    quantity: Mapped[int] = mapped_column(Integer, default=1, nullable=False)
    unit_price: Mapped[int] = mapped_column(Integer, nullable=False)
    total_amount: Mapped[int] = mapped_column(Integer, nullable=False)

    status: Mapped[str] = mapped_column(String(16), default="pending", nullable=False, index=True)
    # statuses: pending | paid | cancelled | refunded | failed

    # NodeLoc payment fields
    transaction_id: Mapped[Optional[str]] = mapped_column(String(64), nullable=True, index=True)
    platform_fee: Mapped[Optional[int]] = mapped_column(Integer, nullable=True)
    merchant_points: Mapped[Optional[int]] = mapped_column(Integer, nullable=True)
    paid_at: Mapped[Optional[datetime]] = mapped_column(DateTime, nullable=True)
    delivered_at: Mapped[Optional[datetime]] = mapped_column(DateTime, nullable=True)

    created_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow, nullable=False)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime, default=datetime.utcnow, onupdate=datetime.utcnow, nullable=False
    )

    user: Mapped[User] = relationship("User", back_populates="orders")
    product: Mapped[Product] = relationship("Product")
    cards: Mapped[list[Card]] = relationship("Card", back_populates="order")

    @property
    def is_paid(self) -> bool:
        return self.status == "paid"

    @property
    def is_pending(self) -> bool:
        return self.status == "pending"

    def __repr__(self) -> str:
        return f"<Order {self.order_no} {self.status}>"


# --------------------------------------------------------------------------- #
# App settings (singleton key/value, populated by install wizard & admin)
# --------------------------------------------------------------------------- #
class AppSetting(db.Model):
    __tablename__ = "app_settings"

    key: Mapped[str] = mapped_column(String(64), primary_key=True)
    value: Mapped[Optional[str]] = mapped_column(Text, nullable=True)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime, default=datetime.utcnow, onupdate=datetime.utcnow, nullable=False
    )

    @classmethod
    def get(cls, key: str, default: Optional[str] = None) -> Optional:
        """Return a setting value, or ``default`` when the key is absent."""
        setting = db.session.get(cls, key)
        return setting.value if setting is not None else default

    @classmethod
    def set(cls, key: str, value: Optional[str]) -> "AppSetting":
        """Create or update a setting and persist it immediately."""
        setting = db.session.get(cls, key)
        if setting is None:
            setting = cls(key=key, value=value)
            db.session.add(setting)
        else:
            setting.value = value
        db.session.commit()
        return setting

    @classmethod
    def is_db_configured(cls) -> bool:
        """Whether the database phase of the installation has completed."""
        return cls.get("install_step") in {"db_done", "complete"}

    @classmethod
    def is_installed(cls) -> bool:
        """Whether all installation phases have completed."""
        return cls.get("install_step") == "complete"


# --------------------------------------------------------------------------- #
# Audit log (admin actions, callback events)
# --------------------------------------------------------------------------- #
class AuditLog(db.Model):
    __tablename__ = "audit_logs"

    id: Mapped[int] = mapped_column(BigInteger, primary_key=True)
    actor_id: Mapped[Optional[int]] = mapped_column(Integer, nullable=True)
    action: Mapped[str] = mapped_column(String(64), nullable=False, index=True)
    target: Mapped[Optional[str]] = mapped_column(String(120), nullable=True)
    detail: Mapped[Optional[str]] = mapped_column(Text, nullable=True)
    ip: Mapped[Optional[str]] = mapped_column(String(64), nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow, nullable=False, index=True)
