include ../../../../Makefile.variables
include ../../../../common.env
include ../../../../local.env

# Override on the command line:
#   make run-once REPO_ALLOWLIST=github.com/bborbe/some-repo
# Default targets bborbe/maintainer because its CI is currently red and
# produces an observable end-to-end publish on smoke tests.
REPO_ALLOWLIST    ?= github.com/bborbe/maintainer
STAGE             ?= dev
BUILD_ASSIGNEE    ?= build-fixer-agent
BUILD_TASK_STATUS ?= todo
BUILD_TASK_PHASE  ?=

run-once:
	@GH_TOKEN=$$(gh auth token) go run -mod=mod . \
		-kafka-brokers="$(KAFKA_BROKERS)" \
		-stage=$(STAGE) \
		-repo-allowlist="$(REPO_ALLOWLIST)" \
		-build-assignee="$(BUILD_ASSIGNEE)" \
		-build-task-status="$(BUILD_TASK_STATUS)" \
		-build-task-phase="$(BUILD_TASK_PHASE)" \
		-v=2
