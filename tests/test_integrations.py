from __future__ import annotations

import sqlite3
from datetime import datetime

import pytest
from sqlalchemy import inspect

import app as app_package
from app.extensions import db
from app.models import AuditLog, Card, DeliveryRecord, Order, PointLedger, Product, User


def configure_test_app(monkeypatch, database_path):
    def apply_test_config(flask_app):
        flask_app.config.update(
            TESTING=True,
            WTF_CSRF_ENABLED=False,
            SECRET_KEY="test-secret",
            SQLALCHEMY_DATABASE_URI=f"sqlite:///{database_path}",
            SQLALCHEMY_ENGINE_OPTIONS={},
        )

    monkeypatch.setattr(app_package, "apply_to", apply_test_config)
    monkeypatch.setattr(app_package, "is_installed", lambda: True)
    return app_package.create_app()


@pytest.fixture
def app(monkeypatch, tmp_path):
    flask_app = configure_test_app(monkeypatch, tmp_path / "test.sqlite3")
    yield flask_app
    with flask_app.app_context():
        db.session.remove()
        db.engine.dispose()


@pytest.fixture
def client(app):
    return app.test_client()


def login(client, user_id):
    with client.session_transaction() as session:
        session["_user_id"] = str(user_id)
        session["_fresh"] = True


def make_user(username, *, points=0, role="user", is_admin=False):
    user = User(
        username=username,
        points=points,
        role=role,
        is_admin=is_admin,
        is_active_flag=True,
    )
    user.set_password("password123")
    db.session.add(user)
    db.session.flush()
    return user


def make_product(slug, *, product_type="card"):
    product = Product(
        slug=slug,
        name=slug,
        price=10,
        product_type=product_type,
        stock_count=0,
        is_published=True,
    )
    db.session.add(product)
    db.session.flush()
    return product


def make_paid_order(user, product, order_no):
    order = Order(
        order_no=order_no,
        user_id=user.id,
        product_id=product.id,
        unit_price=product.price,
        quantity=1,
        total_amount=product.price,
        status="paid",
        fulfillment_status="pending",
        paid_at=datetime.utcnow(),
    )
    db.session.add(order)
    db.session.flush()
    return order


def test_legacy_database_migration_is_idempotent(monkeypatch, tmp_path):
    database_path = tmp_path / "legacy.sqlite3"
    connection = sqlite3.connect(database_path)
    connection.executescript(
        """
        CREATE TABLE users (
            id INTEGER PRIMARY KEY,
            username VARCHAR(64) NOT NULL UNIQUE,
            email VARCHAR(190),
            password_hash VARCHAR(255),
            is_admin BOOLEAN NOT NULL DEFAULT 0,
            is_active_flag BOOLEAN NOT NULL DEFAULT 1,
            created_at DATETIME NOT NULL,
            updated_at DATETIME NOT NULL
        );
        CREATE TABLE products (
            id INTEGER PRIMARY KEY,
            slug VARCHAR(120) NOT NULL UNIQUE,
            name VARCHAR(120) NOT NULL,
            price INTEGER NOT NULL,
            stock_visible BOOLEAN NOT NULL DEFAULT 1,
            stock_count INTEGER NOT NULL DEFAULT 0,
            is_active BOOLEAN NOT NULL DEFAULT 1,
            sort_order INTEGER NOT NULL DEFAULT 0,
            created_at DATETIME NOT NULL,
            updated_at DATETIME NOT NULL
        );
        CREATE TABLE orders (
            id BIGINT PRIMARY KEY,
            order_no VARCHAR(64) NOT NULL UNIQUE,
            user_id INTEGER NOT NULL,
            product_id INTEGER NOT NULL,
            product_name VARCHAR(120) NOT NULL,
            unit_price NUMERIC(12, 2) NOT NULL,
            quantity INTEGER NOT NULL DEFAULT 1,
            total_amount NUMERIC(12, 2) NOT NULL,
            status VARCHAR(32) NOT NULL DEFAULT 'pending',
            created_at DATETIME NOT NULL,
            updated_at DATETIME NOT NULL
        );
        """
    )
    connection.commit()
    connection.close()

    first_app = configure_test_app(monkeypatch, database_path)
    with first_app.app_context():
        inspector = inspect(db.engine)
        tables = set(inspector.get_table_names())
        assert {"oauth_identities", "point_ledger", "delivery_records"} <= tables
        product_columns = {column["name"] for column in inspector.get_columns("products")}
        assert {"is_archived", "archived_at"} <= product_columns
        db.session.remove()
        db.engine.dispose()

    second_app = configure_test_app(monkeypatch, database_path)
    with second_app.app_context():
        inspector = inspect(db.engine)
        product_columns = [column["name"] for column in inspector.get_columns("products")]
        assert product_columns.count("is_archived") == 1
        assert product_columns.count("archived_at") == 1
        assert {"oauth_identities", "point_ledger", "delivery_records"} <= set(
            inspector.get_table_names()
        )
        db.session.remove()
        db.engine.dispose()


def test_admin_positive_and_negative_point_adjustments(app, client):
    with app.app_context():
        admin = make_user("admin", role="super_admin", is_admin=True)
        user = make_user("customer", points=20)
        db.session.commit()
        admin_id, user_id = admin.id, user.id

    login(client, admin_id)
    response = client.post(
        f"/admin/users/{user_id}/points",
        data={"delta": "15", "reason": "service credit"},
    )
    assert response.status_code == 302
    response = client.post(
        f"/admin/users/{user_id}/points",
        data={"delta": "-5", "reason": "correction"},
    )
    assert response.status_code == 302

    with app.app_context():
        user = db.session.get(User, user_id)
        entries = PointLedger.query.filter_by(user_id=user_id).order_by(PointLedger.id).all()
        assert user.points == 30
        assert [(entry.delta, entry.balance_after) for entry in entries] == [(15, 35), (-5, 30)]
        assert all(entry.actor_id == admin_id for entry in entries)


def test_point_ledger_page_only_shows_current_user(app, client):
    with app.app_context():
        first = make_user("first")
        second = make_user("second")
        db.session.flush()
        db.session.add_all(
            [
                PointLedger(
                    user_id=first.id,
                    delta=5,
                    balance_after=5,
                    reason="visible-entry",
                    reference_type="test",
                    reference_id="first-entry",
                ),
                PointLedger(
                    user_id=second.id,
                    delta=9,
                    balance_after=9,
                    reason="hidden-entry",
                    reference_type="test",
                    reference_id="second-entry",
                ),
            ]
        )
        db.session.commit()
        first_id = first.id

    login(client, first_id)
    response = client.get("/user/points")
    assert response.status_code == 200
    assert "visible-entry" in response.get_data(as_text=True)
    assert "hidden-entry" not in response.get_data(as_text=True)


def test_automatic_card_delivery_and_waiting_stock(app):
    from app.blueprints.payment import _fulfill_order_db

    with app.app_context():
        user = make_user("buyer")
        stocked_product = make_product("stocked")
        empty_product = make_product("empty")
        stocked_order = make_paid_order(user, stocked_product, "ORDER-STOCKED")
        empty_order = make_paid_order(user, empty_product, "ORDER-EMPTY")
        card = Card(product_id=stocked_product.id, content="CARD-A", status="available")
        db.session.add(card)
        db.session.commit()

        _fulfill_order_db(stocked_order)
        _fulfill_order_db(empty_order)

        assert stocked_order.fulfillment_status == "delivered"
        assert card.status == "sold"
        assert card.order_id == stocked_order.id
        stocked_records = DeliveryRecord.query.filter_by(order_id=stocked_order.id).all()
        assert len(stocked_records) == 1
        assert stocked_records[0].status == "delivered"
        assert stocked_records[0].content == "CARD-A"

        assert empty_order.fulfillment_status == "waiting_stock"
        empty_records = DeliveryRecord.query.filter_by(order_id=empty_order.id).all()
        assert len(empty_records) == 1
        assert empty_records[0].status == "waiting_stock"


def test_restock_automatically_completes_waiting_delivery(app, client):
    from app.blueprints.payment import _fulfill_order_db

    with app.app_context():
        admin = make_user("admin-restock", role="super_admin", is_admin=True)
        user = make_user("waiting-buyer")
        product = make_product("restocked")
        order = make_paid_order(user, product, "ORDER-RESTOCK")
        db.session.commit()
        _fulfill_order_db(order)
        admin_id, product_id, order_id = admin.id, product.id, order.id

    login(client, admin_id)
    response = client.post(
        f"/admin/products/{product_id}/cards/add",
        data={"cards": "RESTOCK-CARD"},
    )
    assert response.status_code == 302

    with app.app_context():
        order = db.session.get(Order, order_id)
        records = DeliveryRecord.query.filter_by(order_id=order_id).all()
        card = Card.query.filter_by(product_id=product_id).one()
        assert order.fulfillment_status == "delivered"
        assert card.status == "sold"
        assert card.order_id == order_id
        assert len(records) == 1
        assert records[0].status == "delivered"
        assert records[0].content == "RESTOCK-CARD"


def test_manual_delivery_persists_single_completed_record(app, client):
    with app.app_context():
        admin = make_user("admin-manual", role="super_admin", is_admin=True)
        user = make_user("manual-buyer")
        product = make_product("manual-product", product_type="manual")
        order = make_paid_order(user, product, "ORDER-MANUAL")
        order.fulfillment_status = "awaiting_manual"
        db.session.add(
            DeliveryRecord(
                order_id=order.id,
                sequence=1,
                delivery_type="manual",
                status="awaiting_manual",
            )
        )
        db.session.commit()
        admin_id, order_id = admin.id, order.id

    login(client, admin_id)
    response = client.post(
        "/admin/orders/ORDER-MANUAL/deliver",
        data={"delivery_content": "manual-license", "delivery_note": "handled"},
    )
    assert response.status_code == 302

    with app.app_context():
        order = db.session.get(Order, order_id)
        records = DeliveryRecord.query.filter_by(order_id=order_id).all()
        assert order.fulfillment_status == "delivered"
        assert order.delivery_content == "manual-license"
        assert len(records) == 1
        assert records[0].status == "delivered"
        assert records[0].content == "manual-license"
        assert records[0].actor_id == admin_id
        assert records[0].completed_at is not None


def test_log_action_persists_without_a_followup_commit(app):
    from app.utils import log_action

    with app.test_request_context("/payment/callback", headers={"X-Forwarded-For": "203.0.113.10"}):
        log_action("payment.test", target="ORDER-AUDIT", detail="persist immediately")
        db.session.remove()

    with app.app_context():
        row = AuditLog.query.filter_by(action="payment.test", target="ORDER-AUDIT").one()
        assert row.detail == "persist immediately"
        assert row.ip == "203.0.113.10"


def test_admin_pages_disable_response_caching(app, client):
    with app.app_context():
        admin = make_user("admin-cache", role="super_admin", is_admin=True)
        db.session.commit()
        admin_id = admin.id

    login(client, admin_id)
    response = client.get("/admin/")

    assert response.status_code == 200
    assert response.headers["Cache-Control"] == "no-store, no-cache, must-revalidate, max-age=0"
    assert response.headers["Pragma"] == "no-cache"
    assert response.headers["Expires"] == "0"
