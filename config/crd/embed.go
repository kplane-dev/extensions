// Package crd embeds the generated CRD manifests for installation into VCPs.
package crd

import "embed"

// FS embeds all CRD YAML manifests from this directory.
//
//go:embed *.yaml
var FS embed.FS
