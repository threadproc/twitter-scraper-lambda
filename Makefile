LAMBDA_NAME ?= twitter-api-scraper

build:
	mkdir -p out
	GOOS=linux GOARCH=amd64 go build -o out/twitter-api-scraper cmd/main.go
	cd out; zip lambda.zip twitter-api-scraper

deploy: build
	aws lambda update-function-code --function-name '$(LAMBDA_NAME)' --zip-file fileb://out/lambda.zip