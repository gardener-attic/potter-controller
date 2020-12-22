/*

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

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime"
)

func (in *AppRepository) DeepCopyInto(out *AppRepository) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

func (in *AppRepository) DeepCopy() *AppRepository {
	if in == nil {
		return nil
	}
	out := new(AppRepository)
	in.DeepCopyInto(out)
	return out
}

func (in *AppRepository) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *AppRepositoryAuth) DeepCopyInto(out *AppRepositoryAuth) {
	*out = *in
	if in.Header != nil {
		in, out := &in.Header, &out.Header
		*out = new(AppRepositoryAuthHeader)
		(*in).DeepCopyInto(*out)
	}
	if in.CustomCA != nil {
		in, out := &in.CustomCA, &out.CustomCA
		*out = new(AppRepositoryCustomCA)
		(*in).DeepCopyInto(*out)
	}
}

func (in *AppRepositoryAuth) DeepCopy() *AppRepositoryAuth {
	if in == nil {
		return nil
	}
	out := new(AppRepositoryAuth)
	in.DeepCopyInto(out)
	return out
}

func (in *AppRepositoryAuthHeader) DeepCopyInto(out *AppRepositoryAuthHeader) {
	*out = *in
	in.SecretKeyRef.DeepCopyInto(&out.SecretKeyRef)
}

func (in *AppRepositoryAuthHeader) DeepCopy() *AppRepositoryAuthHeader {
	if in == nil {
		return nil
	}
	out := new(AppRepositoryAuthHeader)
	in.DeepCopyInto(out)
	return out
}

func (in *AppRepositoryCustomCA) DeepCopyInto(out *AppRepositoryCustomCA) {
	*out = *in
	in.SecretKeyRef.DeepCopyInto(&out.SecretKeyRef)
}

func (in *AppRepositoryCustomCA) DeepCopy() *AppRepositoryCustomCA {
	if in == nil {
		return nil
	}
	out := new(AppRepositoryCustomCA)
	in.DeepCopyInto(out)
	return out
}

func (in *AppRepositoryList) DeepCopyInto(out *AppRepositoryList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]AppRepository, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *AppRepositoryList) DeepCopy() *AppRepositoryList {
	if in == nil {
		return nil
	}
	out := new(AppRepositoryList)
	in.DeepCopyInto(out)
	return out
}

func (in *AppRepositoryList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *AppRepositorySpec) DeepCopyInto(out *AppRepositorySpec) {
	*out = *in
	in.Auth.DeepCopyInto(&out.Auth)
	in.SyncJobPodTemplate.DeepCopyInto(&out.SyncJobPodTemplate)
}

func (in *AppRepositorySpec) DeepCopy() *AppRepositorySpec {
	if in == nil {
		return nil
	}
	out := new(AppRepositorySpec)
	in.DeepCopyInto(out)
	return out
}

func (in *AppRepositoryStatus) DeepCopyInto(out *AppRepositoryStatus) {
	*out = *in
}

func (in *AppRepositoryStatus) DeepCopy() *AppRepositoryStatus {
	if in == nil {
		return nil
	}
	out := new(AppRepositoryStatus)
	in.DeepCopyInto(out)
	return out
}
