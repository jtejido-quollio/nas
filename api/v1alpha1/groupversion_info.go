// Package v1alpha1 contains API Schema definitions for the nas.io v1alpha1 API group.
//
// This repository originally used unstructured objects in controllers.
// The types here are provided so we can (a) generate DeepCopy methods and (b)
// migrate controllers toward typed reconcilers incrementally.
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

// GroupVersion is group version used to register these objects.
var GroupVersion = schema.GroupVersion{Group: "nas.io", Version: "v1alpha1"}

// SchemeBuilder is used to add go types to the GroupVersionKind scheme.
var SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

// AddToScheme adds the types in this group-version to the given scheme.
var AddToScheme = SchemeBuilder.AddToScheme
