package spread

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pmtesting "github.com/platform-mesh/golang-commons/controller/testSupport"
	"github.com/platform-mesh/golang-commons/logger/testlogger"
)

func TestGetNextReconcilationTime(t *testing.T) {
	expectedEarliest := 12 * time.Hour
	expectedLatest := 24 * time.Hour

	actual := getNextReconcileTime(defaultMaxReconcileDuration)
	if actual < expectedEarliest || actual > expectedLatest {
		t.Errorf("Expected time between %v and %v, but got %v", expectedEarliest, expectedLatest, actual)
	}

	actual2 := getNextReconcileTime(defaultMaxReconcileDuration)
	if actual2 < expectedEarliest || actual2 > expectedLatest {
		t.Errorf("Expected time between %v and %v, but got %v", expectedEarliest, expectedLatest, actual)
	}

	if actual == actual2 {
		t.Errorf("Expected different values, but got same")
	}
}

func TestOnNextReconcile(t *testing.T) {
	nextReconcile := time.Now().Add(10 * time.Minute)
	instanceStatusObj := pmtesting.TestStatus{
		NextReconcileTime: v1.NewTime(nextReconcile),
	}
	s := NewSpreader()
	apiObject := &pmtesting.ImplementingSpreadReconciles{TestApiObject: pmtesting.TestApiObject{Status: instanceStatusObj}}
	tl := testlogger.New()

	requeueAfter, err := s.OnNextReconcile(apiObject, tl.Logger)
	if err != nil {
		t.Errorf("Expected no error, but got %v", err)
	}

	timeTill := time.Until(nextReconcile.UTC())
	if time.Duration(requeueAfter.RequeueAfter.Seconds()) != time.Duration(timeTill.Seconds()) {
		t.Errorf("Expected requeueAfter to be %v, but got %v", timeTill, requeueAfter.RequeueAfter)
	}
	messages, err := tl.GetLogMessages()
	assert.NoError(t, err)
	assert.Contains(t, messages[0].Message, "no processing needed")
}

type testInstance struct {
	mock.Mock
	*pmtesting.ImplementingSpreadReconciles
}

func (t *testInstance) GenerateNextReconcileTime() time.Duration {
	args := t.Called()
	return args.Get(0).(time.Duration)
}

func TestGenerateNextReconcileTimer(t *testing.T) {
	instance := &testInstance{
		ImplementingSpreadReconciles: &pmtesting.ImplementingSpreadReconciles{},
	}
	s := NewSpreader()
	instance.On("GenerateNextReconcileTime").Return(10 * time.Minute)

	s.SetNextReconcileTime(instance, testlogger.New().Logger)

	assert.True(t, instance.AssertCalled(t, "GenerateNextReconcileTime"))
}

func TestUpdateObservedGeneration(t *testing.T) {
	s := NewSpreader()
	instanceStatusObj := pmtesting.TestStatus{
		ObservedGeneration: 0,
	}
	apiObject := &pmtesting.ImplementingSpreadReconciles{TestApiObject: pmtesting.TestApiObject{
		Status: instanceStatusObj,
		ObjectMeta: v1.ObjectMeta{
			Generation: 1,
		},
	},
	}
	tl := testlogger.New()
	s.UpdateObservedGeneration(apiObject, tl.Logger)

	assert.Equal(t, apiObject.GetObservedGeneration(), apiObject.GetGeneration())
	messages, err := tl.GetLogMessages()
	assert.NoError(t, err)
	assert.Contains(t, messages[0].Message, "Updating observed generation")
}

func TestRemoveRefreshLabel(t *testing.T) {
	s := NewSpreader()
	apiObject := &pmtesting.TestApiObject{
		ObjectMeta: v1.ObjectMeta{
			Labels: map[string]string{ReconcileRefreshLabel: ""},
		},
	}
	s.RemoveRefreshLabelIfExists(apiObject)

	_, ok := apiObject.GetLabels()[ReconcileRefreshLabel]
	assert.False(t, ok)
}

func TestRemoveRefreshLabelFilledWithValue(t *testing.T) {
	s := NewSpreader()
	apiObject := &pmtesting.TestApiObject{
		ObjectMeta: v1.ObjectMeta{
			Labels: map[string]string{ReconcileRefreshLabel: "true"},
		},
	}
	s.RemoveRefreshLabelIfExists(apiObject)

	_, ok := apiObject.GetLabels()[ReconcileRefreshLabel]

	assert.False(t, ok)
}

func TestRemoveRefreshLabelNoLabels(t *testing.T) {
	s := NewSpreader()
	apiObject := &pmtesting.TestApiObject{
		ObjectMeta: v1.ObjectMeta{},
	}
	s.RemoveRefreshLabelIfExists(apiObject)

	_, ok := apiObject.GetLabels()[ReconcileRefreshLabel]

	assert.False(t, ok)
}

func TestToRuntimeObjectSpreadReconcileStatusInterface_Success(t *testing.T) {
	s := NewSpreader()
	tl := testlogger.New()
	apiObject := &pmtesting.ImplementingSpreadReconciles{}
	obj, err := s.ToRuntimeObjectSpreadReconcileStatusInterface(apiObject, tl.Logger)
	assert.NoError(t, err)
	assert.NotNil(t, obj)
}

func TestToRuntimeObjectSpreadReconcileStatusInterface_Failure(t *testing.T) {
	s := NewSpreader()
	tl := testlogger.New()
	// DummyRuntimeObject does NOT implement RuntimeObjectSpreadReconcileStatus
	apiObject := &pmtesting.DummyRuntimeObject{}
	obj, err := s.ToRuntimeObjectSpreadReconcileStatusInterface(apiObject, tl.Logger)
	assert.Error(t, err)
	assert.Nil(t, obj)
	messages, logErr := tl.GetLogMessages()
	assert.NoError(t, logErr)
	assert.Contains(t, messages[0].Message, "Failed to cast instance to RuntimeObjectSpreadReconcileStatus")
}

func TestMustToRuntimeObjectSpreadReconcileStatusInterface_Success(t *testing.T) {
	s := NewSpreader()
	tl := testlogger.New()
	apiObject := &pmtesting.ImplementingSpreadReconciles{}
	obj := s.MustToRuntimeObjectSpreadReconcileStatusInterface(apiObject, tl.Logger)
	assert.NotNil(t, obj)
}

func TestMustToRuntimeObjectSpreadReconcileStatusInterface_Panic(t *testing.T) {
	s := NewSpreader()
	tl := testlogger.New()
	apiObject := &pmtesting.DummyRuntimeObject{}
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic but did not panic")
		}
	}()
	_ = s.MustToRuntimeObjectSpreadReconcileStatusInterface(apiObject, tl.Logger)
}
