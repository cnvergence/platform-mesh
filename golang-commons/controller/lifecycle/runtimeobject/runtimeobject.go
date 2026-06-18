package runtimeobject

import (
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type RuntimeObject interface {
	runtime.Object
	v1.Object
}
