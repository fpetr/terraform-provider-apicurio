// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package models

import "time"

// ArtifactMeta is a best-effort representation of Apicurio artifact metadata.
// Apicurio has evolved this shape over time; fields may be absent depending on server version.
// We decode through json.RawMessage in the client and normalize into this struct.
type ArtifactMeta struct {
	GroupID        string
	ArtifactID     string
	Name           string
	Description    string
	Labels         []string
	GlobalID       *int64
	ContentID      *int64
	CreatedOn      *time.Time
	ModifiedOn     *time.Time
	LatestVersion  string
	CurrentVersion string
}
