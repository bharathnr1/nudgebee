# app-e2e-tests (Playwright / Node)
# Cross-service end-to-end tests. `test` runs the Playwright suite (see the
# package.json scripts for environment-scoped variants like test:dev/test:test).
.PHONY: install test report clean

install:
	npm ci

test:
	npx playwright test

report:
	npx playwright show-report

clean:
	rm -rf node_modules/.cache playwright-report test-results
