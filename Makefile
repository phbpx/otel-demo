.PHONY: help demo stop

help:
	 @awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / { printf "\033[36m%-30s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
.DEFAULT_GOAL := help

demo: stop ## start demo environment
	@docker-compose -f ./dev/docker-compose.yaml up --build

stop: ## stop demo environment
	@docker-compose -f ./dev/docker-compose.yaml down
