/*
Copyright 2014 The Kubernetes Authors.

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

package resource

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

// Selector is a Visitor for resources that match a label selector.
type Selector struct {
	Client               RESTClient
	Mapping              *meta.RESTMapping
	Namespace            string
	Selector             string
	Export               bool
	IncludeUninitialized bool
	LimitChunks          int64
}

// NewSelector creates a resource selector which hides details of getting items by their label selector.
func NewSelector(client RESTClient, mapping *meta.RESTMapping, namespace string, selector string, export, includeUninitialized bool, limitChunks int64) *Selector {
	return &Selector{
		Client:               client,
		Mapping:              mapping,
		Namespace:            namespace,
		Selector:             selector,
		Export:               export,
		IncludeUninitialized: includeUninitialized,
		LimitChunks:          limitChunks,
	}
}

// Visit implements Visitor and uses request chunking by default.
func (r *Selector) Visit(fn VisitorFunc) error {
	var continueToken string
	for {
		list, err := NewHelper(r.Client, r.Mapping).List(
			r.Namespace,
			r.ResourceMapping().GroupVersionKind.GroupVersion().String(),
			r.Export,
			&metav1.ListOptions{
				LabelSelector:        r.Selector,
				IncludeUninitialized: r.IncludeUninitialized,
				Limit:                r.LimitChunks,
				Continue:             continueToken,
			},
		)
		if err != nil {
			if errors.IsResourceExpired(err) {
				return err
			}
			if errors.IsBadRequest(err) || errors.IsNotFound(err) {
				if se, ok := err.(*errors.StatusError); ok {
					// modify the message without hiding this is an API error
					if len(r.Selector) == 0 {
						se.ErrStatus.Message = fmt.Sprintf("Unable to list %q: %v", r.Mapping.Resource, se.ErrStatus.Message)
					} else {
						se.ErrStatus.Message = fmt.Sprintf("Unable to find %q that match the selector %q: %v", r.Mapping.Resource, r.Selector, se.ErrStatus.Message)
					}
					return se
				}
				if len(r.Selector) == 0 {
					return fmt.Errorf("Unable to list %q: %v", r.Mapping.Resource, err)
				}
				return fmt.Errorf("Unable to find %q that match the selector %q: %v", r.Mapping.Resource, r.Selector, err)
			}
			return err
		}
		accessor := r.Mapping.MetadataAccessor
		resourceVersion, _ := accessor.ResourceVersion(list)
		nextContinueToken, _ := accessor.Continue(list)
		info := &Info{
			Client:    r.Client,
			Mapping:   r.Mapping,
			Namespace: r.Namespace,

			Object:          list,
			ResourceVersion: resourceVersion,
		}
		if err := fn(info, nil); err != nil {
			return err
		}
		if len(nextContinueToken) == 0 {
			return nil
		}
		continueToken = nextContinueToken
	}
}

func (r *Selector) Watch(resourceVersion string) (watch.Interface, error) {
	return NewHelper(r.Client, r.Mapping).Watch(r.Namespace, resourceVersion, r.ResourceMapping().GroupVersionKind.GroupVersion().String(), r.Selector)
}

// ResourceMapping returns the mapping for this resource and implements ResourceMapping
func (r *Selector) ResourceMapping() *meta.RESTMapping {
	return r.Mapping
}
