package admission

import (
	"encoding/json"

	"k8s.io/api/admission/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newReport(requestReview *v1beta1.AdmissionReview) *report {
	return &report{
		ok:            true,
		message:       "",
		patches:       []patch{},
		requestReview: requestReview,
	}
}

// A report collects the review result: whether clusterbom creation/update is allowed or denied; a message in case of
// denial; and the patches to mutate the clusterbom.
type report struct {
	ok            bool
	message       string
	patches       []patch
	requestReview *v1beta1.AdmissionReview
}

type patch struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

func (r *report) appendPatch(op, path string, value interface{}) {
	r.patches = append(r.patches, patch{
		Op:    op,
		Path:  path,
		Value: value,
	})
}

func (r *report) appendPatches(pp ...patch) {
	r.patches = append(r.patches, pp...)
}

func (r *report) deny(message string) {
	r.ok = false
	r.message = message
}

func (r *report) denied() bool {
	return !r.ok
}

func (r *report) getResponseReview() *v1beta1.AdmissionReview {
	if r.denied() {
		return r.negativeReview()
	}

	if len(r.patches) == 0 {
		return r.positiveReview(nil, nil)
	}

	patchType := v1beta1.PatchTypeJSONPatch
	patchJSON, err := json.Marshal(r.patches)
	if err != nil {
		r.deny("cannot mutate clusterbom; error when marshaling patch: " + err.Error())
		return r.negativeReview()
	}

	return r.positiveReview(&patchType, patchJSON)
}

func (r *report) positiveReview(patchType *v1beta1.PatchType, patchJSON []byte) *v1beta1.AdmissionReview {
	return &v1beta1.AdmissionReview{
		TypeMeta: r.requestReview.TypeMeta,
		Response: &v1beta1.AdmissionResponse{
			UID:       r.requestReview.Request.UID,
			Allowed:   true,
			PatchType: patchType,
			Patch:     patchJSON,
		},
	}
}

func (r *report) negativeReview() *v1beta1.AdmissionReview {
	return &v1beta1.AdmissionReview{
		TypeMeta: r.requestReview.TypeMeta,
		Response: &v1beta1.AdmissionResponse{
			UID:     r.requestReview.Request.UID,
			Allowed: false,
			Result: &metav1.Status{
				Message: r.message,
			},
		},
	}
}
