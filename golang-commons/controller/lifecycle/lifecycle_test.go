package lifecycle

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"k8s.io/apimachinery/pkg/api/errors"

	"github.com/platform-mesh/golang-commons/controller/lifecycle/mocks"
	pmtesting "github.com/platform-mesh/golang-commons/controller/lifecycle/testing"
	"github.com/platform-mesh/golang-commons/controller/testSupport"
	"github.com/platform-mesh/golang-commons/logger"
)

func TestUpdateStatus(t *testing.T) {
	clientMock := new(mocks.Client)
	subresourceClient := new(mocks.SubResourceClient)

	logcfg := logger.DefaultConfig()
	logcfg.NoJSON = true
	log, err := logger.New(logcfg)
	assert.NoError(t, err)

	t.Run("Test UpdateStatus with no changes", func(t *testing.T) {
		original := &pmtesting.ImplementingSpreadReconciles{
			TestApiObject: testSupport.TestApiObject{
				Status: testSupport.TestStatus{
					Some: "string",
				},
			}}

		// When
		err := updateStatus(context.Background(), clientMock, original, original, log, true, nil)

		// Then
		assert.NoError(t, err)
	})

	t.Run("Test UpdateStatus with update error", func(t *testing.T) {
		original := &pmtesting.ImplementingSpreadReconciles{
			TestApiObject: testSupport.TestApiObject{
				Status: testSupport.TestStatus{
					Some: "string",
				},
			}}
		current := &pmtesting.ImplementingSpreadReconciles{
			TestApiObject: testSupport.TestApiObject{
				Status: testSupport.TestStatus{
					Some: "string1",
				},
			}}

		clientMock.EXPECT().Status().Return(subresourceClient)
		subresourceClient.EXPECT().Update(mock.Anything, mock.Anything, mock.Anything).
			Return(errors.NewBadRequest("internal error"))

		// When
		err := updateStatus(context.Background(), clientMock, original, current, log, true, nil)

		// Then
		assert.Error(t, err)
		assert.Equal(t, "internal error", err.Error())
	})

	t.Run("Test UpdateStatus with no status object (original)", func(t *testing.T) {
		original := &testSupport.TestNoStatusApiObject{}
		current := &pmtesting.ImplementConditions{}
		// When
		err := updateStatus(context.Background(), clientMock, original, current, log, true, nil)

		// Then
		assert.Error(t, err)
		assert.Equal(t, "status field not found in current object", err.Error())
	})
	t.Run("Test UpdateStatus with no status object (current)", func(t *testing.T) {
		original := &pmtesting.ImplementConditions{}
		current := &testSupport.TestNoStatusApiObject{}
		// When
		err := updateStatus(context.Background(), clientMock, original, current, log, true, nil)

		// Then
		assert.Error(t, err)
		assert.Equal(t, "status field not found in current object", err.Error())
	})
}
