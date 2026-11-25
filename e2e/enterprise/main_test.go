//go:build e2e

// Package enterprise_test installs the netbird chart's enterprise edition into
// a Kind cluster, fresh or by switching a community release when
// NB_E2E_FROM_EDITION=community, and connects real NetBird peers through it. Start with TestChart in
// chart_test.go; e2e/README.md describes the setup and the knobs.
package enterprise_test

import (
	"testing"

	"github.com/netbirdio/helms/e2e/framework"
	"github.com/netbirdio/helms/e2e/framework/netbird"
	"github.com/netbirdio/helms/e2e/shared"
)

// env is the environment TestMain brought up, shared by every test.
var env *netbird.Env

func TestMain(m *testing.M) {
	shared.Main(m, netbird.Config{
		Edition:     netbird.EditionEnterprise,
		FromEdition: framework.EnvOr("NB_E2E_FROM_EDITION", netbird.EditionEnterprise),
		ClusterName: "netbird-enterprise-chart-e2e",
	}, &env)
}
