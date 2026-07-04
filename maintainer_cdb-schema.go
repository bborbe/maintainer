// Copyright (c) 2026 Benjamin Borbe All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package lib

import "github.com/bborbe/cqrs/cdb"

// CDBSchemaIDs lists every CQRS schema published by the maintainer pipeline.
// trading/strimzi/topic-controller imports this slice and provisions the
// matching Kafka topics (one trio of request/event/result topics per ID).
var CDBSchemaIDs = cdb.SchemaIDs{
	GithubPRReviewV1SchemaID,
	GithubReleaserV1SchemaID,
	GithubBuildV1SchemaID,
}

// GithubPRReviewV1SchemaID is the schema for the github-pr watcher's command
// topic. Carries Trigger-style commands consumed by the watcher pod to drive
// the pr-reviewer pipeline.
var GithubPRReviewV1SchemaID = cdb.SchemaID{
	Group:   "maintainer",
	Kind:    "githubprreview",
	Version: "v1",
}

// GithubReleaserV1SchemaID is the schema for the github-release watcher's
// command topic. Carries Trigger-style commands consumed by the watcher pod
// to drive the github-releaser pipeline.
var GithubReleaserV1SchemaID = cdb.SchemaID{
	Group:   "maintainer",
	Kind:    "githubreleaser",
	Version: "v1",
}

// GithubBuildV1SchemaID is the schema for the github-build watcher's
// command topic. Carries Trigger-style commands consumed by the watcher pod
// to drive the build-fixer pipeline.
var GithubBuildV1SchemaID = cdb.SchemaID{
	Group:   "maintainer",
	Kind:    "githubbuild",
	Version: "v1",
}
