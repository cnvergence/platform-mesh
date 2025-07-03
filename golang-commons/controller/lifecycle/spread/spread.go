package spread

import (
	"fmt"
	"math/rand/v2"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/platform-mesh/golang-commons/controller/lifecycle/runtimeobject"
	"github.com/platform-mesh/golang-commons/logger"
	"github.com/platform-mesh/golang-commons/sentry"
)

const ReconcileRefreshLabel = "platform-mesh.io/refresh-reconcile"

type Spreader struct {
}

func NewSpreader() *Spreader {
	return &Spreader{}
}

type RuntimeObjectSpreadReconcileStatus interface {
	GetGeneration() int64
	GetObservedGeneration() int64
	SetObservedGeneration(int64)
	GetNextReconcileTime() v1.Time
	SetNextReconcileTime(time v1.Time)
}

type GenerateNextReconcileTimer interface {
	GenerateNextReconcileTime() time.Duration
}

const defaultMaxReconcileDuration = 24 * time.Hour

// getNextReconcileTime returns a random time between [maxReconcileTime]/2 and [maxReconcileTime] hours
func getNextReconcileTime(maxReconcileTime time.Duration) time.Duration {

	minTime := maxReconcileTime.Minutes() / 2

	jitter := rand.Int64N(int64(minTime))

	return time.Duration(jitter+int64(minTime)) * time.Minute
}

func (s *Spreader) OnNextReconcile(instanceStatusObj RuntimeObjectSpreadReconcileStatus, log *logger.Logger) (ctrl.Result, error) {
	requeueAfter := time.Until(instanceStatusObj.GetNextReconcileTime().UTC())
	log.Debug().Int64("minutes-till-next-execution", int64(requeueAfter.Minutes())).Msg("Completed reconciliation, no processing needed")
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// SetNextReconcileTime calculates and sets the next reconcile time for the instance
func (s *Spreader) SetNextReconcileTime(instanceStatusObj RuntimeObjectSpreadReconcileStatus, log *logger.Logger) {

	var border = defaultMaxReconcileDuration
	if in, ok := instanceStatusObj.(GenerateNextReconcileTimer); ok {
		border = in.GenerateNextReconcileTime()
	}

	nextReconcileTime := getNextReconcileTime(border)

	log.Debug().Int64("minutes-till-next-execution", int64(nextReconcileTime.Minutes())).Msg("Setting next reconcile time for the instance")
	instanceStatusObj.SetNextReconcileTime(v1.NewTime(time.Now().Add(nextReconcileTime)))
}

// UpdateObservedGeneration updates the observed generation of the instance struct
func (s *Spreader) UpdateObservedGeneration(instanceStatusObj RuntimeObjectSpreadReconcileStatus, log *logger.Logger) {
	log.Debug().Int64("observed-generation", instanceStatusObj.GetObservedGeneration()).Int64("generation", instanceStatusObj.GetGeneration()).Msg("Updating observed generation")
	instanceStatusObj.SetObservedGeneration(instanceStatusObj.GetGeneration())
}
func (s *Spreader) RemoveRefreshLabelIfExists(instance runtimeobject.RuntimeObject) bool {
	keyCount := len(instance.GetLabels())
	delete(instance.GetLabels(), ReconcileRefreshLabel)
	return keyCount != len(instance.GetLabels())
}

func (s *Spreader) ToRuntimeObjectSpreadReconcileStatusInterface(instance runtimeobject.RuntimeObject, log *logger.Logger) (RuntimeObjectSpreadReconcileStatus, error) {
	if obj, ok := instance.(RuntimeObjectSpreadReconcileStatus); ok {
		return obj, nil
	}
	err := fmt.Errorf("SpreadReconciles is enabled, but instance does not implement RuntimeObjectSpreadReconcileStatus interface. This is a programming error")
	log.Error().Err(err).Msg("Failed to cast instance to RuntimeObjectSpreadReconcileStatus")
	sentry.CaptureError(err, nil)
	return nil, err
}

func (s *Spreader) MustToRuntimeObjectSpreadReconcileStatusInterface(instance runtimeobject.RuntimeObject, log *logger.Logger) RuntimeObjectSpreadReconcileStatus {
	obj, err := s.ToRuntimeObjectSpreadReconcileStatusInterface(instance, log)
	if err == nil {
		return obj
	}
	log.Panic().Err(err).Msg("Failed to cast instance to RuntimeObjectSpreadReconcileStatus")
	return nil
}
