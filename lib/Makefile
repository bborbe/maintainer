include ../Makefile.variables
include ../Makefile.precommit

# lib/ is a shared Go library, not a deployable service. The root Makefile's
# `buca` target recurses into every directory with a go.mod; this no-op
# satisfies the recursion without producing a Docker image.
.PHONY: buca apply
buca:
	@echo "lib/ is a shared Go library — no Docker image to build, skipping"

apply:
	@echo "lib/ is a shared Go library — no k8s manifests to apply, skipping"
