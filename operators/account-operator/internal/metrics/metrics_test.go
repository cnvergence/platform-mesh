/*
Copyright The Platform Mesh Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package metrics_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/suite"

	"platform-mesh.io/account-operator/internal/metrics"
)

type MetricsTestSuite struct {
	suite.Suite
}

func TestMetricsTestSuite(t *testing.T) {
	suite.Run(t, new(MetricsTestSuite))
}

// TestAccountsReconciled verifies that the AccountsReconciled counter increments
// correctly for each label combination (type, result). It captures the value
// before and after a simulated increment and asserts the counter increased by 1.
func (s *MetricsTestSuite) TestAccountsReconciled() {
	before := testutil.ToFloat64(metrics.AccountsReconciled.WithLabelValues("org", "success"))
	metrics.AccountsReconciled.WithLabelValues("org", "success").Inc()
	s.Require().Equal(before+1, testutil.ToFloat64(metrics.AccountsReconciled.WithLabelValues("org", "success")))

	before = testutil.ToFloat64(metrics.AccountsReconciled.WithLabelValues("org", "error"))
	metrics.AccountsReconciled.WithLabelValues("org", "error").Inc()
	s.Require().Equal(before+1, testutil.ToFloat64(metrics.AccountsReconciled.WithLabelValues("org", "error")))

	before = testutil.ToFloat64(metrics.AccountsReconciled.WithLabelValues("account", "requeue"))
	metrics.AccountsReconciled.WithLabelValues("account", "requeue").Inc()
	s.Require().Equal(before+1, testutil.ToFloat64(metrics.AccountsReconciled.WithLabelValues("account", "requeue")))
}

// TestWorkspaceReadyDuration verifies that the WorkspaceReadyDuration histogram
// records observations. It checks that the number of collected series increases
// after observing durations for both org and account workspace types.
func (s *MetricsTestSuite) TestWorkspaceReadyDuration() {
	before := testutil.CollectAndCount(metrics.WorkspaceReadyDuration)
	metrics.WorkspaceReadyDuration.WithLabelValues("org").Observe(12.5)
	metrics.WorkspaceReadyDuration.WithLabelValues("org").Observe(45.0)
	metrics.WorkspaceReadyDuration.WithLabelValues("account").Observe(8.3)
	s.Assert().Greater(testutil.CollectAndCount(metrics.WorkspaceReadyDuration), before)
}

// TestOrgProvisioningDuration verifies that the OrgProvisioningDuration histogram
// records observations. It checks that the number of collected series increases
// after observing two provisioning durations.
func (s *MetricsTestSuite) TestOrgProvisioningDuration() {
	before := testutil.CollectAndCount(metrics.OrgProvisioningDuration)
	metrics.OrgProvisioningDuration.WithLabelValues().Observe(30.0)
	metrics.OrgProvisioningDuration.WithLabelValues().Observe(75.0)
	s.Assert().Greater(testutil.CollectAndCount(metrics.OrgProvisioningDuration), before)
}

// TestWorkspaceTypeOperations verifies that the WorkspaceTypeOperations counter
// increments correctly for each label combination (operation, wst_kind). It covers
// created and updated operations for both org and account workspace types.
func (s *MetricsTestSuite) TestWorkspaceTypeOperations() {
	before := testutil.ToFloat64(metrics.WorkspaceTypeOperations.WithLabelValues("created", "org"))
	metrics.WorkspaceTypeOperations.WithLabelValues("created", "org").Inc()
	s.Require().Equal(before+1, testutil.ToFloat64(metrics.WorkspaceTypeOperations.WithLabelValues("created", "org")))

	before = testutil.ToFloat64(metrics.WorkspaceTypeOperations.WithLabelValues("created", "account"))
	metrics.WorkspaceTypeOperations.WithLabelValues("created", "account").Inc()
	s.Require().Equal(before+1, testutil.ToFloat64(metrics.WorkspaceTypeOperations.WithLabelValues("created", "account")))

	before = testutil.ToFloat64(metrics.WorkspaceTypeOperations.WithLabelValues("updated", "org"))
	metrics.WorkspaceTypeOperations.WithLabelValues("updated", "org").Inc()
	s.Require().Equal(before+1, testutil.ToFloat64(metrics.WorkspaceTypeOperations.WithLabelValues("updated", "org")))
}
