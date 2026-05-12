package webhook

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	hermesv1 "github.com/stubbi/hermes-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestSelfConfigValidator_Stub_AlwaysAllows(t *testing.T) {
	t.Parallel()
	v := &HermesSelfConfigValidator{}
	sc := &hermesv1.HermesSelfConfig{ObjectMeta: metav1.ObjectMeta{Name: "demo"}}
	warns, err := v.ValidateCreate(context.Background(), sc)
	assert.NoError(t, err)
	assert.NotEmpty(t, warns, "stub emits a Plan-4-TODO warning")
}
