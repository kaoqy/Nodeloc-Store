"""Thin NodeLoc client: OAuth2 + Payment REST wrappers.

All HTTP calls time out fast and surface a `NodeLocError` for any non-2xx,
so callers can handle UI feedback in one place.
"""
from __future__ import annotations

import hashlib
import hmac
import secrets
import time
from dataclasses import dataclass
from typing import Any, Mapping
from urllib.parse import urlencode

import requests


class NodeLocError(Exception):
    """Raised when the NodeLoc API returns a non-success response."""

    def __init__(self, message: str, *, status: int | None = None, payload: Any = None):
        super().__init__(message)
        self.status = status
        self.payload = payload


# --------------------------------------------------------------------------- #
# OAuth2
# --------------------------------------------------------------------------- #
@dataclass
class OAuthToken:
    access_token: str
    token_type: str = "Bearer"
    expires_in: int = 7200
    refresh_token: str | None = None
    scope: str = ""
    expires_at: float = 0.0  # epoch seconds

    def is_expired(self, skew: int = 60) -> bool:
        if not self.expires_at:
            return False
        return time.time() >= (self.expires_at - skew)

    @classmethod
    def from_response(cls, data: Mapping[str, Any]) -> "OAuthToken":
        expires_in = int(data.get("expires_in") or 7200)
        return cls(
            access_token=data["access_token"],
            token_type=data.get("token_type", "Bearer"),
            expires_in=expires_in,
            refresh_token=data.get("refresh_token"),
            scope=data.get("scope", ""),
            expires_at=time.time() + expires_in,
        )


@dataclass
class OAuthUser:
    id: int
    username: str
    name: str
    avatar_url: str
    trust_level: int
    email: str | None = None

    @classmethod
    def from_response(cls, data: Mapping[str, Any]) -> "OAuthUser":
        return cls(
            id=int(data["id"]),
            username=str(data.get("username") or ""),
            name=str(data.get("name") or data.get("username") or ""),
            avatar_url=str(data.get("avatar_url") or ""),
            trust_level=int(data.get("trust_level") or 0),
            email=data.get("email"),
        )


class NodeLocOAuth:
    """OAuth2 authorization-code client for NodeLoc."""

    PROVIDER_NAME = "nodeloc"

    def __init__(self, base_url: str, client_id: str, client_secret: str, redirect_uri: str, scopes: str = "openid profile email"):
        self.base_url = base_url.rstrip("/")
        self.client_id = client_id
        self.client_secret = client_secret
        self.redirect_uri = redirect_uri
        self.scopes = scopes
        self._session = requests.Session()
        self._session.headers.update({"User-Agent": "NodeLoc-Store/1.0"})

    # -- public -- #
    def is_configured(self) -> bool:
        return bool(self.client_id and self.client_secret and self.redirect_uri)

    def build_authorize_url(self, state: str) -> str:
        params = {
            "client_id": self.client_id,
            "redirect_uri": self.redirect_uri,
            "response_type": "code",
            "scope": self.scopes,
            "state": state,
        }
        return f"{self.base_url}/oauth-provider/authorize?{urlencode(params)}"

    def exchange_code(self, code: str) -> OAuthToken:
        return self._token_request({
            "grant_type": "authorization_code",
            "code": code,
            "redirect_uri": self.redirect_uri,
        })

    def refresh(self, refresh_token: str) -> OAuthToken:
        return self._token_request({
            "grant_type": "refresh_token",
            "refresh_token": refresh_token,
        })

    def fetch_userinfo(self, token: OAuthToken) -> OAuthUser:
        try:
            r = self._session.get(
                f"{self.base_url}/oauth-provider/userinfo",
                headers={"Authorization": f"{token.token_type} {token.access_token}"},
                timeout=10,
            )
        except requests.RequestException as exc:
            raise NodeLocError(f"无法连接 NodeLoc: {exc}") from exc
        if not r.ok:
            raise NodeLocError(f"获取用户信息失败 ({r.status_code})", status=r.status_code, payload=_safe_json(r))
        return OAuthUser.from_response(r.json())

    # -- internals -- #
    def _token_request(self, form: Mapping[str, Any]) -> OAuthToken:
        if not self.is_configured():
            raise NodeLocError("OAuth 客户端未配置")
        body = dict(form)
        body["client_id"] = self.client_id
        body["client_secret"] = self.client_secret
        try:
            r = self._session.post(
                f"{self.base_url}/oauth-provider/token",
                data=body,
                headers={"Accept": "application/json"},
                timeout=15,
            )
        except requests.RequestException as exc:
            raise NodeLocError(f"无法连接 NodeLoc: {exc}") from exc
        if not r.ok:
            raise NodeLocError(
                f"Token 交换失败 ({r.status_code}): {_safe_json(r) or r.text[:200]}",
                status=r.status_code,
                payload=_safe_json(r),
            )
        return OAuthToken.from_response(r.json())


# --------------------------------------------------------------------------- #
# Payment
# --------------------------------------------------------------------------- #
class NodeLocPayment:
    """REST wrapper for NodeLoc payment + transfer endpoints."""

    def __init__(self, base_url: str, payment_id: str, secret_key: str):
        self.base_url = base_url.rstrip("/")
        self.payment_id = payment_id
        self.secret_key = secret_key
        self._session = requests.Session()
        self._session.headers.update({"User-Agent": "NodeLoc-Store/1.0"})

    # -- public -- #
    def is_configured(self) -> bool:
        return bool(self.payment_id and self.secret_key)

    def create_payment(self, *, amount: int, description: str, order_id: str) -> dict:
        """Hit POST /payment/pay/:payment_id/process -> { payment_url, transaction_id, ... }"""
        if not self.is_configured():
            raise NodeLocError("支付未配置")
        params = {"amount": int(amount), "description": description, "order_id": order_id}
        params["signature"] = self._sign(params)
        url = f"{self.base_url}/payment/pay/{self.payment_id}/process"
        return self._post(url, params)

    def query(self, transaction_id: str) -> dict:
        if not self.is_configured():
            raise NodeLocError("支付未配置")
        params = {"transaction_id": transaction_id, "signature": self._sign({"transaction_id": transaction_id})}
        url = f"{self.base_url}/payment/query/{self.payment_id}"
        return self._post(url, params)

    def transfer(self, *, to_user_id: int, to_username: str, amount: int, order_id: str) -> dict:
        if not self.is_configured():
            raise NodeLocError("支付未配置")
        params = {
            "to_user_id": int(to_user_id),
            "to_username": to_username,
            "amount": int(amount),
            "order_id": order_id,
        }
        params["signature"] = self._sign(params)
        url = f"{self.base_url}/payment/transfer/{self.payment_id}"
        return self._post(url, params)

    @staticmethod
    def verify_callback(params: Mapping[str, Any], secret_key: str) -> bool:
        """Verify a payment callback signature.

        NodeLoc signs the callback with the raw payment SECRET KEY (not the
        SHA256-hashed token used for outgoing requests). All non-signature
        parameters are sorted alphabetically and joined as `k=v&k=v`.
        """
        params = dict(params)
        signature = params.pop("signature", None)
        if not signature:
            return False
        return hmac.compare_digest(signature, _sign_with_secret(params, secret_key))

    # -- internals -- #
    def _post(self, url: str, params: Mapping[str, Any]) -> dict:
        try:
            r = self._session.post(url, data=params, headers={"Accept": "application/json"}, timeout=15)
        except requests.RequestException as exc:
            raise NodeLocError(f"无法连接 NodeLoc: {exc}") from exc
        if not r.ok:
            raise NodeLocError(
                f"支付接口错误 ({r.status_code}): {_safe_json(r) or r.text[:200]}",
                status=r.status_code,
                payload=_safe_json(r),
            )
        return r.json()

    def _sign(self, params: Mapping[str, Any]) -> str:
        return _sign_with_secret(params, self.secret_key)


# --------------------------------------------------------------------------- #
# Helpers
# --------------------------------------------------------------------------- #
def _safe_json(r: requests.Response) -> Any:
    try:
        return r.json()
    except Exception:
        return None


def _sign_with_secret(params: Mapping[str, Any], secret: str) -> str:
    """Sign outgoing NodeLoc requests.

    Per docs: token_hash = SHA256(your_token) is the HMAC key, where
    `your_token` is the secret itself (we don't apply a second hash since
    NodeLoc's Payment Secret is already the high-entropy key it gave us).
    The doc wording is consistent with HMAC-SHA256(secret, param_string).
    """
    sorted_items = sorted((k, "" if v is None else str(v)) for k, v in params.items())
    param_string = "&".join(f"{k}={v}" for k, v in sorted_items)
    return hmac.new(secret.encode("utf-8"), param_string.encode("utf-8"), hashlib.sha256).hexdigest()


def new_state() -> str:
    return secrets.token_urlsafe(32)
