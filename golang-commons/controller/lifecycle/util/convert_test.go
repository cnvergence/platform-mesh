package util

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/platform-mesh/golang-commons/controller/lifecycle/api"
	pmtesting "github.com/platform-mesh/golang-commons/controller/testSupport"
	"github.com/platform-mesh/golang-commons/logger/testlogger"
)

func TestToRuntimeObjectSpreadReconcileStatusInterface_Success(t *testing.T) {
	tl := testlogger.New()
	apiObject := &pmtesting.ImplementingSpreadReconciles{}
	obj, err := ToInterface[api.RuntimeObjectSpreadReconcileStatus](apiObject, tl.Logger)
	assert.NoError(t, err)
	assert.NotNil(t, obj)
}

func TestToRuntimeObjectSpreadReconcileStatusInterface_Failure(t *testing.T) {
	tl := testlogger.New()
	// DummyRuntimeObject does NOT implement RuntimeObjectSpreadReconcileStatus
	apiObject := &pmtesting.DummyRuntimeObject{}
	_, err := ToInterface[api.RuntimeObjectSpreadReconcileStatus](apiObject, tl.Logger)
	assert.Error(t, err)

	messages, logErr := tl.GetLogMessages()
	assert.NoError(t, logErr)
	assert.Contains(t, messages[0].Message, "Failed to cast instance to target interface")
}

func TestMustToRuntimeObjectSpreadReconcileStatusInterface_Success(t *testing.T) {
	tl := testlogger.New()
	apiObject := &pmtesting.ImplementingSpreadReconciles{}
	obj := MustToInterface[api.RuntimeObjectSpreadReconcileStatus](apiObject, tl.Logger)
	assert.NotNil(t, obj)
}

func TestMustToRuntimeObjectSpreadReconcileStatusInterface_Panic(t *testing.T) {
	tl := testlogger.New()
	apiObject := &pmtesting.DummyRuntimeObject{}
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic but did not panic")
		}
	}()
	MustToInterface[api.RuntimeObjectSpreadReconcileStatus](apiObject, tl.Logger)
}
