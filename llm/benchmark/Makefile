# llm/benchmark (Python / uv)
# Installs the package and its dependencies from pyproject.toml into the system
# environment via uv. fmt/lint/test use the black, flake8 and pytest tools
# declared in pyproject.toml.
.PHONY: install fmt lint test clean

install:
	uv pip install --system -e .

fmt:
	black .

lint:
	black --check .
	flake8 .

test:
	pytest

clean:
	rm -rf .mypy_cache __pycache__ .pytest_cache dist build *.egg-info
