package deployutil

import (
	"context"

	hubv1 "github.wdf.sap.corp/kubernetes/hub-controller/api/v1"
	"github.wdf.sap.corp/kubernetes/hub-controller/pkg/util"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
)

const (
	ReasonSuccessDeployment        = "SuccessDeployment"
	ReasonFailedFetchingObject     = "FailedFetchingObject"
	ReasonFailedClusterUnreachable = "FailedClusterUnreachable"
	ReasonFailedDeployment         = "FailedDeployment"
	ReasonFailedJob                = "FailedJob"
	ReasonFailedWriteState         = "FailedWriteState"
)

type EventWriterKey struct{}

func ContextWithEventWriter(ctx context.Context, eventWriter *EventWriter) context.Context {
	return context.WithValue(ctx, EventWriterKey{}, eventWriter)
}

func GetEventWriterFromContext(ctx context.Context) *EventWriter {
	value := ctx.Value(EventWriterKey{})
	if value == nil {
		return nil
	}

	eventWriter, ok := value.(*EventWriter)
	if !ok {
		return nil
	}

	return eventWriter
}

func NewEventWriter(clusterBom *hubv1.ClusterBom, eventRecorder record.EventRecorder) *EventWriter {
	return &EventWriter{
		clusterBom:    clusterBom,
		eventRecorder: eventRecorder,
	}
}

type EventWriter struct {
	clusterBom    *hubv1.ClusterBom
	eventRecorder record.EventRecorder
}

func (w *EventWriter) WriteEvent(eventtype, reason, message string) {
	if w != nil && w.clusterBom != nil && w.eventRecorder != nil {
		w.eventRecorder.Event(w.clusterBom, eventtype, reason, message)
	}
}

func LogSuccess(ctx context.Context, reason, message string) {
	log := ctx.Value(util.LoggerKey{}).(logr.Logger)
	log.V(util.LogLevelDebug).Info(message)

	eventWriter := GetEventWriterFromContext(ctx)
	eventWriter.WriteEvent(corev1.EventTypeNormal, reason, message)
}

func LogApplicationFailure(ctx context.Context, reason, message string) {
	log := ctx.Value(util.LoggerKey{}).(logr.Logger)
	log.V(util.LogLevelWarning).Info(message)

	eventWriter := GetEventWriterFromContext(ctx)
	eventWriter.WriteEvent(corev1.EventTypeWarning, reason, message)
}

func LogHubFailure(ctx context.Context, reason, message string, err error) {
	log := ctx.Value(util.LoggerKey{}).(logr.Logger)
	log.Error(err, message)

	if err != nil {
		message = message + " - " + err.Error()
	}
	eventWriter := GetEventWriterFromContext(ctx)
	eventWriter.WriteEvent(corev1.EventTypeWarning, reason, message)
}
