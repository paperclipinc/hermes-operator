package webhook

import (
	"context"
	"fmt"

	hermesv1 "github.com/stubbi/hermes-operator/api/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// HermesSelfConfigValidator is the Plan-2 stub validator. It always allows but
// emits a warning. Plan 4 replaces this with policy-aware validation.
type HermesSelfConfigValidator struct{}

var _ admission.CustomValidator = &HermesSelfConfigValidator{}

func (v *HermesSelfConfigValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return validateSelfConfigStub(obj)
}

func (v *HermesSelfConfigValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	return validateSelfConfigStub(newObj)
}

func (v *HermesSelfConfigValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func validateSelfConfigStub(obj runtime.Object) (admission.Warnings, error) {
	_, ok := obj.(*hermesv1.HermesSelfConfig)
	if !ok {
		return nil, fmt.Errorf("expected *HermesSelfConfig, got %T", obj)
	}
	return admission.Warnings{
		"HermesSelfConfig policy is NOT enforced in operator v1.0.0 (Plan 2 stub); Plan 4 wires the real policy-aware validator.",
	}, nil
}
