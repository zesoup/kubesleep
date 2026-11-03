package k8s

import (
	"context"
	kubesleep "github.com/Y0-L0/kubesleep/kubesleep"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func CreateCronJob(ctx context.Context, k8s K8Simpl, namespace string, name string, suspended bool) (func() error, error) {
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: batchv1.CronJobSpec{
			Schedule: "*/1 * * * *",
			Suspend:  func(b bool) *bool { v := b; return &v }(suspended),
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							RestartPolicy: corev1.RestartPolicyNever,
							Containers: []corev1.Container{
								{
									Name:  name,
									Image: "k8s.gcr.io/pause:3.9",
								},
							},
						},
					},
				},
			},
		},
	}

	_, err := k8s.clientset.BatchV1().CronJobs(namespace).Create(ctx, cj, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}

	delete := func() error {
		return k8s.clientset.BatchV1().CronJobs(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	}
	return delete, nil
}

func (s *Integrationtest) TestCronJob_CreateDelete() {
	deleteNamespace, err := testNamespace(s.ctx, "create-delete-cronjob", s.k8s, false)
	s.Require().NoError(err)
	defer deleteNamespace()

	delete, err := CreateCronJob(s.ctx, *s.k8s, "create-delete-cronjob", "test-cronjob", false)
	s.Require().NoError(err)
	defer delete()
}

func (s *Integrationtest) TestCronJobs_Get() {
	deleteNamespace, err := testNamespace(s.ctx, "get-cronjobs", s.k8s, false)
	s.Require().NoError(err)
	defer deleteNamespace()

	delete, err := CreateCronJob(s.ctx, *s.k8s, "get-cronjobs", "test-cronjob", false)
	s.Require().NoError(err)
	defer delete()

	actual := s.getSuspendable("get-cronjobs", "2:test-cronjob")
	actual.Suspend = nil

	s.Require().Equal(
		kubesleep.NewSuspendable(
			kubesleep.CronJob,
			"test-cronjob",
			int32(1),
			nil,
		),
		actual,
	)
}

func (s *Integrationtest) TestCronJob_Suspend() {
	deleteNamespace, err := testNamespace(s.ctx, "suspend-cronjobs", s.k8s, false)
	s.Require().NoError(err)
	defer deleteNamespace()

	delete, err := CreateCronJob(s.ctx, *s.k8s, "suspend-cronjobs", "test-cronjob", false)
	s.Require().NoError(err)
	defer delete()

	before := s.getSuspendable("suspend-cronjobs", "2:test-cronjob")
	s.Require().Equal(int32(1), before.Replicas)

	s.Require().NoError(before.Suspend(s.ctx))

	actual := s.getSuspendable("suspend-cronjobs", "2:test-cronjob")
	s.Require().Equal(int32(0), actual.Replicas)
}

func (s *Integrationtest) TestCronJob_Scale() {
	deleteNamespace, err := testNamespace(s.ctx, "scale-cronjobs", s.k8s, false)
	s.Require().NoError(err)
	defer deleteNamespace()

	delete, err := CreateCronJob(s.ctx, *s.k8s, "scale-cronjobs", "test-cronjob", true)
	s.Require().NoError(err)
	defer delete()

	err = s.k8s.ScaleSuspendable(s.ctx, "scale-cronjobs", kubesleep.CronJob, "test-cronjob", 1)
	s.Require().NoError(err)

	actual := s.getSuspendable("scale-cronjobs", "2:test-cronjob")
	s.Require().Equal(int32(1), actual.Replicas)
}

func (s *Integrationtest) TestCronJob_SuspendNoopWhenAlreadySuspended() {
	deleteNamespace, err := testNamespace(s.ctx, "suspend-cronjobs-noop", s.k8s, false)
	s.Require().NoError(err)
	defer deleteNamespace()

	name := "noop-cronjob"
	delete, err := CreateCronJob(s.ctx, *s.k8s, "suspend-cronjobs-noop", name, true)
	s.Require().NoError(err)
	defer delete()

	before, err := s.k8s.clientset.BatchV1().CronJobs("suspend-cronjobs-noop").Get(
		s.ctx,
		name,
		metav1.GetOptions{},
	)
	s.Require().NoError(err)

	sus := s.getSuspendable("suspend-cronjobs-noop", "2:"+name)
	s.Require().NoError(sus.Suspend(s.ctx))

	after, err := s.k8s.clientset.BatchV1().CronJobs("suspend-cronjobs-noop").Get(
		s.ctx,
		name,
		metav1.GetOptions{},
	)
	s.Require().NoError(err)
	s.Equal(before.ResourceVersion, after.ResourceVersion, "cronjob resourceVersion changed; suspend should be a no-op when already suspended")
}

// TestCronJob_IncludedByDefault prüft die Vorgabe: ohne --label-selector
// (labelSelectors == nil) muss ein CronJob im Namespace erfasst und beim
// Suspendieren angehalten werden. Das ist die Zusage, auf die sich der
// openDesk-Nachtlauf verlässt.
func (s *Integrationtest) TestCronJob_IncludedByDefault() {
	ns := "cronjob-default"
	deleteNamespace, err := testNamespace(s.ctx, ns, s.k8s, false)
	s.Require().NoError(err)
	defer deleteNamespace()

	delete, err := CreateCronJob(s.ctx, *s.k8s, ns, "test-cronjob", false)
	s.Require().NoError(err)
	defer delete()

	suspendables, err := s.k8s.GetSuspendables(s.ctx, ns, nil)
	s.Require().NoError(err)

	sus, ok := suspendables["2:test-cronjob"]
	s.Require().True(ok, "CronJob fehlt in den Suspendables bei leerer Label-Auswahl")
	s.Require().Equal(int32(1), sus.Replicas)

	s.Require().NoError(sus.Suspend(s.ctx))

	cj, err := s.k8s.clientset.BatchV1().CronJobs(ns).Get(s.ctx, "test-cronjob", metav1.GetOptions{})
	s.Require().NoError(err)
	s.Require().NotNil(cj.Spec.Suspend)
	s.Require().True(*cj.Spec.Suspend, "CronJob ist nach dem Suspendieren nicht angehalten")

	s.Require().NoError(s.k8s.ScaleSuspendable(s.ctx, ns, kubesleep.CronJob, "test-cronjob", sus.Replicas))

	cj, err = s.k8s.clientset.BatchV1().CronJobs(ns).Get(s.ctx, "test-cronjob", metav1.GetOptions{})
	s.Require().NoError(err)
	s.Require().NotNil(cj.Spec.Suspend)
	s.Require().False(*cj.Spec.Suspend, "CronJob ist nach dem Wecken nicht freigegeben")
}
