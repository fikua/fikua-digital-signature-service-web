.PHONY: dev web test lint image

web:           ## Build the page into internal/web/dist
	cd web && npm ci && npm run build

test:          ## Go and page tests
	go test -race -count=1 ./...
	cd web && npm test

lint:
	gofmt -l . | (! grep .)
	go vet ./...

image:         ## The production image, locally
	docker build -t fikua-digital-signature-service-web:local .
