package avcheck

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-logr/zapr"
	"go.uber.org/zap"
)

type controllerStub struct {
	getNameFunc                     func() string
	getLastAVCheckReconcileTimeFunc func() time.Time
}

func (c controllerStub) GetName() string {
	return c.getNameFunc()
}

func (c controllerStub) GetLastAVCheckReconcileTime() time.Time {
	return c.getLastAVCheckReconcileTimeFunc()
}

func TestHandleAVCheck(t *testing.T) {
	tests := []struct {
		name                  string
		controllerAVCheckData map[string]time.Time
		expectedStatus        int
		expectedResponse      string
	}{
		{
			name: "initial state",
			controllerAVCheckData: map[string]time.Time{
				"TestReconciler": {},
				"TheReconciler":  {},
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "all fine",
			controllerAVCheckData: map[string]time.Time{
				"TestReconciler": time.Now().Add(-55 * time.Millisecond),
				"TheReconciler":  time.Now().Add(-30 * time.Millisecond),
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "one controller failed",
			controllerAVCheckData: map[string]time.Time{
				"TestReconciler":   time.Now().Add(-5 * time.Second),
				"FailedReconciler": time.Now().Add(-(minFailureThreshold + 5*time.Second)),
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", "/availability", nil)
			if err != nil {
				t.Fatal(err)
			}

			controllerStubs := []Controller{}
			for controllerName, lastAVCheckReconcileTime := range tt.controllerAVCheckData {
				controllerName := controllerName
				lastAVCheckReconcileTime := lastAVCheckReconcileTime
				c := controllerStub{
					getNameFunc: func() string {
						return controllerName
					},
					getLastAVCheckReconcileTimeFunc: func() time.Time {
						return lastAVCheckReconcileTime
					},
				}
				controllerStubs = append(controllerStubs, c)
			}

			rr := httptest.NewRecorder()
			nopLogger := zapr.NewLogger(zap.NewNop())
			handler := http.HandlerFunc(createAVCheckHandler(controllerStubs, minFailureThreshold, nopLogger))

			handler.ServeHTTP(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("handler returned wrong status code: got %v want %v", status, tt.expectedStatus)
			}
		})
	}
}
