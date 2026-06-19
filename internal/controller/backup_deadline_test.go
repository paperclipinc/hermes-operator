package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	hermesv1 "github.com/paperclipinc/hermes-operator/api/v1"
)

// onDeleteInstance returns a HermesInstance that holds the backup-on-delete
// finalizer and has spec.backup.onDelete + a (placeholder) S3 target.
func onDeleteInstance() *hermesv1.HermesInstance {
	return &hermesv1.HermesInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "dl",
			Namespace:  "default",
			Finalizers: []string{hermesv1.FinalizerBackupOnDelete},
		},
		Spec: hermesv1.HermesInstanceSpec{
			Backup: hermesv1.BackupSpec{
				OnDelete: true,
				S3: &hermesv1.BackupS3Spec{
					Bucket:               "b",
					Endpoint:             "https://s3.example.com",
					CredentialsSecretRef: hermesv1.LocalObjectReference{Name: "creds"},
				},
			},
		},
	}
}

func backupTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	sch := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(sch))
	require.NoError(t, hermesv1.AddToScheme(sch))
	return sch
}

// deletingClient builds a fake client holding inst, then Deletes it so it carries
// a deletionTimestamp (the finalizer keeps it around), and returns the live copy.
func deletingClient(t *testing.T, sch *runtime.Scheme, inst *hermesv1.HermesInstance) (client.Client, *hermesv1.HermesInstance) {
	t.Helper()
	cl := fake.NewClientBuilder().
		WithScheme(sch).
		WithObjects(inst).
		WithStatusSubresource(&hermesv1.HermesInstance{}).
		Build()
	ctx := context.Background()
	require.NoError(t, cl.Delete(ctx, inst))
	got := &hermesv1.HermesInstance{}
	require.NoError(t, cl.Get(ctx, client.ObjectKeyFromObject(inst), got))
	require.NotNil(t, got.DeletionTimestamp, "expected deletionTimestamp after Delete")
	return cl, got
}

// #93: once the grace window elapses, a failing/stuck final backup must not keep
// the instance undeletable — HandleDeletion releases the finalizer.
func TestHandleDeletion_DeadlineReleasesFinalizer(t *testing.T) {
	orig := finalBackupDeadline
	finalBackupDeadline = 0 // grace window already exceeded
	defer func() { finalBackupDeadline = orig }()

	sch := backupTestScheme(t)
	cl, inst := deletingClient(t, sch, onDeleteInstance())

	b := &BackupReconciler{Client: cl, Scheme: sch, Recorder: record.NewFakeRecorder(10)}
	_, held, err := b.HandleDeletion(context.Background(), inst)
	require.NoError(t, err)
	assert.False(t, held, "deadline exceeded: finalizer must be released so deletion proceeds")

	// Finalizer gone -> the fake client garbage-collects the terminating object.
	after := &hermesv1.HermesInstance{}
	if err := cl.Get(context.Background(), client.ObjectKeyFromObject(inst), after); err == nil {
		assert.False(t, controllerutil.ContainsFinalizer(after, hermesv1.FinalizerBackupOnDelete),
			"finalizer should be removed after the deadline give-up")
	}
}

// Within the grace window the finalizer is still held and the final backup Job is
// started — deletion is intentionally blocked while the snapshot is attempted.
func TestHandleDeletion_WithinDeadlineHoldsFinalizer(t *testing.T) {
	orig := finalBackupDeadline
	finalBackupDeadline = time.Hour // plenty of grace
	defer func() { finalBackupDeadline = orig }()

	sch := backupTestScheme(t)
	cl, inst := deletingClient(t, sch, onDeleteInstance())

	b := &BackupReconciler{Client: cl, Scheme: sch, Recorder: record.NewFakeRecorder(10)}
	_, held, err := b.HandleDeletion(context.Background(), inst)
	require.NoError(t, err)
	assert.True(t, held, "within the grace window the finalizer must still be held")

	// The final backup Job should have been created, and the finalizer kept.
	job := &batchv1.Job{}
	require.NoError(t, cl.Get(context.Background(),
		client.ObjectKey{Namespace: inst.Namespace, Name: FinalBackupJobName(inst)}, job))

	after := &hermesv1.HermesInstance{}
	require.NoError(t, cl.Get(context.Background(), client.ObjectKeyFromObject(inst), after))
	assert.True(t, controllerutil.ContainsFinalizer(after, hermesv1.FinalizerBackupOnDelete),
		"finalizer must remain while the backup is in flight")
}
