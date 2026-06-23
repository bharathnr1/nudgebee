from concurrent.futures.thread import ThreadPoolExecutor
from datetime import datetime
from decimal import Decimal
import enum
import json
import logging
from typing import Any, Callable
import uuid

from sqlalchemy import inspect

from notifications_server.exceptions.exceptions import Err

LOG = logging.getLogger(__name__)
tp_executor = ThreadPoolExecutor(15)

MIN_PORT = 1
MAX_PORT = 65535


class ModelEncoder(json.JSONEncoder):
    # pylint: disable=E0202
    def default(self, obj):
        if isinstance(obj, datetime):
            return obj.isoformat()
        if isinstance(obj, enum.Enum):
            return obj.value
        if isinstance(obj, Decimal):
            return float(obj)
        if isinstance(obj, uuid.UUID):
            return str(obj)
        if isinstance(obj, bytes):
            return obj.decode()
        if isinstance(obj, set):
            return list(obj)
        if hasattr(obj, "to_dict"):
            return obj.to_dict()
        if isinstance(obj, str):
            try:
                return json.loads(obj)
            except Exception:
                pass
        return json.JSONEncoder.default(self, obj)


def gen_id() -> str:
    return str(uuid.uuid4())


def singleton(class_: type) -> Callable[..., Any]:
    instances = {}

    def get_instance(*args, **kwargs):
        if class_ not in instances:
            instances[class_] = class_(*args, **kwargs)
        return instances[class_]

    return get_instance


def as_dict(obj: Any) -> dict:
    return {c.key: getattr(obj, c.key) for c in inspect(obj).mapper.column_attrs}


def is_valid_port(value: Any) -> bool:
    try:
        port = int(value)
    except (ValueError, TypeError):
        return False
    return MIN_PORT <= port <= MAX_PORT
