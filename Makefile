.PHONY: verify fmt

verify:
	uv run ruff check .
	uv run ruff format --check .
	uv run pytest -q

fmt:
	uv run ruff check --fix .
	uv run ruff format .
