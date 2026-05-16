include ../../Makefile.variables
include ../../Makefile.precommit
include ../../Makefile.docker
include ../../Makefile.env
include ../../common.env

SERVICE = maintainer-agent-pr-reviewer

TOKEN ?= $(shell teamvault-password --teamvault-config ~/.teamvault.json --teamvault-key=$(AGENT_PR_REVIEWER_GH_TOKEN_KEY))

# verify-gh-token: fetch GH_TOKEN from teamvault (using the key configured
# in dev.env / prod.env) and probe GitHub's rate_limit API. PAT = 5000/hr,
# anonymous (revoked / scope-stripped) = 60/hr. Catches broken tokens before
# they cause silent agent failures in dev/prod.
.PHONY: verify-gh-token
verify-gh-token:
	@echo "teamvault-key: $(AGENT_PR_REVIEWER_GH_TOKEN_KEY)"
	@echo "token-prefix:  $(shell echo $(TOKEN) | head -c 12)..."
	@user=$$(curl -sH "Authorization: token $(TOKEN)" https://api.github.com/user | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('login','<none>'))"); \
	rl_json=$$(curl -sH "Authorization: token $(TOKEN)" https://api.github.com/rate_limit); \
	limit=$$(echo "$$rl_json" | python3 -c "import json,sys; print(json.load(sys.stdin)['resources']['core']['limit'])"); \
	echo "user:          $$user"; \
	echo "rate-limit:    $$limit/hr"; \
	echo "usage (used/limit, reset in):"; \
	echo "$$rl_json" | python3 -c "import json,sys,time; \
r=json.load(sys.stdin)['resources']; \
keys=['core','search','graphql','code_search']; \
now=int(time.time()); \
[print(f\"  {k:<12} {r[k]['limit']-r[k]['remaining']:>5}/{r[k]['limit']:<5}  reset in {max(0,r[k]['reset']-now)//60:>2}m{max(0,r[k]['reset']-now)%60:02d}s\") for k in keys if k in r]"; \
	if [ "$$limit" = "5000" ]; then \
		echo "PASS: token authenticates as PAT"; \
	elif [ "$$limit" = "60" ]; then \
		echo "FAIL: token degrades to anonymous (60/hr) — revoked or scope-stripped"; exit 1; \
	else \
		echo "FAIL: unexpected rate-limit $$limit"; exit 1; \
	fi
