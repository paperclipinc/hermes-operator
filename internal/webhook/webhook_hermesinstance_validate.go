package webhook

import (
	"context"
	"fmt"

	hermesv1 "github.com/stubbi/hermes-operator/api/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// HermesInstanceValidator enforces design §7.3 rules.
type HermesInstanceValidator struct{}

var _ admission.CustomValidator = &HermesInstanceValidator{}

// Ptr is the package-local generic pointer helper.
func Ptr[T any](v T) *T { return &v }

// intOrStr is a test/internal helper.
func intOrStr(s string) intstr.IntOrString { return intstr.FromString(s) }

// ValidateCreate runs the full sanity ruleset on a fresh resource.
func (v *HermesInstanceValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	inst, ok := obj.(*hermesv1.HermesInstance)
	if !ok {
		return nil, fmt.Errorf("expected *HermesInstance, got %T", obj)
	}
	return validateCommon(inst)
}

// ValidateUpdate runs the create rules + immutability rules.
func (v *HermesInstanceValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldI, ok1 := oldObj.(*hermesv1.HermesInstance)
	newI, ok2 := newObj.(*hermesv1.HermesInstance)
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("ValidateUpdate types: old=%T new=%T", oldObj, newObj)
	}
	if err := validateImmutable(oldI, newI); err != nil {
		return nil, err
	}
	return validateCommon(newI)
}

// ValidateDelete is a no-op.
func (v *HermesInstanceValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func validateCommon(inst *hermesv1.HermesInstance) (admission.Warnings, error) {
	var warns admission.Warnings

	if inst.Spec.Image.Repository == "" {
		return warns, fmt.Errorf("spec.image.repository is required (set on the instance or via HermesClusterDefaults)")
	}
	if inst.Spec.Storage.Persistence.Size == "" {
		return warns, fmt.Errorf("spec.storage.persistence.size is required")
	}

	if inst.Spec.Config.Raw != nil && inst.Spec.Config.ConfigMapRef != nil && inst.Spec.Config.MergeMode == "" {
		warns = append(warns, "spec.config.raw and spec.config.configMapRef are both set without spec.config.mergeMode; defaults to 'replace' (Raw wins)")
	}

	if inst.Spec.SelfConfigure.Enabled != nil && *inst.Spec.SelfConfigure.Enabled {
		if len(inst.Spec.SelfConfigure.ProtectedKeys) == 0 {
			return warns, fmt.Errorf("spec.selfConfigure.enabled=true requires non-empty spec.selfConfigure.protectedKeys (explicit allowlist policy)")
		}
		if len(inst.Spec.SelfConfigure.AllowedActions) == 0 {
			return warns, fmt.Errorf("spec.selfConfigure.enabled=true requires non-empty spec.selfConfigure.allowedActions")
		}
	}

	pdb := inst.Spec.Availability.PodDisruptionBudget
	if pdb.MinAvailable != nil && pdb.MaxUnavailable != nil {
		return warns, fmt.Errorf("spec.availability.podDisruptionBudget: MinAvailable and MaxUnavailable are mutually exclusive")
	}

	hpa := inst.Spec.Availability.HorizontalPodAutoscaler
	if hpa.MinReplicas != nil && hpa.MaxReplicas != nil && *hpa.MinReplicas > *hpa.MaxReplicas {
		return warns, fmt.Errorf("spec.availability.horizontalPodAutoscaler: MinReplicas > MaxReplicas")
	}

	return warns, nil
}

func validateImmutable(oldI, newI *hermesv1.HermesInstance) error {
	if oldI.Spec.Storage.Persistence.StorageClassName != nil &&
		(newI.Spec.Storage.Persistence.StorageClassName == nil ||
			*oldI.Spec.Storage.Persistence.StorageClassName != *newI.Spec.Storage.Persistence.StorageClassName) {
		return fmt.Errorf("spec.storage.persistence.storageClassName is immutable")
	}
	if oldI.Name != newI.Name {
		return fmt.Errorf("metadata.name is immutable")
	}
	return nil
}

var _ = webhook.Admission{}
