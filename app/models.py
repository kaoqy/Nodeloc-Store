"""SQLAlchemy ORM models."""
from __future__ import annotations

from datetime import date, datetime
from decimal import Decimal
from typing import Optional

from flask_login import UserMixin
from sqlalchemy import (
    BigInteger, Boolean, Date, DateTime, ForeignKey, Integer, Numeric,
    String, Text, UniqueConstraint, Index, func
)
from sqlalchemy.orm import Mapped, mapped_column, relationship

from .extensions import db


BIGINT_PK = BigInteger().with_variant(Integer, "sqlite")


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
    role: Mapped[str] = mapped_column(String(32), default="user", nullable=False, index=True)
    points: Mapped[int] = mapped_column(Integer, default=0, nullable=False)
    consecutive_checkins: Mapped[int] = mapped_column(Integer, default=0, nullable=False)
    total_checkins: Mapped[int] = mapped_column(Integer, default=0, nullable=False)
    last_checkin_date: Mapped[Optional[date]] = mapped_column(Date, nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow, nullable=False)
    last_login_at: Mapped[Optional[datetime]] = mapped_column(DateTime, nullable=True)

    orders: Mapped[list["Order"]] = relationship("Order", back_populates="user", lazy="dynamic")

    @property
    def is_active(self) -> bool:
        return bool(self.is_active_flag)

    @property
    def effective_role(self) -> str:
        if self.is_admin:
            return "super_admin" if self.role in {"", "user", None} else self.role
        return self.role or "user"

    def has_permission(self, permission: str) -> bool:
        permissions = {
            "super_admin": {"*"},
            "admin": {
                "dashboard.view", "products.manage", "cards.manage",
                "orders.manage", "users.view", "users.manage", "logs.view",
                "settings.manage",
            },
            "operator": {
                "dashboard.view", "products.manage", "cards.manage",
                "orders.manage",
            },
            "support": {"dashboard.view", "orders.manage", "users.view"},
            "user": set(),
        }
        granted = permissions.get(self.effective_role, set())
        return "*" in granted or permission in granted

    @property
    def can_access_admin(self) -> bool:
        return self.is_admin or self.effective_role in {
            "super_admin", "admin", "operator", "support"
        }

    checkins: Mapped[list["CheckIn"]] = relationship(
        "CheckIn", back_populates="user", cascade="all, delete-orphan"
    )

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


class OAuthIdentity(db.Model):
    __tablename__ = "oauth_identities"
    __table_args__ = (
        UniqueConstraint("provider", "provider_uid", name="uq_oauth_provider_uid"),
        UniqueConstraint("user_id", "provider", name="uq_oauth_user_provider"),
    )

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    user_id: Mapped[int] = mapped_column(
        Integer, ForeignKey("users.id", ondelete="CASCADE"), nullable=False, index=True
    )
    provider: Mapped[str] = mapped_column(String(32), nullable=False)
    provider_uid: Mapped[str] = mapped_column(String(128), nullable=False)
    username: Mapped[Optional[str]] = mapped_column(String(128), nullable=True)
    display_name: Mapped[Optional[str]] = mapped_column(String(128), nullable=True)
    avatar_url: Mapped[Optional[str]] = mapped_column(String(500), nullable=True)
    scope: Mapped[Optional[str]] = mapped_column(String(255), nullable=True)
    access_token: Mapped[Optional[str]] = mapped_column(Text, nullable=True)
    refresh_token: Mapped[Optional[str]] = mapped_column(Text, nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow, nullable=False)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime, default=datetime.utcnow, onupdate=datetime.utcnow, nullable=False
    )

    user: Mapped[User] = relationship("User", backref="oauth_identities")


class PointLedger(db.Model):
    __tablename__ = "point_ledger"
    __table_args__ = (
        UniqueConstraint("reference_type", "reference_id", name="uq_point_reference"),
        Index("ix_point_ledger_user_created", "user_id", "created_at"),
    )

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    user_id: Mapped[int] = mapped_column(
        Integer, ForeignKey("users.id", ondelete="CASCADE"), nullable=False, index=True
    )
    delta: Mapped[int] = mapped_column(Integer, nullable=False)
    balance_after: Mapped[int] = mapped_column(Integer, nullable=False)
    reason: Mapped[str] = mapped_column(String(120), nullable=False)
    reference_type: Mapped[str] = mapped_column(String(32), nullable=False)
    reference_id: Mapped[str] = mapped_column(String(128), nullable=False)
    actor_id: Mapped[Optional[int]] = mapped_column(Integer, nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow, nullable=False)

    user: Mapped[User] = relationship("User", foreign_keys=[user_id], backref="point_entries")


class CheckIn(db.Model):
    __tablename__ = "checkins"
    __table_args__ = (
        UniqueConstraint("user_id", "checkin_date", name="uq_checkins_user_date"),
        Index("ix_checkins_date", "checkin_date"),
    )

    id: Mapped[int] = mapped_column(BIGINT_PK, primary_key=True)
    user_id: Mapped[int] = mapped_column(
        Integer,
        ForeignKey("users.id", ondelete="CASCADE"),
        nullable=False,
        index=True,
    )
    checkin_date: Mapped[date] = mapped_column(Date, nullable=False)
    reward_points: Mapped[int] = mapped_column(Integer, default=0, nullable=False)
    consecutive_days: Mapped[int] = mapped_column(Integer, default=1, nullable=False)
    created_at: Mapped[datetime] = mapped_column(
        DateTime, default=datetime.utcnow, nullable=False
    )

    user: Mapped[User] = relationship("User", back_populates="checkins")


class Category(db.Model):
    __tablename__ = "categories"

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    slug: Mapped[str] = mapped_column(String(120), unique=True, nullable=False, index=True)
    name: Mapped[str] = mapped_column(String(120), nullable=False)
    description: Mapped[Optional[str]] = mapped_column(Text, nullable=True)
    icon: Mapped[Optional[str]] = mapped_column(String(50), nullable=True)
    sort_order: Mapped[int] = mapped_column(Integer, default=0, nullable=False)
    is_visible: Mapped[bool] = mapped_column(Boolean, default=True, nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow, nullable=False)

    products: Mapped[list["Product"]] = relationship("Product", back_populates="category")

    def __repr__(self) -> str:
        return f"<Category {self.id} {self.name}>"


class Product(db.Model):
    __tablename__ = "products"

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    slug: Mapped[str] = mapped_column(String(120), unique=True, nullable=False, index=True)
    name: Mapped[str] = mapped_column(String(120), nullable=False)
    summary: Mapped[Optional[str]] = mapped_column(String(255), nullable=True)
    description: Mapped[Optional[str]] = mapped_column(Text, nullable=True)
    image_path: Mapped[Optional[str]] = mapped_column(String(255), nullable=True)
    product_type: Mapped[str] = mapped_column(
        String(32), default="card", nullable=False, index=True
    )
    delivery_instructions: Mapped[Optional[str]] = mapped_column(Text, nullable=True)
    require_contact: Mapped[bool] = mapped_column(Boolean, default=False, nullable=False)

    price: Mapped[int] = mapped_column(Integer, nullable=False)
    original_price: Mapped[Optional[int]] = mapped_column(Integer, nullable=True)

    stock_visible: Mapped[bool] = mapped_column(Boolean, default=True, nullable=False)
    stock_count: Mapped[int] = mapped_column(Integer, default=0, nullable=False)
    auto_deliver: Mapped[bool] = mapped_column(Boolean, default=True, nullable=False)
    is_published: Mapped[bool] = mapped_column(Boolean, default=True, nullable=False)
    is_archived: Mapped[bool] = mapped_column(Boolean, default=False, nullable=False, index=True)
    archived_at: Mapped[Optional[datetime]] = mapped_column(DateTime, nullable=True)
    sort_order: Mapped[int] = mapped_column(Integer, default=0, nullable=False)

    category_id: Mapped[Optional[int]] = mapped_column(
        Integer, ForeignKey("categories.id"), nullable=True, index=True
    )

    created_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow, nullable=False)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime, default=datetime.utcnow, onupdate=datetime.utcnow, nullable=False
    )

    cards: Mapped[list["Card"]] = relationship(
        "Card", back_populates="product", lazy="dynamic",
        primaryjoin="and_(Card.product_id==Product.id, Card.status=='available')",
    )
    category: Mapped[Optional[Category]] = relationship("Category", back_populates="products")

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
            "category_id": self.category_id,
        }

    def __repr__(self) -> str:
        return f"<Product {self.id} {self.name}>"


class Card(db.Model):
    __tablename__ = "cards"

    id: Mapped[int] = mapped_column(BIGINT_PK, primary_key=True)
    product_id: Mapped[int] = mapped_column(Integer, ForeignKey("products.id"), nullable=False, index=True)
    content: Mapped[str] = mapped_column(Text, nullable=False)
    status: Mapped[str] = mapped_column(String(16), default="available", nullable=False, index=True)
    order_id: Mapped[Optional[int]] = mapped_column(BigInteger, ForeignKey("orders.id"), nullable=True)
    sold_at: Mapped[Optional[datetime]] = mapped_column(DateTime, nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow, nullable=False)

    product: Mapped[Product] = relationship("Product", back_populates="cards")
    order: Mapped[Optional["Order"]] = relationship("Order", back_populates="cards")

    __table_args__ = (
        Index("ix_cards_product_status", "product_id", "status"),
    )


class Order(db.Model):
    __tablename__ = "orders"

    id: Mapped[int] = mapped_column(BIGINT_PK, primary_key=True)
    order_no: Mapped[str] = mapped_column(String(64), unique=True, nullable=False, index=True)
    user_id: Mapped[int] = mapped_column(Integer, ForeignKey("users.id"), nullable=False, index=True)
    product_id: Mapped[int] = mapped_column(Integer, ForeignKey("products.id"), nullable=False, index=True)

    quantity: Mapped[int] = mapped_column(Integer, default=1, nullable=False)
    unit_price: Mapped[int] = mapped_column(Integer, nullable=False)
    total_amount: Mapped[int] = mapped_column(Integer, nullable=False)

    status: Mapped[str] = mapped_column(String(16), default="pending", nullable=False, index=True)
    transaction_id: Mapped[Optional[str]] = mapped_column(String(64), nullable=True, index=True)
    platform_fee: Mapped[Optional[int]] = mapped_column(Integer, nullable=True)
    merchant_points: Mapped[Optional[int]] = mapped_column(Integer, nullable=True)
    paid_at: Mapped[Optional[datetime]] = mapped_column(DateTime, nullable=True)
    delivered_at: Mapped[Optional[datetime]] = mapped_column(DateTime, nullable=True)
    fulfillment_status: Mapped[str] = mapped_column(
        String(32), default="pending", nullable=False, index=True
    )
    customer_contact: Mapped[Optional[str]] = mapped_column(String(255), nullable=True)
    customer_note: Mapped[Optional[str]] = mapped_column(Text, nullable=True)
    delivery_content: Mapped[Optional[str]] = mapped_column(Text, nullable=True)
    delivery_note: Mapped[Optional[str]] = mapped_column(Text, nullable=True)

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


class DeliveryRecord(db.Model):
    __tablename__ = "delivery_records"
    __table_args__ = (
        UniqueConstraint("order_id", "sequence", name="uq_delivery_order_sequence"),
    )

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    order_id: Mapped[int] = mapped_column(
        BigInteger, ForeignKey("orders.id", ondelete="CASCADE"), nullable=False, index=True
    )
    sequence: Mapped[int] = mapped_column(Integer, default=1, nullable=False)
    delivery_type: Mapped[str] = mapped_column(String(32), nullable=False)
    status: Mapped[str] = mapped_column(String(32), nullable=False, index=True)
    content: Mapped[Optional[str]] = mapped_column(Text, nullable=True)
    note: Mapped[Optional[str]] = mapped_column(Text, nullable=True)
    actor_id: Mapped[Optional[int]] = mapped_column(Integer, nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow, nullable=False)
    completed_at: Mapped[Optional[datetime]] = mapped_column(DateTime, nullable=True)

    order: Mapped[Order] = relationship("Order", backref="delivery_records")


class Coupon(db.Model):
    __tablename__ = "coupons"

    id: Mapped[int] = mapped_column(Integer, primary_key=True)
    code: Mapped[str] = mapped_column(String(64), unique=True, nullable=False, index=True)
    discount_type: Mapped[str] = mapped_column(String(16), nullable=False)
    discount_value: Mapped[int] = mapped_column(Integer, nullable=False)
    min_order_amount: Mapped[int] = mapped_column(Integer, default=0, nullable=False)
    max_uses: Mapped[int] = mapped_column(Integer, default=0, nullable=False)
    used_count: Mapped[int] = mapped_column(Integer, default=0, nullable=False)
    valid_from: Mapped[Optional[datetime]] = mapped_column(DateTime, nullable=True)
    valid_until: Mapped[Optional[datetime]] = mapped_column(DateTime, nullable=True)
    is_active: Mapped[bool] = mapped_column(Boolean, default=True, nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow, nullable=False)

    def is_valid(self) -> bool:
        if not self.is_active:
            return False
        if self.max_uses > 0 and self.used_count >= self.max_uses:
            return False
        now = datetime.utcnow()
        if self.valid_from and now < self.valid_from:
            return False
        if self.valid_until and now > self.valid_until:
            return False
        return True

    def calculate_discount(self, amount: int) -> int:
        if self.discount_type == "percent":
            return min(amount, int(amount * self.discount_value / 100))
        elif self.discount_type == "fixed":
            return min(amount, self.discount_value)
        return 0


class Notification(db.Model):
    __tablename__ = "notifications"

    id: Mapped[int] = mapped_column(BIGINT_PK, primary_key=True)
    user_id: Mapped[int] = mapped_column(
        Integer, ForeignKey("users.id", ondelete="CASCADE"), nullable=False, index=True
    )
    type: Mapped[str] = mapped_column(String(32), nullable=False)
    title: Mapped[str] = mapped_column(String(200), nullable=False)
    content: Mapped[Optional[str]] = mapped_column(Text, nullable=True)
    link: Mapped[Optional[str]] = mapped_column(String(500), nullable=True)
    is_read: Mapped[bool] = mapped_column(Boolean, default=False, nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow, nullable=False)

    user: Mapped[User] = relationship("User", backref="notifications")


class AppSetting(db.Model):
    __tablename__ = "app_settings"

    key: Mapped[str] = mapped_column(String(64), primary_key=True)
    value: Mapped[Optional[str]] = mapped_column(Text, nullable=True)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime, default=datetime.utcnow, onupdate=datetime.utcnow, nullable=False
    )

    @classmethod
    def get(cls, key: str, default: Optional[str] = None) -> Optional:
        setting = db.session.get(cls, key)
        return setting.value if setting is not None else default

    @classmethod
    def set(cls, key: str, value: Optional[str]) -> "AppSetting":
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
        return cls.get("install_step") in {"db_done", "complete"}

    @classmethod
    def is_installed(cls) -> bool:
        return cls.get("install_step") == "complete"


class AuditLog(db.Model):
    __tablename__ = "audit_logs"

    id: Mapped[int] = mapped_column(BIGINT_PK, primary_key=True)
    actor_id: Mapped[Optional[int]] = mapped_column(Integer, nullable=True)
    action: Mapped[str] = mapped_column(String(64), nullable=False, index=True)
    target: Mapped[Optional[str]] = mapped_column(String(120), nullable=True)
    detail: Mapped[Optional[str]] = mapped_column(Text, nullable=True)
    ip: Mapped[Optional[str]] = mapped_column(String(64), nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=datetime.utcnow, nullable=False, index=True)
